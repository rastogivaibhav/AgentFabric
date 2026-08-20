cmake_minimum_required(VERSION 3.16)
project(GrapheneDBA1FrozenCore LANGUAGES CXX)
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)
add_library(graphenedb
  src/db.cpp
  src/platform_posix.cpp
)
add_library(GrapheneDB::graphenedb ALIAS graphenedb)
target_include_directories(graphenedb PUBLIC ${CMAKE_CURRENT_SOURCE_DIR}/include)
find_package(Threads REQUIRED)
target_link_libraries(graphenedb PUBLIC Threads::Threads)
