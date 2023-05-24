// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/flashrom"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FWROMSize,
		Desc: "Check that flash sizes are within reasonable range",
		Contacts: []string{
			"chromeos-faft@google.com",
			"js@semihalf.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Fixture:      fixture.NormalMode,
	})
}

// FWROMSize simply checks if AP and EC chips are above their
// minimum required values to prevent improper readings w/ flashrom
func FWROMSize(ctx context.Context, s *testing.State) {

	const (
		minECSize = 512
		minAPSize = 4096
	)

	h := s.FixtValue().(*fixture.Value).Helper

	var flashromConfig flashrom.Config
	flashromHost, ctx, shutdown, _, err := flashromConfig.
		FlashromInit(flashrom.VerbosityInfo).
		ProgrammerInit(flashrom.ProgrammerHost, "").
		SetDut(h.DUT).
		Probe(ctx)
	defer func() {
		if err := shutdown(); err != nil {
			s.Error("Failed to shutdown flashromInstance: ", err)
		}
	}()
	if err != nil {
		s.Fatal("Flashrom probe failed, unable to build host flashrom instance: ", err)
	}

	apSize, _, err := flashromHost.Size(ctx)
	if err != nil {
		s.Fatal("Failed to determine AP firmware size: ", err)
	}

	flashromEc, ctx, shutdown, _, err := flashromConfig.
		FlashromInit(flashrom.VerbosityInfo).
		ProgrammerInit(flashrom.ProgrammerEc, "").
		SetDut(h.DUT).
		Probe(ctx)
	defer func() {
		if err := shutdown(); err != nil {
			s.Error("Failed to shutdown flashromInstance: ", err)
		}
	}()
	if err != nil {
		s.Fatal("Flashrom probe failed, unable to build ec flashrom instance: ", err)
	}

	ecSize, _, err := flashromEc.Size(ctx)
	if err != nil {
		s.Fatal("Failed to determine EC firmware size: ", err)
	}

	s.Log("AP firmware size in kilobytes: ", apSize/1024)
	s.Log("EC firmware size in kilobytes: ", ecSize/1024)
	if (apSize / 1024) < minAPSize {
		s.Fatalf("AP firmware size is less than expected: want >-%d KB, got %d KB", minAPSize, apSize/1024)
	}

	if (ecSize / 1024) < minECSize {
		s.Fatalf("AP firmware size is less than expected: want >-%d KB, got %d KB", minECSize, ecSize/1024)
	}
}
