// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dev

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GDBPrettyPrinter,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check GDB supports libc++ pretty printer",
		Contacts:     []string{"crostc-staff@google.com"},
		BugComponent: "b:1038090", // ChromeOS Public Tracker > Services > EngProd > Toolchain
		Attr:         []string{"group:mainline", "informational"},
	})
}

func GDBPrettyPrinter(ctx context.Context, s *testing.State) {
	// Start a debugging session simply running `gdb --version`; as GDB is linked
	// against libc++, this should cause the corresponding pretty-printer to be
	// auto-loaded, so we can verify it is actually installed and supported.
	cmd := testexec.CommandContext(
		ctx,
		"gdb",
		"-batch",
		"-ex", "run",
		"--args", "gdb", "--version",
	)
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to run GDB: ", err)
	}

	if !strings.Contains(string(out), "Loading libc++ pretty-printers.") {
		s.Errorf("Didn't load libc++ pretty-printers: %s", out)
	}
}
