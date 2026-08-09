// PolyDeploy C++ Client
// Uses only Windows-native WinHTTP — no external libraries required.
// Reads deployment.json, calls getText() via JSON-RPC (eth_call), prints result.
//
// Build:   cmake -B build && cmake --build build --config Release
// Run:     build\Release\polydeploy-client.exe [--deployment <path>] [--rpc <url>]

#define WIN32_LEAN_AND_MEAN
#define NOMINMAX
#include <windows.h>
#include <winhttp.h>

#include <cstdint>
#include <fstream>
#include <iostream>
#include <sstream>
#include <stdexcept>
#include <string>
#include <vector>

// ─── RAII wrapper for HINTERNET ───────────────────────────────────────────────

struct HttpHandle {
    HINTERNET h = nullptr;
    explicit HttpHandle(HINTERNET h) : h(h) {}
    ~HttpHandle() { if (h) WinHttpCloseHandle(h); }
    operator HINTERNET() const { return h; }
    bool operator!()     const { return !h; }
    HttpHandle(const HttpHandle&)            = delete;
    HttpHandle& operator=(const HttpHandle&) = delete;
};

// ─── String helpers ───────────────────────────────────────────────────────────

static std::wstring toWide(const std::string& s) {
    if (s.empty()) return {};
    int n = MultiByteToWideChar(CP_UTF8, 0, s.c_str(), -1, nullptr, 0);
    std::wstring w(n, L'\0');
    MultiByteToWideChar(CP_UTF8, 0, s.c_str(), -1, w.data(), n);
    if (!w.empty() && w.back() == L'\0') w.pop_back();
    return w;
}

// ─── WinHTTP HTTPS POST ───────────────────────────────────────────────────────

static std::string httpPost(const std::string& url, const std::string& body) {
    std::wstring wUrl = toWide(url);

    wchar_t host[512]  = {};
    wchar_t path[2048] = {};
    URL_COMPONENTS uc  = {};
    uc.dwStructSize      = sizeof(uc);
    uc.lpszHostName      = host;
    uc.dwHostNameLength  = ARRAYSIZE(host);
    uc.lpszUrlPath       = path;
    uc.dwUrlPathLength   = ARRAYSIZE(path);

    if (!WinHttpCrackUrl(wUrl.c_str(), 0, 0, &uc))
        throw std::runtime_error("Invalid URL: " + url);

    HttpHandle hSess{WinHttpOpen(L"PolyDeploy/1.0",
                                  WINHTTP_ACCESS_TYPE_DEFAULT_PROXY,
                                  WINHTTP_NO_PROXY_NAME,
                                  WINHTTP_NO_PROXY_BYPASS, 0)};
    if (!hSess) throw std::runtime_error("WinHttpOpen failed");
    WinHttpSetTimeouts(hSess, 30000, 30000, 30000, 30000);

    HttpHandle hConn{WinHttpConnect(hSess, host, uc.nPort, 0)};
    if (!hConn) throw std::runtime_error("WinHttpConnect failed");

    const wchar_t* pPath = (path[0] ? path : L"/");
    bool isHttps = (uc.nScheme == INTERNET_SCHEME_HTTPS);
    HttpHandle hReq{WinHttpOpenRequest(hConn, L"POST", pPath,
                                        nullptr, WINHTTP_NO_REFERER,
                                        WINHTTP_DEFAULT_ACCEPT_TYPES,
                                        isHttps ? WINHTTP_FLAG_SECURE : 0)};
    if (!hReq) throw std::runtime_error("WinHttpOpenRequest failed");

    WinHttpAddRequestHeaders(hReq,
        L"Content-Type: application/json\r\n",
        (DWORD)-1, WINHTTP_ADDREQ_FLAG_ADD);

    if (!WinHttpSendRequest(hReq, WINHTTP_NO_ADDITIONAL_HEADERS, 0,
                             (LPVOID)body.c_str(), (DWORD)body.size(),
                             (DWORD)body.size(), 0) ||
        !WinHttpReceiveResponse(hReq, nullptr)) {
        throw std::runtime_error("HTTP request failed (WinHTTP error " +
                                  std::to_string(GetLastError()) + ")");
    }

    std::string resp;
    DWORD avail = 0;
    while (WinHttpQueryDataAvailable(hReq, &avail) && avail > 0) {
        std::vector<char> buf(avail);
        DWORD read = 0;
        if (!WinHttpReadData(hReq, buf.data(), avail, &read)) break;
        resp.append(buf.data(), read);
    }
    return resp;
}

// ─── Minimal JSON helpers ─────────────────────────────────────────────────────
// Purpose-built for the fixed shapes of deployment.json and the RPC response.

/// First string value for "key": "value" found anywhere in json.
static std::string jStr(const std::string& json, const std::string& key) {
    const std::string pat = "\"" + key + "\"";
    size_t p = json.find(pat);
    if (p == std::string::npos)
        throw std::runtime_error("JSON key not found: " + key);
    p = json.find('"', json.find(':', p + pat.size()) + 1);
    if (p == std::string::npos)
        throw std::runtime_error("JSON value missing for: " + key);
    size_t e = p + 1;
    while (e < json.size() && json[e] != '"') {
        if (json[e] == '\\') ++e; // skip escaped character
        ++e;
    }
    return json.substr(p + 1, e - p - 1);
}

