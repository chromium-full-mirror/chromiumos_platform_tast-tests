// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint/rpcdut"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FpROCanUpdateRW,
		Desc: "Verify that RO can update RW",
		Contacts: []string{
			"chromeos-fingerprint@google.com",
			"josienordrum@google.com", // Test author
			"tomhughes@chromium.org",
		},
		// ChromeOS > Platform > Services > Fingerprint
		BugComponent: "b:782045",
		Attr:         []string{"group:mainline", "group:fingerprint-cq", "group:cq-medium"},
		Timeout:      9 * time.Minute,
		SoftwareDeps: []string{"biometrics_daemon"},
		HardwareDeps: hwdep.D(hwdep.Fingerprint()),
		ServiceDeps:  []string{"tast.cros.platform.UpstartService", dutfs.ServiceName},
		Vars:         []string{"servo"},
		Fixture:      fixture.FingerprintImages,
	})
}

type testRWFlashParams struct {
	firmwarePath                string
	expectedROVersion           string
	expectedRWVersion           string
	expectedRunningFirmwareCopy fingerprint.FWImageType
}

func testFlashingRWFirmware(ctx context.Context, d *rpcdut.RPCDUT, params *testRWFlashParams) error {
	testing.ContextLog(ctx, "Flashing RW firmware: ", params.firmwarePath)
	if err := fingerprint.FlashRWFirmware(ctx, d, params.firmwarePath); err != nil {
		return errors.Wrapf(err, "failed to flash firmware: %q", params.firmwarePath)
	}

	testing.ContextLog(ctx, "Checking for versions: RO: ", params.expectedROVersion, ", RW: ", params.expectedRWVersion)
	if err := fingerprint.CheckRunningFirmwareVersionMatches(ctx, d, params.expectedROVersion, params.expectedRWVersion); err != nil {
		return errors.Wrap(err, "unexpected firmware version")
	}

	testing.ContextLog(ctx, "Checking that ", params.expectedRunningFirmwareCopy, " firmware is running")
	if err := fingerprint.CheckRunningFirmwareCopy(ctx, d.DUT(), params.expectedRunningFirmwareCopy); err != nil {
		return errors.Wrap(err, "running unexpected firmware copy")
	}
	testing.ContextLog(ctx, "Checking that rollback meets expected values")
	if err := fingerprint.CheckRollbackSetToInitialValue(ctx, d); err != nil {
		return errors.Wrap(err, "rollback not set to initial value")
	}

	return nil
}

// FpROCanUpdateRW flashes RW firmware with a version string that ends in '.rb0'
// (has rollback ID '0') and validates that it is running. Then flashes RW
// firmware with version string that ends in '.dev' (also has rollback ID '0')
// and validates that it is running.
func FpROCanUpdateRW(ctx context.Context, s *testing.State) {
	d, err := rpcdut.NewRPCDUT(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect RPCDUT: ", err)
	}
	defer d.Close(ctx)

	servoSpec, ok := s.Var("servo")
	if !ok {
		servoSpec = ""
	}

	testImages := s.FixtValue().(*fixture.ImagesTestData).TestImages

	firmwareFile := fingerprint.NewFirmwareFile(testImages[fingerprint.TestImageTypeDev].Path, fingerprint.KeyTypeDev, testImages[fingerprint.TestImageTypeDev].ROVersion, testImages[fingerprint.TestImageTypeDev].RWVersion)
	// Set both HW write protect and SW write protect true.
	t, err := fingerprint.NewFirmwareTest(ctx, d, servoSpec, s.OutDir(), firmwareFile, true /*HW protect*/, true /*SW protect*/)
	if err != nil {
		s.Fatal("Failed to create new firmware test: ", err)
	}
	cleanupCtx := ctx
	defer func() {
		if err := t.Close(cleanupCtx); err != nil {
			s.Fatal("Failed to clean up: ", err)
		}
	}()
	ctx, cancel := ctxutil.Shorten(ctx, t.CleanupTime())
	defer cancel()

	testing.ContextLog(ctx, "Flashing RW firmware with rollback ID of '0'")
	if err := testFlashingRWFirmware(ctx, d,
		&testRWFlashParams{
			firmwarePath: testImages[fingerprint.TestImageTypeDevRollbackZero].Path,
			// RO version should remain unchanged.
			expectedROVersion: testImages[fingerprint.TestImageTypeDev].ROVersion,
			// RW version should match what we requested to be flashed.
			expectedRWVersion: testImages[fingerprint.TestImageTypeDevRollbackZero].RWVersion,
			// Signature check will pass, so we should be running RW.
			expectedRunningFirmwareCopy: fingerprint.ImageTypeRW,
		}); err != nil {
		s.Fatal("Rollback ID 0 test failed: ", err)
	}
	testing.ContextLog(ctx, "Flashing RW with dev firmware")
	if err := testFlashingRWFirmware(ctx, d,
		&testRWFlashParams{
			firmwarePath: testImages[fingerprint.TestImageTypeDev].Path,
			// RO version should remain unchanged.
			expectedROVersion: testImages[fingerprint.TestImageTypeDev].ROVersion,
			// RW version should match what we requested to be flashed.
			expectedRWVersion: testImages[fingerprint.TestImageTypeDev].RWVersion,
			// Signature check will pass, so we should be running RW.
			expectedRunningFirmwareCopy: fingerprint.ImageTypeRW,
		}); err != nil {
		s.Fatal("Dev firmware test failed: ", err)
	}
}
