// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/typecutils"
	"go.chromium.org/tast-tests/cros/common/usbutils/utc"
	"go.chromium.org/tast-tests/cros/remote/typec/typecutc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	params := typecutc.GenerateUtcParams(typecutc.TestSetupData{
		InitialPdState: utc.InitPdStateDfp,
	}, 10)
	testing.AddTest(&testing.Test{
		Func:     ChargingCurrent,
		Desc:     "Test if charging current is ~3A after connecting utc as source",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Fixture:      "typecUtcAndServo",
		Attr:         []string{"group:typec", "typec_informational"},
		Params:       params,
	})
}

func ChargingCurrent(ctx context.Context, s *testing.State) {
	d := s.DUT()

	// Get utc controller from fixture.
	fixtData, ok := s.FixtValue().(*typecutc.FixtureData)
	if !ok {
		s.Fatal("Failed to get utc controller from fixture")
	}
	utcctl := fixtData.Utc

	// Setup utc.
	if err := typecutc.SetupUtc(ctx, utcctl, s.Param().(typecutc.TestSetupData)); err != nil {
		s.Fatal("Failed to setup utc: ", err)
	}

	// Expected current in mA and tolerance buffer (e.g., +/- 10%)
	const expectedCurrent = 3000
	currentBuffer := expectedCurrent / 10 // 10% of expected current

	s.Logf("Polling for DUT charging current to be around %dmA", expectedCurrent)
	if err := testing.Poll(ctx, func(ctx context.Context) error {

		// Check DUT's reported current.
		reportedCurrentDUT, err := typecutils.GetChargerCurrent(ctx, d)
		if err != nil {
			return errors.Wrap(err, "failed to get DUT current report")
		}
		if reportedCurrentDUT < expectedCurrent-currentBuffer || reportedCurrentDUT > expectedCurrent+currentBuffer {
			return errors.Wrapf(err, "DUT reported current %dmA, expected %dmA", reportedCurrentDUT, expectedCurrent)
		}

		// Check utc's reported current.
		reportedCurrentutc, err := utcctl.VbusCurrent(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get utc current report")
		}
		if reportedCurrentutc < expectedCurrent-currentBuffer || reportedCurrentutc > expectedCurrent+currentBuffer {
			return errors.Wrapf(err, "utc reported current %dmA, expected %dmA", reportedCurrentutc, expectedCurrent)
		}

		s.Logf("DUT: %dmA, utc: %dmA. Current is within expected range", reportedCurrentDUT, reportedCurrentutc)
		return nil
	}, &testing.PollOptions{Interval: 2 * time.Second, Timeout: 30 * time.Second}); err != nil {
		s.Fatal("DUT failed to negotiate expected current: ", err)
	}
}
