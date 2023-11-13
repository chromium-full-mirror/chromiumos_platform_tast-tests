// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
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
		Attr:         []string{"group:firmware", "firmware_ec"},
		Requirements: []string{"sys-fw-0022-v02"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      10 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// ECADC measures the EC internal temperature sensors in a loop for
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

	s.Logf("Reading EC temperature sensors for %d iterations", readCount)
	for i := 1; i <= readCount; i++ {
		ecTemperatureOut, err := h.Servo.RunECCommandGetOutput(ctx, "temps", []string{`.*(>|~\$)`})
		if err != nil {
			s.Fatal("Failed to read EC temperature sensors: ", err)
		}
		ecTempsParsed, err := parseTempsOutput(ctx, ecTemperatureOut[0][0])
		if err != nil {
			s.Fatalf("Failed to parse temperature reading (%s): %s",
				ecTemperatureOut[0][1],
				err)
		}
		for _, ecTemperature := range ecTempsParsed {
			if ecTemperature.TempKelvin > maxECTemp || ecTemperature.TempKelvin < minECTemp {
				s.Fatalf("Abnormal EC temperature: %+v", ecTemperature)
			}
		}
	}
}

type temp struct {
	Name       string
	TempKelvin int64
}

var tempRe = regexp.MustCompile(`(\S+)\s*:?\s*(\d+) K`)

func parseTempsOutput(ctx context.Context, output string) ([]temp, error) {
	var result []temp
	for _, line := range strings.Split(output, "\r\n") {
		m := tempRe.FindStringSubmatch(line)
		if m != nil {
			k, err := strconv.ParseInt(m[2], 10, 64)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse int")
			}
			result = append(result, temp{
				Name:       m[1],
				TempKelvin: k,
			})
		}
	}
	return result, nil
}
