// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Helper for cryptohome.SequentialConsistency tast test. This program
// sequentially writes a number of files, conveniently named
// SequentialConsistencyTest.0.txt, SequentialConsistencyTest.1.txt, etc, to
// a directory specified by the command line. This is useful for testing that
// one process sees files created by another process being created in the same
// order.

#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>

#include <string>

#include <base/containers/span.h>
#include <base/files/file.h>
#include <base/files/file_path.h>
#include <base/logging.h>
#include <base/strings/stringprintf.h>
#include <brillo/flag_helper.h>
#include <brillo/syslog_logging.h>

namespace {
// Both of these should match sequential_consistency.go.
const int kNumFiles = 9;
const char kBasePhrase[] = "This is file #%d";

// makeFilePath here should match makeFilePath in sequential_consistency.go.
base::FilePath makeFilePath(const base::FilePath &path, int file_num) {
  return path.Append(
      base::StringPrintf("SequentialConsistencyTest.%d.txt", file_num));
}

bool createFile(const base::FilePath &path, int file_num, bool slowly = false) {
  base::FilePath file_path = makeFilePath(path, file_num);
  base::File f(file_path,
               base::File::FLAG_CREATE_ALWAYS | base::File::FLAG_WRITE);
  if (!f.IsValid()) {
    LOG(ERROR) << "file open(" << file_path.value()
               << "): " << base::File::ErrorToString(f.error_details());
    return false;
  }

  std::string phrase = base::StringPrintf(kBasePhrase, file_num);
  if (slowly) {
    for (int i = 0; i < phrase.length(); i++) {
      char c = phrase[i];
      auto result = f.WriteAtCurrentPos(base::byte_span_from_ref(c));
      if (!result.has_value() || *result != 1u) {
        LOG(ERROR) << file_path.value() << " WriteAtCurrentPos() returned "
                   << (result.has_value() ? static_cast<long long>(*result)
                                          : -1)
                   << ": "
                   << base::File::ErrorToString(base::File::GetLastFileError());
        return false;
      }
    }
  } else {
    auto result = f.WriteAtCurrentPos(base::as_byte_span(phrase));
    if (!result.has_value() || *result != phrase.size()) {
      LOG(ERROR) << file_path.value() << " WriteAtCurrentPos() returned "
                 << (result.has_value() ? static_cast<long long>(*result) : -1)
                 << ": "
                 << base::File::ErrorToString(base::File::GetLastFileError());
      return false;
    }
  }
  return true;
}

int createFiles(bool create_dir, const base::FilePath &path) {
  // sleep briefly to increase the odds that the Go half of the
  // cryptohome.SequentialConsistency test is in its testing.Poll loop. This is
  // not necessary for the test to be correct -- if, say, the machine is
  // overloaded and the Go process doesn't schedule for 10 seconds, the test
  // will still pass. It just increases the odds that if there is a problem in
  // the depths of the file system driver, a particular test run will catch it.
  // We don't do more careful synchronization because the ideal test run has
  // the function inside the testing.Poll code running on one CPU at the
  // same instant the file creation code below is running on another CPU.
  // There's no way to force that with synchronization. In fact, careful
  // synchronization will decrease the odds of it happening. In many cases,
  // when one process releases a lock that another process is waiting for, the
  // OS will context-switch to the waiting process and (if there's not enough
  // CPUs available) have the releasing process end up going into a wait.
  sleep(2);

  if (create_dir) {
    // base::CreateDirectory doesn't support permissions
    if (mkdir(path.value().c_str(), 0755) != 0) {
      PLOG(ERROR) << "mkdir(" << path.value() << "): ";
      return EXIT_FAILURE;
    }
  }

  int file_num = 0;
  // Immediately create first three files.
  for (; file_num < 3; ++file_num) {
    if (!createFile(path, file_num)) {
      return EXIT_FAILURE;
    }
  }

  // Sleep briefly and create the next three. The sleep is just to exercise a
  // few different scenarios (multiple rapid-fire file creation vs a delay
  // between the file creations); it's not necessary for the correctness of the
  // test.
  sleep(2);
  for (; file_num < 6; ++file_num) {
    if (!createFile(path, file_num)) {
      return EXIT_FAILURE;
    }
  }

  // Write out the remaining three slowly.
  for (; file_num < kNumFiles; ++file_num) {
    if (!createFile(path, file_num, true)) {
      return EXIT_FAILURE;
    }
  }

  return EXIT_SUCCESS;
}
} // namespace

int main(int argc, char **argv) {
  brillo::InitLog(brillo::kLogToSyslog | brillo::kLogToStderrIfTty);

  DEFINE_bool(create_dir, false, "Create directory given by path");
  DEFINE_string(path, "", "Directory to put files in");
  brillo::FlagHelper::Init(argc, argv,
                           "cryptohome.SequentialConsistency tast test helper");
  CHECK_NE(FLAGS_path, "");

  // Fork to detach process from parent. This more closely replicates the
  // conditions where errors have been seen in the wild.
  pid_t fork_result = fork();
  if (fork_result == -1) {
    perror("fork");
    return EXIT_FAILURE;
  }

  if (fork_result == 0) {
    // Child.
    return createFiles(FLAGS_create_dir, base::FilePath(FLAGS_path));
  } else {
    return EXIT_SUCCESS;
  }
}
