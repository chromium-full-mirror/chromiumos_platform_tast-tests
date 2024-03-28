// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/firmware/fwupd"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FwupdInstallRemoteCLI,
		Desc: "Checks that fwupd can install using a remote repository",
		// ChromeOS > Platform > Services > Peripherals > Firmware Update - fwupd
		BugComponent: "b:857851",
		Contacts: []string{
			"chromeos-fwupd@google.com", // CrOS FWUPD
			"rishabhagr@chromium.org",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"fwupd"},
		HardwareDeps: hwdep.D(
			hwdep.Battery(),  // Test doesn't run on ChromeOS devices without a battery.
			hwdep.ChromeEC(), // Test requires Chrome EC to set battery to charge via ectool.
		),
		Timeout:      fwupd.ChargingStateTimeout + 1*time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// FwupdInstallRemoteCLI runs the fwupdtool utility in CLI (command line interface) and verifies that it
// can update a device in the system using a remote repository.
func FwupdInstallRemoteCLI(ctx context.Context, s *testing.State) {
	// make sure dut battery is charging/charged
	if cleanup, err := fwupd.SetFwupdChargingState(ctx, true); err != nil {
		s.Fatal("Failed to set charging state: ", err)
	} else {
		defer func() {
			if err := cleanup(ctx); err != nil {
				s.Fatal("Failed to cleanup: ", err)
			}
		}()
	}

	cmd := testexec.CommandContext(ctx, "/usr/bin/fwupdmgr", "install", "--allow-reinstall", "-v", fwupd.ReleaseURI)
	cmd.Env = append(os.Environ(), "CACHE_DIRECTORY=/var/cache/fwupd")
	output, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		s.Errorf("%q failed: %v", cmd.Args, err)
	}
	if err := os.WriteFile(filepath.Join(s.OutDir(), "fwupdmgr.txt"), output, 0644); err != nil {
		s.Error("Failed to write output from update: ", err)
	}
}
