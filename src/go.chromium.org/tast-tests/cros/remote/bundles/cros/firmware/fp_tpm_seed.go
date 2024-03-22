// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint/rpcdut"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// Path to the file where TPM seed is temporarily stored.
const fingerprintTPMSeedFile = "/run/bio_crypto_init/seed"

func init() {
	testing.AddTest(&testing.Test{
		Func: FpTpmSeed,
		Desc: "Check using ectool if bio_crypto_init set the TPM seed",
		Contacts: []string{
			"chromeos-fingerprint@google.com",
			"tomhughes@chromium.org",
		},
		// ChromeOS > Platform > Services > Fingerprint
		BugComponent: "b:782045",
		Attr:         []string{"group:mainline", "group:fingerprint-cq", "group:cq-medium", "group:fingerprint-release"},
		SoftwareDeps: []string{"biometrics_daemon"},
		HardwareDeps: hwdep.D(hwdep.Fingerprint()),
		ServiceDeps:  []string{"tast.cros.platform.UpstartService", dutfs.ServiceName},
		Vars:         []string{"servo"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func FpTpmSeed(ctx context.Context, s *testing.State) {
	d, err := rpcdut.NewRPCDUT(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect RPCDUT: ", err)
	}
	defer d.Close(ctx)

	servoSpec, ok := s.Var("servo")
	if !ok {
		servoSpec = ""
	}
	pxy, err := servo.NewProxy(ctx, servoSpec, d.KeyFile(), d.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(ctx)

	fpBoard, err := fingerprint.Board(ctx, d)
	if err != nil {
		s.Fatal("Failed to get fingerprint board: ", err)
	}

	needsReboot, err := fingerprint.NeedsRebootAfterFlashing(ctx, d)
	if err != nil {
		s.Fatal("Failed to determine whether reboot is needed: ", err)
	}
	firmwareFile, err := fingerprint.NewMPFirmwareFile(ctx, d)
	if err != nil {
		s.Fatal("Failed to create MP firmwareFile: ", err)
	}
	removeSWWP := false
	if err := fingerprint.InitializeKnownState(ctx, d, s.OutDir(), pxy,
		fpBoard, *firmwareFile, needsReboot, removeSWWP); err != nil {
		s.Fatal("Initialization failed: ", err)
	}

	// Reboot DUT to make sure that there was an opportunity to initialize
	// FPMCU with TPM seed.
	if err := d.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	// The seed is only set after bio_crypto_init runs. The boot-services
	// service is blocked until bio_crypto_init finishes.
	// The system-services starts after boot-services, then failsafe and
	// finally openssh-server. As a result if SSH server is running, then
	// we are sure that bio_crypto_init initialized TPM seed.

	// Check if the TPM seed file does not exist. It is created by
	// cryptohome and supposed to be removed by bio_fw_updater. The file
	// contains TPM seed passed to FPMCU. If the file exists, it will be
	// a security issue.
	dutfsClient := dutfs.NewClient(d.RPC().Conn)
	exists, err := dutfsClient.Exists(ctx, fingerprintTPMSeedFile)
	if err != nil {
		s.Fatal(err, "Error checking that TPM seed file exists: ", err)
	}
	if exists {
		s.Errorf("File with TPM seed (%q) exists", fingerprintTPMSeedFile)
	}

	// Check if FPMCU was initialized with TPM seed.
	testing.ContextLog(ctx, "Validating that FPMCU was initialized with TPM seed")
	e, err := fingerprint.GetEncryptionStatus(ctx, d.DUT())
	if err != nil {
		s.Fatal("Failed to get encryption status: ", err)
	}
	testing.ContextLogf(ctx, "FPMCU encryption engine status: %#08x", e.Current)
	if e.TPMSeedSet() {
		testing.ContextLog(ctx, "TPM seed is set")
	} else {
		s.Error("TPM seed is not set")
	}
}
