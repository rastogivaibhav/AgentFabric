#include "graphene/platform.hpp"

#ifndef _WIN32
#include <cerrno>
#include <chrono>
#include <cstring>
#include <fcntl.h>
#include <fstream>
#include <sstream>
#include <signal.h>
#include <sys/types.h>
#include <unistd.h>
#ifdef __APPLE__
#include <sys/sysctl.h>
#include <sys/time.h>
#endif

namespace graphene::platform {

Status open_append(const std::filesystem::path& path, FileHandle* out) {
  if (!out) return Status::error(ErrorCode::InvalidInput, "out file handle cannot be null");
  int fd = ::open(path.c_str(), O_CREAT | O_APPEND | O_WRONLY, 0644);
  if (fd < 0) return Status::error(ErrorCode::IoError, std::string("open append failed: ") + std::strerror(errno));
  *out = static_cast<FileHandle>(fd);
  return Status::ok();
}

Status write_all(FileHandle handle, const char* data, size_t size) {
  int fd = static_cast<int>(handle);
  size_t remaining = size;
  while (remaining > 0) {
    ssize_t n = ::write(fd, data, remaining);
    if (n < 0) return Status::error(ErrorCode::IoError, std::string("write failed: ") + std::strerror(errno));
    data += n;
    remaining -= static_cast<size_t>(n);
  }
  return Status::ok();
}

Status flush(FileHandle handle) {
  int fd = static_cast<int>(handle);
  if (::fsync(fd) != 0) return Status::error(ErrorCode::IoError, std::string("fsync failed: ") + std::strerror(errno));
  return Status::ok();
}

void close_file(FileHandle handle) {
  if (handle != kInvalidFile) ::close(static_cast<int>(handle));
}

Status create_file_exclusive(const std::filesystem::path& path, const std::string& contents) {
  int fd = ::open(path.c_str(), O_CREAT | O_EXCL | O_WRONLY, 0644);
  if (fd < 0) {
    if (errno == EEXIST) return Status::error(ErrorCode::LockBusy, "file already exists: " + path.string());
    return Status::error(ErrorCode::IoError, std::string("exclusive create failed: ") + std::strerror(errno));
  }
  auto st = write_all(static_cast<FileHandle>(fd), contents.data(), contents.size());
  if (st) st = flush(static_cast<FileHandle>(fd));
  const int close_result = ::close(fd);
  if (st && close_result != 0) {
    st = Status::error(ErrorCode::IoError, std::string("close exclusive file failed: ") + std::strerror(errno));
  }
  if (!st) {
    std::error_code ec;
    std::filesystem::remove(path, ec);
  }
  return st;
}

Status flush_path(const std::filesystem::path& path) {
  int fd = ::open(path.c_str(), O_RDONLY);
  if (fd < 0) return Status::error(ErrorCode::IoError, std::string("open for flush failed: ") + std::strerror(errno));
  const int flush_result = ::fsync(fd);
  const int flush_errno = errno;
  const int close_result = ::close(fd);
  if (flush_result != 0) return Status::error(ErrorCode::IoError, std::string("fsync path failed: ") + std::strerror(flush_errno));
  if (close_result != 0) return Status::error(ErrorCode::IoError, std::string("close flushed path failed: ") + std::strerror(errno));
  return Status::ok();
}

Status durable_replace(const std::filesystem::path& source, const std::filesystem::path& destination) {
  auto st = flush_path(source);
  if (!st) return st;
  if (::rename(source.c_str(), destination.c_str()) != 0) {
    return Status::error(ErrorCode::IoError, std::string("atomic replace failed: ") + std::strerror(errno));
  }
#ifdef O_DIRECTORY
  int dfd = ::open(destination.parent_path().c_str(), O_RDONLY | O_DIRECTORY);
#else
  int dfd = ::open(destination.parent_path().c_str(), O_RDONLY);
#endif
  if (dfd < 0) return Status::error(ErrorCode::IoError, std::string("open parent directory failed: ") + std::strerror(errno));
  const int flush_result = ::fsync(dfd);
  const int flush_errno = errno;
  ::close(dfd);
  if (flush_result != 0) return Status::error(ErrorCode::IoError, std::string("fsync parent directory failed: ") + std::strerror(flush_errno));
  return Status::ok();
}

Status truncate_and_flush(const std::filesystem::path& path) {
  int fd = ::open(path.c_str(), O_CREAT | O_TRUNC | O_WRONLY, 0644);
  if (fd < 0) return Status::error(ErrorCode::IoError, std::string("open for truncate failed: ") + std::strerror(errno));
  const int flush_result = ::fsync(fd);
  const int flush_errno = errno;
  ::close(fd);
  if (flush_result != 0) return Status::error(ErrorCode::IoError, std::string("fsync truncated file failed: ") + std::strerror(flush_errno));
  return Status::ok();
}

uint64_t current_pid() { return static_cast<uint64_t>(::getpid()); }

bool process_is_alive(uint64_t pid) {
  if (pid <= 1) return true;
  return (::kill(static_cast<pid_t>(pid), 0) == 0 || errno != ESRCH);
}

std::optional<std::chrono::system_clock::time_point> current_process_start_time() {
#ifdef __APPLE__
  int mib[4] = {CTL_KERN, KERN_PROC, KERN_PROC_PID,
                static_cast<int>(::getpid())};
  struct kinfo_proc process_info {};
  size_t process_info_size = sizeof(process_info);
  if (::sysctl(mib, 4, &process_info, &process_info_size, nullptr, 0) != 0 ||
      process_info_size == 0) {
    return std::nullopt;
  }
  const auto seconds = std::chrono::seconds(
      static_cast<int64_t>(process_info.kp_proc.p_starttime.tv_sec));
  const auto microseconds = std::chrono::microseconds(
      static_cast<int64_t>(process_info.kp_proc.p_starttime.tv_usec));
  return std::chrono::system_clock::time_point{seconds + microseconds};
#else
  std::ifstream stat("/proc/self/stat");
  std::ifstream proc_stat("/proc/stat");
  if (!stat || !proc_stat) return std::nullopt;

  std::string line;
  std::getline(stat, line);
  const auto rparen = line.rfind(')');
  if (rparen == std::string::npos) return std::nullopt;
  std::istringstream fields(line.substr(rparen + 2));
  std::string field;
  uint64_t start_ticks = 0;
  for (int index = 3; index <= 22; ++index) {
    if (!(fields >> field)) return std::nullopt;
    if (index == 22) {
      try {
        start_ticks = std::stoull(field);
      } catch (...) {
        return std::nullopt;
      }
    }
  }

  std::string proc_line;
  uint64_t boot_time = 0;
  while (std::getline(proc_stat, proc_line)) {
    if (proc_line.rfind("btime ", 0) == 0) {
      try {
        boot_time = std::stoull(proc_line.substr(6));
      } catch (...) {
        return std::nullopt;
      }
      break;
    }
  }
  if (boot_time == 0) return std::nullopt;

  const long ticks_per_second = ::sysconf(_SC_CLK_TCK);
  if (ticks_per_second <= 0) return std::nullopt;

  const auto boot = std::chrono::system_clock::time_point{
      std::chrono::seconds(static_cast<int64_t>(boot_time))};
  const auto offset = std::chrono::duration_cast<
      std::chrono::system_clock::duration>(std::chrono::duration<double>(
      static_cast<double>(start_ticks) /
      static_cast<double>(ticks_per_second)));
  return boot + offset;
#endif
}

} // namespace graphene::platform
#endif
