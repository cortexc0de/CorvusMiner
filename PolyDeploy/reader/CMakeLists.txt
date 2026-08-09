cmake_minimum_required(VERSION 3.16)
project(polydeploy-client VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

add_executable(polydeploy-client main.cpp)

# WinHTTP ships with every Windows installation — no external packages needed.
target_link_libraries(polydeploy-client PRIVATE winhttp)
