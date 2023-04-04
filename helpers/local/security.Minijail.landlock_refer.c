// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

#include <linux/types.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

//  Landlock defs, because <linux/landlock.h> may not be available.

#ifndef LANDLOCK_ACCESS_FS_REFER
#define LANDLOCK_ACCESS_FS_REFER (1ULL << 13)
#endif

#ifndef __NR_landlock_create_ruleset
#define __NR_landlock_create_ruleset 444
#endif

/**
 * struct landlock_ruleset_attr - Ruleset definition
 *
 * Argument of sys_landlock_create_ruleset().
 */
struct landlock_ruleset_attr {
  __u64 handled_access_fs;
};

int landlock_create_ruleset(const struct landlock_ruleset_attr *const attr,
      const size_t size, const __u32 flags) {
  return syscall(__NR_landlock_create_ruleset, attr, size, flags);
}

// Test helper for using LANDLOCK_ACCESS_FS_REFER.
int main(int argc, char *argv[]) {
  int ruleset_fd;
  struct landlock_ruleset_attr ruleset_attr = {
    .handled_access_fs = LANDLOCK_ACCESS_FS_REFER,
  };

  // Attempting to create a ruleset with LANDLOCK_ACCESS_FS_REFER
  // will result in an error if it’s not supported.
  ruleset_fd =
    landlock_create_ruleset(&ruleset_attr, sizeof(ruleset_attr), 0);
 if (ruleset_fd >= 0) {
    return 0;
  } else {
    fprintf(stderr, "Unexpected ruleset_fd: %i \n", ruleset_fd);
    return -1;
  }
}
