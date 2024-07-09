// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dev

import (
	"context"
	"regexp"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LocalGDB,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check local GDB works properly",
		Contacts:     []string{"crostc-staff@google.com"},
		BugComponent: "b:1038090", // ChromeOS Public Tracker > Services > EngProd > Toolchain
		Attr:         []string{"group:mainline", "informational"},
	})
}

func LocalGDB(ctx context.Context, s *testing.State) {
	// This runs gdb, performing the following actions:
	// * load `coreutils` with the right arguments for running `ls`
	// * set a breakpoint on `__libc_start_main()`: this is an internal libc function which
	//   is always present, even though the software doesn't include debug symbols
	// * start execution
	// * print a backtrace upon reaching the breakpoint
	// * continue execution until the program exits
	// gdb should then exit normally, or error out during execution if one of those actions fail
	cmd := testexec.CommandContext(
		ctx,
		"gdb",
		"-batch",
		"-ex", "set breakpoint pending on",
		"-ex", "break __libc_start_main",
		"-ex", "run",
		"-ex", "backtrace",
		"-ex", "continue",
		"--args", "/usr/bin/coreutils", "--coreutils-prog=ls",
	)
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to run GDB: ", err)
	}

	// Check gdb output for the indication that the `break` command succeeded;
	// depending on the target this can be either `Breakpoint 1 (__libc_start_main) pending.`
	// or `Breakpoint 1 at 0x...`, so we must account for both cases.
	if re := regexp.MustCompile(`Breakpoint \d+ (\(.*\) pending.|at 0x[\da-f]+)`); !re.Match(out) {
		s.Errorf("Unable to set breakpoint: %s", out)
	}

	// Ensure the breakpoint was reached
	if re := regexp.MustCompile(`Breakpoint \d+, __libc_start_main_impl \(.*\) at .*:\d+`); !re.Match(out) {
		s.Errorf("GDB didn't break on __libc_start_main(): %s", out)
	}

	// Check gdb output for a valid backtrace
	if re := regexp.MustCompile(`#0  __libc_start_main_impl \(.*\) at .*:\d+`); !re.Match(out) {
		s.Errorf("Backtrace not found in gdb output: %s", out)
	}

	// Ensure the debugged program exited without any error
	if re := regexp.MustCompile(`\[Inferior \d+ \(process \d+\) exited normally\]`); !re.Match(out) {
		s.Errorf("Debugged process didn't exit normally: %s", out)
	}
}
