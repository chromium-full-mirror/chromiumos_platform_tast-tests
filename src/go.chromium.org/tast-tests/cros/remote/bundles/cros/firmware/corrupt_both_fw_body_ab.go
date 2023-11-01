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
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CorruptBothFWBodyAB,
		Desc: "Corrupt both copies of AP firmware, verify broken screen with reason 0x1b, restore backup via servo",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware"},
		Timeout:      25 * time.Minute,
		SoftwareDeps: []string{"crossystem", "flashrom"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:              "normal_mode",
				Fixture:           fixture.NormalMode,
				Val:               "normal",
				ExtraAttr:         []string{"firmware_bios"},
				ExtraRequirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
			},
		},
	})
}

func CorruptBothFWBodyAB(ctx context.Context, s *testing.State) {
	corruptFWSectionTest(ctx, s, string(bios.FWBodyAImageSection), string(bios.FWBodyBImageSection))
}
