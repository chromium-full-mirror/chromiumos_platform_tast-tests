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

// This program accepts one argument, which is a path to an executable. It will
// attempt to create a memfd, copy the executable to it and execute it.
// Return code 0 means the execution of memfd was blocked as expected, error
// code 2 indicates that the memfd execution was not blocked, and error code 1
// indicates an error in the auxiliary operations.
int main(int argc, char *argv[]) {
  int n;
  char buf[4096];

  if (argc != 2) err(1, "Please supply exactly one argument.\n");

  int exe = open(argv[1], O_RDONLY);
  if (exe == -1) {
    err(1, "Error opening the executable.\n");
  }

  int fd = memfd_create("script", 0);
  if (fd == -1) err(1, "%s failed", "memfd_create");

  while ((n = read(exe, buf, 4096)) > 0) {
    if (write(fd, buf, n) != n) err(1, "Error writing to file.\n");
  }

  {
    const char *const argv[] = {NULL};
    const char *const envp[] = {NULL};
    int ret = fexecve(fd, (char *const *)argv, (char *const *)envp);
    // Memfd execution in ChromeOS should fail with EACCES.
    if (ret != -1 || errno != EACCES) err(2, "%s not blocked", "memfd_fexecve");
  }
  return 0;
}
