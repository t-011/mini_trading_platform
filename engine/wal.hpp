#pragma once
// Write-ahead log: raw Requests, appended before they're handled.
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <string>
#include <fcntl.h>
#include <sys/stat.h>
#include <unistd.h>

#include "book.hpp"

int open_wal(const std::string& path) {
    return open(path.c_str(), O_RDWR | O_CREAT | O_APPEND, 0600);
}

// No events are sent during replay, Go already got them. A partial last
// record (crash mid-write) was never handled, so it gets truncated.
uint64_t replay_wal(int wal, Book& book) {
    struct stat info;
    if (fstat(wal, &info) != 0) return 0;
    off_t whole = info.st_size - info.st_size % (off_t)sizeof(Request);
    if (whole != info.st_size && ftruncate(wal, whole) != 0) {
        std::perror("wal: truncate partial record");
        std::exit(1);
    }

    uint64_t count = 0;
    Request request;
    // TODO: read in chunks if replay gets slow
    while (pread(wal, &request, sizeof request, (off_t)(count * sizeof request)) == sizeof request) {
        handle(book, request, [](const Event&) {});
        count++;
    }
    return count;
}

// NOTE: no fsync. Survives the engine crashing, not the machine going down.
bool append_wal(int wal, const Request& request) {
    return write(wal, &request, sizeof request) == sizeof request;
}
