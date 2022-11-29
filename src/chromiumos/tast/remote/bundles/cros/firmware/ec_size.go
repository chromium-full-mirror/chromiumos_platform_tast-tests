// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"chromiumos/tast/remote/firmware"
	"chromiumos/tast/remote/firmware/fixture"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
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
		Fixture:      fixture.NormalMode,
	})
}

// chipSizeMap is a map of chipName to size of flash in KiB. Please keep items alphabetized.
var chipSizeMap = map[string]int{
	"it81302":         1024,
	"it8320":          512,
	"ite_spi_ccd_i2c": 1024,
	"mec1322":         512,
	"npcx_int_spi":    512,
	"npcx_spi":        512,
	"npcx_uut":        512,
	"stm32":           256,
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
		s.Fatal("Failed to get ec chip: ", err)
	}

	s.Logf("Flash size: %d KB", size)
	s.Logf("EC Chip name: %s, vendor: %s, revision: %s", chip.Name, chip.Vendor, chip.Revision)

	expSize, ok := chipSizeMap[chip.Name]
	if !ok {
		s.Fatalf("Failed to find ec chip %v in chipSizeMap", chip.Name)
	}

	if expSize != size {
		s.Fatalf("Failed to verify EC size, expected size %d, got %d KB for chip %v", expSize, size, chip)
	}
}
