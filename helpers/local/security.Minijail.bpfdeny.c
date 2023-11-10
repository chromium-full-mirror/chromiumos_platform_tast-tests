// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

#include <errno.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <asm/unistd.h>
#include <linux/bpf.h>

void usage(const char *comm) {
  fprintf(stderr, "Usage: %s [expect-syscall-error]\n", comm);
  return;
}

// Test helper for executing BPF syscall.
int main(int argc, char *argv[]) {
  union bpf_attr attr;
  int bpf_fd;
  int saved_errno;
  bool expect_blocked = true;
  if (argc == 2) {
     if (!strcmp(argv[1], "expect-syscall-error")) {
       expect_blocked = false;
     } else {
        usage(argv[0]);
        return 1;
     }
  } else if (argc > 2) {
     usage(argv[0]);
     return 1;
  }
  memset(&attr, 0, sizeof(attr));
  attr.prog_type = BPF_PROG_TYPE_TRACEPOINT;
  errno = 0;
  bpf_fd = syscall(__NR_bpf, BPF_PROG_LOAD, &attr, sizeof(attr));
  saved_errno = errno;

  // If Minijail is blocking BPF, we should see EPERM. Otherwise, we should
  // see either EACCES, EFAULT, ENOSYS, or E2BIG based on the board being
  // tested.
  if ((expect_blocked && saved_errno != EPERM)
      || (!expect_blocked && saved_errno != EACCES && saved_errno != EFAULT
              && saved_errno != ENOSYS && saved_errno != E2BIG)) {
    fprintf(stderr,
                "Unexpected BPF errno. bpf_fd: %i errno: %d\n",
                bpf_fd, saved_errno);
        return saved_errno;
  }

  return 0;
}
