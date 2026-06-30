// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECADC,
		Desc: "Check that all temperature sensors return reasonable values over 200 iterations",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT

		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_ec", "firmware_stressed", "firmware_meets_kpi", "firmware_ec_ro", "firmware_ec_rw"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      10 * time.Minute,
	})
}

// ECADC measures the EC internal temperature sensors in a loop for
// couple of retries. This test might fail on boards which don't have
// "temps" EC command available.
func ECADC(ctx context.Context, s *testing.State) {
	const (
		// Repeat read count
		readCount = 100
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

	s.Logf("Reading EC temperature sensors for %d iterations", readCount)
	extraTries := 0
	goodReads := 0
	failedReads := 0
	for i := 1; i <= readCount+extraTries; i++ {
		s.Log("Reading EC temps")

		// Empirical testing shows that lab devices showed the following
		// fail rates at these capture times:
		//   250ms: 56% fail
		//   500ms: 30% fail
		//   750ms: 0% fail
		output, err := h.Servo.CaptureECCommand(ctx, "temps", time.Millisecond*750)

		s.Log("EC output: ", output)

		ecTempsParsed, err := parseTempsOutput(ctx, output)
		if err != nil {
			s.Errorf("Failed to parse temperature reading (%s): %s",
				output,
				err)
			continue
		}

		if len(ecTempsParsed) != 0 {
			goodReads++
		} else {
			failedReads++
		}

		for _, ecTemperature := range ecTempsParsed {
			// Sometimes the temp (eg 316 K) gets broken up into 2 lines resulting in it being parsed as {Name:31 TempKelvin:6}.
			// Allow some retries for this situation as well.
			if ecTemperature.TempKelvin > maxECTemp || ecTemperature.TempKelvin < minECTemp {
				// Ignore up to 20 failures as it may be anomaly/poorly parse regex, but after that start failing.
				if extraTries > 20 {
					s.Errorf("%d: Abnormal EC temperature: %+v raw: %q", i, ecTemperature, output)
				} else {
					extraTries++
					s.Logf("%d: Abnormal EC temperature: %+v raw: %q, retrying", i, ecTemperature, output)
				}
				continue
			}
		}

	}
	s.Log("Good reads: ", goodReads)
	s.Log("Failed reads: ", failedReads)
}

type temp struct {
	Name       string
	TempKelvin int64
}

func parseTempsOutput(ctx context.Context, output string) ([]temp, error) {
	var result []temp
	var parsing bool

	lines := strings.Split(output, "\r\n")
	for _, line := range lines {
		// Skip output until we find the EC echoed the "temps" command back
		if strings.Contains(line, "temps") {
			parsing = true
			continue
		}

		if parsing {
			fields := strings.Fields(line)
			kIndex := -1
			for i, f := range fields {
				if f == "K" {
					kIndex = i
					break
				}
			}

			// Legacy EC uses "<name> : <temperature> K"
			// Zepyhr EC uses "<name> <temperature> K"
			//
			// We need at least two fields before "K": name and temperature,
			// legacy EC needs 3.
			if kIndex >= 2 {
				nameIndex := kIndex - 2
				tempIndex := kIndex - 1
				if fields[kIndex-2] == ":" && kIndex >= 3 {
					nameIndex = kIndex - 3
				}

				name := fields[nameIndex]
				tempStr := fields[tempIndex]
				k, err := strconv.ParseInt(tempStr, 10, 64)
				if err != nil {
					// Malformed line, skip.
					continue
				}
				result = append(result, temp{
					Name:       name,
					TempKelvin: k,
				})
			}
		}
	}
	return result, nil
}
