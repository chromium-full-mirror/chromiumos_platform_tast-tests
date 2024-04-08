// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECSize,
		Desc: "Compare ec flash size to expected ec size from a chip-to-size map",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_ec"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Requirements: []string{"sys-fw-0022-v02"},
		Fixture:      fixture.NormalMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// chipSizeMap is a map of chip Name(.Revision) to size of flash in KiB.
// Please keep items alphabetized.
var chipSizeMap = map[string]int{
	"it82102.ax":          1024,
	"it81202.bx":          1024,
	"it81202.cx":          1024,
	"it81202.dx":          1024,
	"it81302":             1024,
	"it81302.bx":          1024,
	"it8320.dx":           512,
	"ite_spi_ccd_i2c":     1024,
	"mec1322.81":          256,
	"NPCX586G.A.05":       512,
	"NPCX796F.A.07":       512,
	"NPCX796F.B.02":       512,
	"NPCX796F.C.02":       512,
	"NPCX797F.C.02":       512,
	"NPCX797W.C.02":       512,
	"NPCX993F.A.00160207": 512,
	"npcx9m3f.00160207":   512,
	"npcx9m7f.00160207":   1024,
	"npcx_int_spi":        512,
	"npcx_spi":            512,
	"npcx_uut":            512,
	"stm32":               256,
	"stm32f09x":           256,
}

func ECSize(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	ectool := firmware.NewECTool(h.DUT, firmware.ECToolNameMain)

	sizeInBytes, err := ectool.FlashSize(ctx)
	if err != nil {
		s.Fatal("Failed to get flashinfo from ectool: ", err)
	}
	size := sizeInBytes / 1024

	chip, err := ectool.ChipInfo(ctx)
	if err != nil {
		s.Fatalf("Failed to get ec chip, got size %d: %v", size, err)
	}

	chipID := chip.Name
	if chip.Revision != "" {
		chipID = fmt.Sprintf("%s.%s", chip.Name, chip.Revision)
	}

	s.Logf("Flash size: %d KB", size)
	s.Logf("EC Chip: %s - {name: %s, vendor: %s, revision: %s}", chipID, chip.Name, chip.Vendor, chip.Revision)

	expSize, ok := chipSizeMap[chipID]
	if !ok {
		s.Fatalf("Failed to find ec chip %v in chipSizeMap", chipID)
	}

	if expSize != size {
		s.Fatalf("Failed to verify EC size, expected size %d, got %d KB for chip %v", expSize, size, *chip)
	}
}
