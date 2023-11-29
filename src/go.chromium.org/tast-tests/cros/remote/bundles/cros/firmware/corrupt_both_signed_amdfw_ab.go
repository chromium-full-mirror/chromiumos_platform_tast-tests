// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CorruptBothSignedAMDFWAB,
		Desc:         "Servo based both A and B signed AMDFW corruption test. This test requires a USB disk with ChromeOS test image plugged-in. This test corrupts both A and B SIGNED_AMDFW FMAP section. On next reboot, the firmware verification fails and enters recovery mode. This test then checks the success of the recovery boot",
		Contacts:     []string{"chromeos-faft@google.com", "kramasub@google.com"},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      50 * time.Minute,
		Vars:         []string{"firmware.skipFlashUSB"},
		SoftwareDeps: []string{"crossystem", "flashrom", "amd_cpu"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService", "tast.cros.firmware.UtilsService"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "normal_mode",
				Fixture: fixture.NormalMode,
				Val:     "normal",
			},
			{
				Name:    "dev_mode",
				Fixture: fixture.DevMode,
				Val:     "developer",
			},
		},
	})
}

func CorruptBothSignedAMDFWAB(ctx context.Context, s *testing.State) {
	corruptFWSectionTest(ctx, s, string(bios.SignedAMDFWAImageSection), string(bios.SignedAMDFWBImageSection), string(bios.SignedAMDFWAImageSection), string(bios.SignedAMDFWBImageSection), "RW firmware vendor blob verification failure")
}
