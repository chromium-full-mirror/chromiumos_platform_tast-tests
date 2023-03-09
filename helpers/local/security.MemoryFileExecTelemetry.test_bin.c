// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

#include <err.h>

// The purpose of this program is to be passed as an argument to the memfd
// execution test program: `security.MemoryFileExecTelemetry.memfd_fexecve.c`.
// `memfd_fexecve` will attempt to run this program in a memfd, which is
// expected to be blocked. If not blocked, this program will execute and will
// return error code 2 to indicate to the caller (Tast test) that the memfd
// execution was not blocked.
int main(int argc, char *argv[]) { err(2, "Test binary ran successfully"); }
