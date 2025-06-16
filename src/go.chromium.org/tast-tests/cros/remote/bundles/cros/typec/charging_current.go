// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/typecutils"
	"go.chromium.org/tast-tests/cros/common/usbutils/unigraf"
	"go.chromium.org/tast-tests/cros/remote/typec/typecunigraf"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     ChargingCurrent,
		Desc:     "Test if charging current is ~3A after connecting Unigraf as source",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Fixture:      "typecUnigraf",
		Attr:         []string{"group:typec", "typec_unigraf274", "typec_informational"},
	})
}

func ChargingCurrent(ctx context.Context, s *testing.State) {
	d := s.DUT()

	// Get Unigraf controller from fixture.
	fixtData, ok := s.FixtValue().(*typecunigraf.FixtureData)
	if !ok {
		s.Fatal("Failed to get Unigraf controller from fixture")
	}
	unigrafctl := fixtData.Unigraf

	// Set up servo
	if servoSpec, present := s.Var("servo"); present {
		pxy, err := servo.NewProxy(ctx, servoSpec, d.KeyFile(), d.KeyDir())
		if err != nil {
			s.Fatal("Failed to setup servo proxy: ", err)
		}
		// Ensure servo is not interfering by connecting as a charge source.
		if err := pxy.Servo().ServoCcSnk(ctx); err != nil {
			s.Fatal("Failed to set servo CC to off: ", err)
		}
		defer pxy.Servo().ServoCcSrc(ctx, true)

		// On Unigraf setup, ethernet might be connected via servo, wait for connection.
		connectCtx, connectCtxCancel := context.WithTimeout(ctx, 30*time.Second)
		defer connectCtxCancel()
		if err := d.WaitConnect(connectCtx); err != nil {
			s.Fatal("DUT not reachable in time: ", err)
		}
	}

	// Make sure Unigraf is set to correct test port.
	if err := unigrafctl.SetTestPort(ctx, 0); err != nil {
		s.Fatal("Failed to set testing port: ", err)
	}
	s.Log("Unigraf testing port was set to port 0")

	// Set unigraf as a power source (DFP)
	if err := unigrafctl.SetInitPdState(ctx, unigraf.InitPdStateDfp); err != nil {
		s.Fatal("Failed to set power role to SRC (DFP): ", err)
	}
	// Ensure Unigraf state is reset at the end.
	defer unigrafctl.SetInitPdState(ctx, unigraf.InitPdStateDrp)

	// Ensure Unigraf sends 20V PDO.
	if err := unigrafctl.SetSrcPdoCount(ctx, 4); err != nil {
		s.Fatal("Failed to set power role to SRC (DFP): ", err)
	}

	// Replug the Unigraf to trigger negotiation.
	s.Log("Replugging Unigraf")
	if err := unigrafctl.Replug(ctx); err != nil {
		s.Fatal("Failed to replug unigraf: ", err)
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

		// Check Unigraf's reported current.
		reportedCurrentUnigraf, err := unigrafctl.VbusCurrent(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get unigraf current report")
		}
		if reportedCurrentUnigraf < expectedCurrent-currentBuffer || reportedCurrentUnigraf > expectedCurrent+currentBuffer {
			return errors.Wrapf(err, "Unigraf reported current %dmA, expected %dmA", reportedCurrentUnigraf, expectedCurrent)
		}

		s.Logf("DUT: %dmA, Unigraf: %dmA. Current is within expected range", reportedCurrentDUT, reportedCurrentUnigraf)
		return nil
	}, &testing.PollOptions{Interval: 2 * time.Second, Timeout: 30 * time.Second}); err != nil {
		s.Fatal("DUT failed to negotiate expected current: ", err)
	}
}
