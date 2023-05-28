// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strconv"

	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECADC,
		Desc: "Basic check for EC ADC temperature",
		Contacts: []string{
			"chromeos-faft@google.com",
			"js@semihalf.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO(b/194908031): Add back to firmware_unstable and firmware_bringup once this test actually works.
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		Attr:         []string{},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
	})
}

// ECADC mesaures the EC internal temperature sensors in a loop for
// couple of retries. This test might fail on boards which don't have
// "temps" EC command available.
func ECADC(ctx context.Context, s *testing.State) {
	const (
		// Repeat read count
		readCount = 200
		// Maximum sensible EC temperature (in Kelvins)
		maxECTemp = 373
		// Minimum sensible EC temperature (in Kelvins)
		minECTemp = 273
	)

	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.Servo.RunECCommand(ctx, "chan save"); err != nil {
		s.Fatal("Failed to send 'chan save' to EC: ", err)
	}
	if err := h.Servo.RunECCommand(ctx, "chan 0"); err != nil {
		s.Fatal("Failed to send 'chan 0' to EC: ", err)
	}

	defer func() {
		if err := h.Servo.RunECCommand(ctx, "chan restore"); err != nil {
			s.Fatal("Failed to send 'chan restore' to EC: ", err)
		}
	}()

	s.Logf("Reading EC internal temperature for %d times", readCount)
	for i := 1; i <= readCount; i++ {
		ecTemperatureOut, err := h.Servo.RunECCommandGetOutput(ctx, "temps", []string{`ECInternal\s+: (\d+) K`})
		if err != nil {
			s.Fatal("Failed to read EC internal temperature temperature: ", err)
		}
		ecTemperatureStr := ecTemperatureOut[0][1]
		ecTemperature, err := strconv.ParseInt(ecTemperatureStr, 10, 64)
		if err != nil {
			s.Fatalf("Failed to parse EC internal temperature (%s) as int: %s",
				ecTemperatureStr,
				err)
		}
		if ecTemperature > maxECTemp || ecTemperature < minECTemp {
			s.Fatal("Abnormal EC temperature: ", ecTemperature)
		}
	}
}
