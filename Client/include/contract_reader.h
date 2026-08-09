#pragma once
#ifdef ENABLE_CONTRACT_URL

#include <string>

// Fetches the URL stored in the TextStorage smart contract via Ethereum JSON-RPC.
// CONTRACT_ADDRESS and CONTRACT_RPC_URL must be set as compile-time definitions.
// Returns an empty string on failure.
std::string FetchUrlFromContract();

#endif // ENABLE_CONTRACT_URL
