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
		Func:     SprVoltages,
		Desc:     "Test negotiation for highest PDO reported by charger",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Fixture:      "typecUtcAndServo",
		Attr:         []string{"group:typec", "typec_informational"},
		Params:       params,
	})
}

func SprVoltages(ctx context.Context, s *testing.State) {
	d := s.DUT()

	// Get utc controller from fixture.
	fixtData, ok := s.FixtValue().(*typecutc.FixtureData)
	if !ok {
		s.Fatal("Failed to get utc controller from fixture")
	}
	utcctl := fixtData.Utc

	// Perform utc setup.
	if err := typecutc.SetupUtc(ctx, utcctl, s.Param().(typecutc.TestSetupData)); err != nil {
		s.Fatal("Failed to setup utc: ", err)
	}

	// Voltages are in mV
	voltages := []int{5000, 9000, 15000, 20000}
	for index, voltage := range voltages {
		pdoCnt := index + 1
		voltageDutBuf := voltage / 5
		voltageUtcBuf := voltage / 10
		s.Logf("Setting SrcPdoCount to %d for voltage %dV", pdoCnt, voltage)
		if err := utcctl.SetSrcPdoCount(ctx, int64(pdoCnt)); err != nil {
			s.Fatalf("Failed to set SrcPdoCount to %d: %v", pdoCnt, err)
		}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if connected, err := typecutils.VerifyChargerConnected(ctx, d); err != nil {
				return errors.Wrap(err, "failed to verify charger connection")
			} else if !connected {
				return errors.New("charger is not connected")
			}

			if reportedVoltageDUT, err := typecutils.GetChargerVoltage(ctx, d); err != nil {
				return errors.Wrap(err, "failed to get DUT voltage report")
			} else if reportedVoltageDUT < voltage-voltageDutBuf || reportedVoltageDUT > voltage+voltageDutBuf {
				return errors.Wrapf(err, "DUT reported voltage %d, expected %d", reportedVoltageDUT, voltage)
			}

			if reportedVoltageUtc, err := utcctl.VbusVoltage(ctx); err != nil {
				return errors.Wrap(err, "failed to get utc voltage report")
			} else if reportedVoltageUtc < voltage-voltageUtcBuf || reportedVoltageUtc > voltage+voltageUtcBuf {
				return errors.Wrapf(err, "utc reported voltage %d, expected %d", reportedVoltageUtc, voltage)
			}
			return nil
		}, &testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second}); err != nil {
			s.Fatal("DUT failed to negotiate voltage: ", err)
		}
	}
}
