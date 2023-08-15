// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

// File opener utility to assist in Landlock tests.
int main(int argc, char *argv[]) {
  if (argc != 3) {
    fprintf(stderr, "Usage: %s <filename> <mode>\n", argv[0] );
    return 1;
  }
  FILE *file = fopen(argv[1],  argv[2]);
  if (file == 0) {
    return errno;
  }

  return 0;
}
