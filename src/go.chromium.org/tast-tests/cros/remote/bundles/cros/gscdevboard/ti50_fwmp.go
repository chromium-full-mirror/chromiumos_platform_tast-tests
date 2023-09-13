// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"github.com/google/go-tpm/legacy/tpm2"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"

	"go.chromium.org/tast/core/testing"
)

const (
	// fwmpDisableDevMode disabled developer mode
	fwmpDisableDevMode = 1 << 0
	// fwmpDisableUnlock disabled ccd unlock and other functionality on GSC
	fwmpDisableUnlock = 1 << 6
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    Ti50Fwmp,
		Desc:    "Verifies various FWMP enforcement for GSC",
		Timeout: 30 * time.Second,
		Contacts: []string{
			"chromeos-faft@google.com", // CrOS Firmware Developers
			"jettrink@chromium.org",    // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr:         []string{"group:gsc", "gsc_dt_ab", "gsc_image_ti50", "gsc_nightly"},
		Fixture:      fixture.Ti50CcdOpen,
	})
}

func Ti50Fwmp(ctx context.Context, s *testing.State) {
	f := s.FixtValue().(*fixture.Value)
	b := utils.NewDevboardHelper(f.DevBoard(), s)
	i := ti50.NewCrOSImage(b)

	tpm := b.ResetAndTpmStartup(ctx, i, ti50.TpmBusSpi, ti50.CcdSuzyQ, ti50.FfClamshell)

	// Need to wait more than 3 seconds for ti50's delay to commit to NvMem after AP turns on
	s.Log("Wait 5 second for NvWrites to actually commit to flash")
	testing.Sleep(ctx, 5*time.Second) // GoBigSleepLint: No way to poll for this state

	// Undefine the space to ensure we are in a good state. Not a failure if doesn't work
	tpm2.NVUndefineSpace(tpm, ti50.EmptyPassword, ti50.RootPlatformHandle, ti50.FwmpFileID)

	verifyWpDisabledWithFwmp(ctx, s, b, i, tpm)
}

func verifyWpDisabledWithFwmp(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage, tpm *utils.TpmHelper) {
	s.Log("Verify WP works before FWMP unlock disable")
	_, err := i.Command(ctx, "wp disable atboot")
	if err != nil {
		s.Fatal("Error communicating with ti50: ", err)
	}

	// Ensure write protect is disabled
	if b.GpioGet(ctx, ti50.GpioTi50WriteProtectL) != true {
		s.Fatal("WP signal not disable after `wp disable` console command")
	}

	s.Log("Write FWMP file with unlock disabled")
	fwmpFile := makeFWMPFile(fwmpDisableUnlock)
	// Define space in NV storage and clean up afterwards
	if err := tpm2.NVDefineSpace(tpm,
		ti50.RootPlatformHandle,
		ti50.FwmpFileID,
		ti50.EmptyPassword,
		ti50.EmptyPassword,
		nil,
		ti50.FwmpFileAttr,
		uint16(len(fwmpFile)),
	); err != nil {
		s.Fatal("NVDefineSpace failed: ", err)
	}
	defer tpm2.NVUndefineSpace(tpm, ti50.EmptyPassword, ti50.RootPlatformHandle, ti50.FwmpFileID)

	// Write the fwmp file data to new space.
	if err := tpm2.NVWrite(tpm,
		ti50.RootPlatformHandle,
		ti50.FwmpFileID,
		ti50.EmptyPassword,
		fwmpFile,
		0,
	); err != nil {
		s.Fatal("NVWrite failed: ", err)
	}

	// Ensure Write protect is disabled
	if b.GpioGet(ctx, ti50.GpioTi50WriteProtectL) != false {
		s.Fatal("WP signal not enabled after policy gets written to NVmem with FWMP unlocked disabled")
	}

	// Type the "wp disable" command again; this should have no affect because of FWMP
	_, err = i.Command(ctx, "wp disable atboot")
	if err != nil {
		s.Fatal("Error communicating with ti50: ", err)
	}

	// Ensure Write protect is still enabled since wp command should have been blocked
	if b.GpioGet(ctx, ti50.GpioTi50WriteProtectL) != false {
		s.Fatal("WP signal not enabled after `wp disable` command, but should be blocked")
	}
}

// makeFWMPFile create the 40 bytes FWMP file with the specified flags
func makeFWMPFile(flags uint32) []byte {
	out := make([]byte, 40)
	out[0] = 0                   // crc -- filled in later
	out[1] = 40                  // size
	out[2] = 0x10                // version 1.0
	out[3] = 0x00                // reserved
	out[4] = byte(flags >> 0)    // flags (1 of 4)
	out[5] = byte(flags >> 8)    // flags (2 of 4)
	out[6] = byte(flags >> 16)   // flags (3 of 4)
	out[7] = byte(flags >> 24)   // flags (4 of 4)
	out[0] = utils.Crc8(out[2:]) // crc
	return out
}
