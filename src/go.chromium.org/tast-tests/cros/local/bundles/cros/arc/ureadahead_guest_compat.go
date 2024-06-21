// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UreadaheadGuestCompat,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Validates that ureadahead binary from CrOS host can work in the guest ARCVM OS",
		Contacts: []string{
			"arc-performance@google.com",
			"alanding@google.com",
			"khmel@google.com",
		},
		// ChromeOS > Software > ARC++ > Performance
		BugComponent: "b:168382",
		Attr:         []string{"group:mainline", "group:arc-functional"},
		SoftwareDeps: []string{"chrome", "no_arc_userdebug"},
		Params: []testing.Param{{
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		Fixture: "arcBootedWithDisableExternalStorage",
		Timeout: 10 * time.Minute,
	})
}

func UreadaheadGuestCompat(ctx context.Context, s *testing.State) {
	const (
		logName = "readahead.log"
		// Must use BootstrapCommand since exec-cros-binary is not accessible by adb.
		execCrosBinaryPath = "/system/bin/exec-cros-binary"
		crosUreadaheadPath = "/var/run/arc/ro/sbin/ureadahead"
		catCmd             = "/system/bin/cat"
		fontsPath          = "/system/etc/fonts.xml"
	)

	outDir := s.OutDir()
	logPath := filepath.Join(outDir, logName)
	logFile, err := os.Create(logPath)
	if err != nil {
		s.Fatal("Failed to create log file: ", err)
	}
	defer logFile.Close()

	// Must enable trace events first or ureadahead generate will not run (exit early without tracing).
	if err := enableTraceEvents(ctx, true /*enable*/); err != nil {
		s.Fatal("Failed to enable trace events")
	}

	// Verify generate mode can run to completion with timeout since otherwise ureadahead process doesn't complete.
	generateCmd := arc.BootstrapCommand(ctx, execCrosBinaryPath, crosUreadaheadPath, "--force-trace", "--use-existing-trace-events",
		"--force-ssd-mode", "--verbose", "--timeout=5")
	stdoutPipe, err := generateCmd.StdoutPipe()
	if err != nil {
		s.Fatal("Failed to get stdout pipe to generate command: ", err)
	}
	// Run fs trace in the background and explicitly read a file in /system to make sure there is something to trace.
	if err := generateCmd.Start(); err != nil {
		s.Fatal("Failed to start CrOS ureadahead trace in guest OS: ", err)
	}
	stdout := bufio.NewReader(stdoutPipe)
	// Wait for some output to prevent race condition with concurrent catCmd.
	if outStr, err := stdout.ReadString('\n'); err != nil {
		s.Fatal("Failed to read output from generate command: ", err)
	} else if len(strings.TrimSpace(outStr)) == 0 {
		s.Fatalf("Failed to confirm valid non-whitespace output, got %q: %v", outStr, err)
	}
	// Run cat to explicitly trigger a trace event.
	if err := arc.BootstrapCommand(ctx, catCmd, fontsPath).Run(testexec.DumpLogOnError); err != nil {
		s.Fatalf("Failed to read from path %q: %v", fontsPath, err)
	}
	// Synchronize before resetting trace events.
	if err := generateCmd.Wait(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed waiting for CrOS ureadahead trace to complete: ", err)
	}
	if err := enableTraceEvents(ctx, false /*enable*/); err != nil {
		s.Fatal("Failed to reset trace events: ", err)
	}
}

// enableTraceEvents enables or resets kernel fs trace events.
// TODO(b313691860): Remove once we no longer need to enable trace events external to ureadahead process.
func enableTraceEvents(ctx context.Context, enable bool) error {
	const (
		echoCmd     = "/system/bin/echo"
		tracingRoot = "/sys/kernel/tracing"
	)

	var enableStr string
	if enable {
		enableStr = "1"
	} else {
		enableStr = "0"
	}
	doSysOpen := filepath.Join(tracingRoot, "events/fs/do_sys_open/enable")
	if err := arc.BootstrapCommand(ctx, echoCmd, enableStr, ">", doSysOpen).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to write to path: %s", doSysOpen)
	}
	openExec := filepath.Join(tracingRoot, "events/fs/open_exec/enable")
	if err := arc.BootstrapCommand(ctx, echoCmd, enableStr, ">", openExec).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to write to path: %s", openExec)
	}
	return nil
}
