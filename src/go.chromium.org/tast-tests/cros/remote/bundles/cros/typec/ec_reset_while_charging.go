// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/typecutils"
	"go.chromium.org/tast-tests/cros/remote/typec/typecunigraf"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	params := typecunigraf.GenerateUnigrafParams(typecunigraf.TestSetupData{}, 3)
	testing.AddTest(&testing.Test{
		Func: ECResetWhileCharging,
		Desc: "Check that DUT is charging after EC reset",
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Fixture:      "typecUnigrafAndServo",
		Contacts:     []string{"chromeos-usb-champs@google.com", "bszpila@google.com"},
		Attr:         []string{"group:typec", "typec_informational"},
		Params:       params,
	})
}

func ECResetWhileCharging(ctx context.Context, s *testing.State) {
	d := s.DUT()

	// Get Unigraf controller from fixture.
	fixtData, ok := s.FixtValue().(*typecunigraf.FixtureData)
	if !ok {
		s.Fatal("Failed to get Unigraf controller from fixture")
	}
	unigrafctl := fixtData.Unigraf

	// Setup Unigraf.
	if err := typecunigraf.SetupUnigraf(ctx, unigrafctl, s.Param().(typecunigraf.TestSetupData)); err != nil {
		s.Fatal("Failed to setup Unigraf: ", err)
	}

	// Verify that the DUT is charging within 10 seconds.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if connected, err := typecutils.VerifyChargerConnected(ctx, d); err != nil {
			return errors.Wrap(err, "failed to verify charger connection")
		} else if !connected {
			return errors.New("charger is not connected after EC reset")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 500 * time.Millisecond}); err != nil {
		s.Fatal("Failed to verify charger connection after EC reset: ", err)
	}

	// Issue EC reset command.
	s.Log("Issuing EC reset command")
	if err := d.Conn().CommandContext(ctx, "ectool", "reboot_ec").Start(); err != nil {
		s.Fatal("Failed to issue EC reset command: ", err)
	}

	// Verify DUT is unreachable after ec reboot
	if err := d.WaitUnreachable(ctx); err != nil {
		s.Fatal("DUT is not powered down after ec reboot: ", err)
	}

	// Wait for the DUT to reboot.
	if err := testing.Poll(ctx, d.Connect, &testing.PollOptions{Timeout: time.Minute}); err != nil {
		s.Fatal("Failed to re-connect to DUT after reboot: ", err)
	}

	// Verify that the DUT is charging within 10 seconds.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if connected, err := typecutils.VerifyChargerConnected(ctx, d); err != nil {
			return errors.Wrap(err, "failed to verify charger connection")
		} else if !connected {
			return errors.New("charger is not connected after EC reset")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 500 * time.Millisecond}); err != nil {
		s.Fatal("Failed to verify charger connection after EC reset: ", err)
	}

	s.Log("Charger connected successfully after EC reset")
}
