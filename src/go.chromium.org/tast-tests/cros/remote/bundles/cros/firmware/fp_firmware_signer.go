// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint/rpcdut"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FpFirmwareSigner,
		Desc: "Validate that FPMCU firmware in rootfs signed with MP key",
		Contacts: []string{
			"chromeos-fingerprint@google.com",
			"josienordrum@google.com",
		},
		// ChromeOS > Platform > Services > Fingerprint
		BugComponent: "b:782045",
		Attr:         []string{"group:fingerprint-cq", "group:fingerprint-informational"},
		Timeout:      2 * time.Minute,
		SoftwareDeps: []string{"biometrics_daemon"},
		HardwareDeps: hwdep.D(hwdep.Fingerprint()),
	})
}

// FpFirmwareSigner checks if FPMCU firmware in rootfs signed with MP key.
func FpFirmwareSigner(ctx context.Context, s *testing.State) {
	d, err := rpcdut.NewRPCDUT(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect RPCDUT: ", err)
	}
	defer d.Close(ctx)

	fpBoard, err := fingerprint.Board(ctx, d)
	if err != nil {
		s.Fatal("Failed to get fp board: ", err)
	}
	firmwareFile, err := fingerprint.NewMPFirmwareFile(ctx, d)
	if err != nil {
		s.Fatal("Failed to create MP firmwareFile: ", err)
	}
	allowedKeys := []fingerprint.KeyType{fingerprint.KeyTypeMp}
	fingerprint.ValidateBuildFwFile(ctx, d, fpBoard, firmwareFile.FilePath, allowedKeys)
	s.Log("FPMCU firmware in rootfs signed with MP key")
}
