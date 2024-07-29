// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lldb

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LocalSetBreakpoint,
		Desc:         "Sets a breakpoint in a program to verify lldb is working",
		Contacts:     []string{"c-compiler-chrome@google.com", "ajordanr@google.com"},
		BugComponent: "b:1038090",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"lldb"},
		Timeout:      time.Minute,
		Data:         []string{"set_breakpoint_lldb_commands"},
	})
}

func LocalSetBreakpoint(ctx context.Context, s *testing.State) {
	commandsPath := s.DataPath("set_breakpoint_lldb_commands")

	lldbCmd := testexec.CommandContext(
		ctx,
		"/usr/local/bin/lldb",
		"-s", // run the commands from the file upon loading the program
		commandsPath,
		"--no-lldbinit",
		"--batch",
		"--",
		// We use coreutils here because it is a simple program that is
		// guaranteed to be installed on all ChromeOS devices.
		"/usr/bin/coreutils",
		"--coreutils-prog=ls",
	)
	out, err := lldbCmd.CombinedOutput(testexec.DumpLogOnError)
	if err != nil {
		s.Errorf("executing lldb failed with output %q: %v", out, err)
	}
	s.Logf("LLDB output: %q", out)
	lldbOutput := string(out[:])
	if !(strings.Contains(lldbOutput, "Breakpoint 1") &&
		strings.Contains(lldbOutput, "stop reason = breakpoint")) {
		s.Errorf("lldb failed to set a breakpoint with output: %q", out)
	}
}
