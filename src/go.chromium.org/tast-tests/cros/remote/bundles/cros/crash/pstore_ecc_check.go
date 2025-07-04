// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"
	"regexp"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/testexec"
	crashservice "go.chromium.org/tast-tests/cros/services/cros/crash"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PstoreECCCheck,
		Desc:         "Triggers a kernel crash to verify that the ECC status is correctly reported in the resulting pstore/ramoops log",
		Contacts:     []string{"chromeos-platform-stability-team@google.com", "naoyatezuka@google.com"},
		BugComponent: "b:1672909",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"reboot", "pstore"},
		// TODO(b/432640575): Remove this dependency after ECC is enabled on ARM boards.
		HardwareDeps: hwdep.D(hwdep.X86()),
		ServiceDeps:  []string{"tast.cros.crash.FixtureService"},
		Timeout:      5 * time.Minute,
	})
}

// PstoreECCCheck triggers a kernel crash to verify that the ECC status is
// correctly reported in the resulting pstore/ramoops log.
func PstoreECCCheck(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	const systemCrashDir = "/var/spool/crash"

	d := s.DUT()

	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer func() {
		if cl != nil {
			cl.Close(cleanupCtx)
		}
	}()

	fs := crashservice.NewFixtureServiceClient(cl.Conn)
	if _, err := fs.SetUp(ctx, &crashservice.SetUpCrashTestRequest{}); err != nil {
		s.Fatal("Failed to set up crash test: ", err)
	}

	// This is a bit delicate. If the test fails _before_ we panic the machine,
	// we need to do TearDown then, and on the same connection (so we can close Chrome).
	//
	// If it fails to reconnect, we do not need to clean these up.
	//
	// Otherwise, we need to run TearDown on the re-established connection to the machine.
	defer func() {
		s.Log("Cleaning up")
		if fs != nil {
			if _, err := fs.TearDown(cleanupCtx, &empty.Empty{}); err != nil {
				s.Error("Failed to tear down: ", err)
			}
		}
	}()

	// Force all cached file system data to be written to disk.
	// Otherwise kernel panic might discard this data, resulting in unexpected file corruption
	// and cascading test failures after reboot.
	if err := d.Conn().CommandContext(ctx, "sync").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to invoke sync: ", err)
	}

	s.Log("Triggering kernel crash and waiting for the DUT to reboot")
	if err := d.RebootWithCommand(ctx, "sh", "-c", "echo PANIC > /sys/kernel/debug/provoke-crash/DIRECT"); err != nil {
		s.Fatal("Failed to trigger kernel crash: ", err)
	}

	// When we lost the connection, these connections broke.
	cl.Close(ctx)
	cl = nil
	fs = nil

	cl, err = rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	fs = crashservice.NewFixtureServiceClient(cl.Conn)

	const base = `kernel\.\d{8}\.\d{6}\.\d+\.0`
	waitReq := &crashservice.WaitForCrashFilesRequest{
		Dirs:    []string{systemCrashDir},
		Regexes: []string{base + `\.kcrash`},
	}
	s.Log("Waiting for files to become present")
	res, err := fs.WaitForCrashFiles(ctx, waitReq)
	if err != nil {
		s.Fatal("Failed to find crash files: ", err)
	}

	// We expect exactly one crash and that there is exactly one .kcrash file from our single panic command.
	if len(res.Matches) != 1 {
		s.Fatalf("Wrong number of crash file match groups: got %d, want 1", len(res.Matches))
	}
	match := res.Matches[0]
	if len(match.Files) != 1 {
		s.Fatalf("Wrong number of kcrash files: got %d, want 1", len(match.Files))
	}
	kcrashFile := match.Files[0]

	s.Log("Checking for ECC status in ", kcrashFile)
	lastLine, err := d.Conn().CommandContext(ctx, "tail", "-n", "1", kcrashFile).Output()
	if err != nil {
		s.Fatalf("Failed to read last line of %s: %v", kcrashFile, err)
	}
	s.Logf("The last line of %s: %q", kcrashFile, lastLine)

	noErrorsRegex := regexp.MustCompile(`No errors detected`)
	correctedErrorsRegex := regexp.MustCompile(`\d+ Corrected bytes, \d+ unrecoverable blocks`)
	// Here we focus on whether the ECC feature is enabled,
	// and don't care if ECC can correct corruption in the ramoops region completely.
	if noErrorsRegex.Match(lastLine) {
		s.Log("ECC is enabled and no errors were detected")
	} else if correctedErrorsRegex.Match(lastLine) {
		s.Log("ECC is enabled and errors were detected")
	} else {
		s.Errorf("ECC status not found in the last line of %s", kcrashFile)
	}
}
