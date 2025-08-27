// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runtimeprobe

import (
	"context"
	"os"
	"path/filepath"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/runtimeprobe/utils"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RuntimeHWIDRebootDelete,
		Desc: "Test Runtime HWID should be deleted if it exists after rebooting DUT",
		Contacts: []string{
			"chromeos-runtime-probe@google.com",
			"allenshihmc@google.com",
		},
		BugComponent: "b:606088",
		SoftwareDeps: []string{"reboot", "racc"},
		HardwareDeps: hwdep.D(hwdep.RuntimeProbeConfig(), hwdep.RuntimeProbeConfigPrivate(false)),
		Attr:         []string{"group:racc", "racc_config_installed"},
		Fixture:      fixture.CleanupRuntimeHWID,
	})
}

// RuntimeHWIDRebootDelete checks that the Runtime HWID file is deleted after rebooting DUT.
func RuntimeHWIDRebootDelete(ctx context.Context, s *testing.State) {
	const (
		runtimeHWIDFileDir  = "/var/cache/hardware_verifier"
		runtimeHWIDFileName = "runtime_hwid"
		fakeHWIDContent     = "FAKE_RUNTIME_HWID"
	)

	d := s.DUT()

	tmpDir, err := os.MkdirTemp("", "tast.runtimeprobe.RuntimeHWIDRebootDelete.")
	if err != nil {
		s.Fatal("Failed to create temp dir: ", err)
	}
	defer os.RemoveAll(tmpDir)

	localFilePath := filepath.Join(tmpDir, runtimeHWIDFileName)
	if err := os.WriteFile(localFilePath, []byte(fakeHWIDContent), 0644); err != nil {
		s.Fatalf("Failed to write to temp file %q: %v", localFilePath, err)
	}

	// Puts the fake Runtime HWID file to DUT.
	runtimeHWIDFilePath := filepath.Join(runtimeHWIDFileDir, runtimeHWIDFileName)
	if _, err := linuxssh.PutFiles(ctx, d.Conn(), map[string]string{localFilePath: runtimeHWIDFilePath}, linuxssh.DereferenceSymlinks); err != nil {
		s.Fatalf("Failed to put file to %q: %v", runtimeHWIDFilePath, err)
	}

	s.Log("Reboot to trigger a Runtime HWID check in hardware_verifier")
	if err := d.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	if err := utils.WaitServiceState(ctx, d, "hardware_verifier", "stop/waiting"); err != nil {
		s.Fatal("Service hardware_verifier timed out: ", err)
	}

	// Checks if the file is deleted.
	if err := d.Conn().CommandContext(ctx, "test", "-f", runtimeHWIDFilePath).Run(); err == nil {
		s.Fatalf("File %q still exists after reboot", runtimeHWIDFilePath)
	}
}
