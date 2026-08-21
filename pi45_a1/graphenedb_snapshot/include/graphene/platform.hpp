#pragma once
#include "graphene/types.hpp"
#include <chrono>
#include <filesystem>
#include <optional>
#include <string>

namespace graphene::platform {

using FileHandle = intptr_t;
static constexpr FileHandle kInvalidFile = -1;

Status open_append(const std::filesystem::path& path, FileHandle* out);
Status write_all(FileHandle handle, const char* data, size_t size);
Status flush(FileHandle handle);
void close_file(FileHandle handle);
Status create_file_exclusive(const std::filesystem::path& path, const std::string& contents);
Status flush_path(const std::filesystem::path& path);
Status durable_replace(const std::filesystem::path& source, const std::filesystem::path& destination);
Status truncate_and_flush(const std::filesystem::path& path);

uint64_t current_pid();
bool process_is_alive(uint64_t pid);
std::optional<std::chrono::system_clock::time_point> current_process_start_time();

} // namespace graphene::platform