/// First integer value for "key": number found in json.
static int64_t jNum(const std::string& json, const std::string& key) {
    const std::string pat = "\"" + key + "\"";
    size_t p = json.find(pat);
    if (p == std::string::npos) return 0;
    p = json.find(':', p + pat.size());
    if (p == std::string::npos) return 0;
    ++p;
    while (p < json.size() && (json[p]==' '||json[p]=='\t'||
                                json[p]=='\r'||json[p]=='\n')) ++p;
    return std::stoll(json.substr(p));
}

// ─── ABI string decoder ───────────────────────────────────────────────────────
// getText() returns a Solidity dynamic string encoded as:
//   [32-byte offset] [32-byte length] [UTF-8 bytes, right-padded to 32 bytes]

static uint64_t parseWord64(const std::string& hex, size_t wordStart) {
    if (hex.size() < wordStart + 64)
        throw std::runtime_error("ABI data truncated at word offset " +
                                  std::to_string(wordStart));
    // Take last 16 hex chars of the 64-char word (enough for any real value).
    return std::stoull(hex.substr(wordStart + 48, 16), nullptr, 16);
}

static std::string decodeABIString(const std::string& hexData) {
    std::string h = hexData;
    if (h.size() >= 2 && h[0] == '0' && (h[1] == 'x' || h[1] == 'X'))
        h = h.substr(2);
    if (h.empty())
        throw std::runtime_error("Empty response — is the contract deployed?");

    uint64_t byteOffset = parseWord64(h, 0);
    uint64_t byteLen    = parseWord64(h, static_cast<size_t>(byteOffset * 2));
    size_t   dataStart  = static_cast<size_t>(byteOffset * 2) + 64;

    if (h.size() < dataStart + byteLen * 2)
        throw std::runtime_error("ABI string data is truncated");

    std::string result;
    result.reserve(static_cast<size_t>(byteLen));
    for (uint64_t i = 0; i < byteLen; ++i)
        result += static_cast<char>(
            std::stoul(h.substr(dataStart + i * 2, 2), nullptr, 16));
    return result;
}

// ─── main ─────────────────────────────────────────────────────────────────────

int main(int argc, char* argv[]) {
    std::string deploymentFile = "../deployment.json";
    std::string rpcUrl;

    for (int i = 1; i < argc; ++i) {
        std::string a = argv[i];
        if ((a == "--deployment" || a == "-d") && i + 1 < argc)
            deploymentFile = argv[++i];
        else if ((a == "--rpc" || a == "-r") && i + 1 < argc)
            rpcUrl = argv[++i];
        else if (a == "--help" || a == "-h") {
            std::cout << "Usage: polydeploy-client [--deployment <file>] [--rpc <url>]\n";
            return 0;
        }
    }

    // Load deployment.json
    std::string depJson;
    {
        std::ifstream f(deploymentFile);
        if (!f.is_open()) f.open("deployment.json");
        if (!f.is_open()) {
            std::cerr << "Cannot open deployment.json\n"
                      << "Run the Go deployer first to generate it.\n";
            return 1;
        }
        std::ostringstream ss;
        ss << f.rdbuf();
        depJson = ss.str();
    }

    std::string contractAddress, getTextSelector, network;
    int64_t     chainId = 0;
    try {
        contractAddress = jStr(depJson, "address");
        getTextSelector = jStr(depJson, "getText");  // inside "selectors": { ... }
        network         = jStr(depJson, "network");
        chainId         = jNum(depJson, "chainId");
    } catch (const std::exception& e) {
        std::cerr << "Failed to parse deployment.json: " << e.what() << "\n";
        return 1;
    }

    if (rpcUrl.empty()) rpcUrl = "https://polygon.drpc.org";

    std::cout << "PolyDeploy Client\n"
              << "=================\n"
              << "Contract  : " << contractAddress << "\n"
              << "Network   : " << network << " (chain " << chainId << ")\n"
              << "RPC       : " << rpcUrl << "\n"
              << "Selector  : getText() = " << getTextSelector << "\n\n";

    // Build JSON-RPC eth_call request body
    std::string reqBody =
        R"({"jsonrpc":"2.0","method":"eth_call","params":[{"to":")" +
        contractAddress + R"(","data":")" + getTextSelector +
        R"("},"latest"],"id":1})";

    std::cout << "Querying contract...\n";

    std::string rawResp;
    try {
        rawResp = httpPost(rpcUrl, reqBody);
    } catch (const std::exception& e) {
        std::cerr << "HTTP error: " << e.what() << "\n";
        return 1;
    }

    if (rawResp.find("\"error\"") != std::string::npos) {
        std::cerr << "RPC returned an error:\n" << rawResp << "\n";
        return 1;
    }

    std::string hexResult;
    try {
        hexResult = jStr(rawResp, "result");
    } catch (const std::exception& e) {
        std::cerr << "Failed to parse RPC response: " << e.what()
                  << "\nRaw: " << rawResp << "\n";
        return 1;
    }

    std::string text;
    try {
        text = decodeABIString(hexResult);
    } catch (const std::exception& e) {
        std::cerr << "ABI decode failed: " << e.what()
                  << "\nRaw hex: " << hexResult << "\n";
        return 1;
    }

    std::cout << "\nStored text\n"
              << "-----------\n"
              << text << "\n";

    return 0;
}

