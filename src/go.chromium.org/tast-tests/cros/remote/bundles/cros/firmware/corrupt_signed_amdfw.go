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
		Func:         CorruptSignedAMDFW,
		Desc:         "Servo based Signed AMDFW section corruption test",
		Contacts:     []string{"chromeos-faft@google.com", "kramasub@google.com"},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		Timeout:      20 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		SoftwareDeps: []string{"crossystem", "flashrom", "amd_cpu"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService", "tast.cros.firmware.UtilsService"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "normal_mode",
				Fixture: fixture.NormalMode,
				Val: &corruptSingleSectionVals{
					bios.SignedAMDFWAImageSection, bios.SignedAMDFWBImageSection, bios.SignedAMDFWAImageSection, bios.SignedAMDFWBImageSection,
				},
			},
			{
				Name:    "dev_mode",
				Fixture: fixture.DevModeGBB,
				Val: &corruptSingleSectionVals{
					bios.SignedAMDFWAImageSection, bios.SignedAMDFWBImageSection, bios.SignedAMDFWAImageSection, bios.SignedAMDFWBImageSection,
				},
			},
		},
	})
}

func CorruptSignedAMDFW(ctx context.Context, s *testing.State) {
	CorruptFW(ctx, s)
}
