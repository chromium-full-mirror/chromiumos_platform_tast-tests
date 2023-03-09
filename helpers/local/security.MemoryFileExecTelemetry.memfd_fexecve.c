// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

#define _GNU_SOURCE
#define _POSIX_C_SOURCE 200809L

#include <asm-generic/errno-base.h>
#include <err.h>
#include <errno.h>
#include <fcntl.h>
#include <sys/mman.h>
#include <sys/types.h>
#include <unistd.h>

// This program accepts a path to a binary; It attempts to load the binary into
// a memory file and execute it. This is used for verifying that memfd
// executions are blocked and reported as expected.
//
// The supplied program is another helper named `test_bin` compiled from:
// `security.MemoryFileExecTelemetry.test_bin.c`. This makes it possible to have
// a defined behavior (e.g.: return code) regardless of whether the execution is
// blocked or not.
//
// Return code 0 means that the execution of memfd was blocked as expected.
// Error code 1 indicates a failure in an auxiliary operation. Error code 2
// indicates that the memfd execution attempt did not fail with EACCES, or did
// not fail at all.
int main(int argc, char *argv[]) {
  int n;
  char buf[4096];

  if (argc != 2) err(1, "Please supply exactly one argument");

  int exe = open(argv[1], O_RDONLY);
  if (exe == -1) {
    err(1, "Error opening the supplied executable");
  }

  int fd = memfd_create("script", 0);
  if (fd == -1) err(1, "Error creating memory file");

  while ((n = read(exe, buf, 4096)) > 0) {
    if (write(fd, buf, n) != n) err(1, "Error writing to memory file");
  }

  {
    const char *const argv[] = {NULL};
    const char *const envp[] = {NULL};
    int ret = fexecve(fd, (char *const *)argv, (char *const *)envp);
    // Memfd execution in ChromeOS should fail with EACCES.
    if (ret != -1 || errno != EACCES)
      err(2, "Unexpected return code from fexecve");
  }
  return 0;
}
