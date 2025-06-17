// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/typecutils"
	"go.chromium.org/tast-tests/cros/common/usbutils/unigraf"
	"go.chromium.org/tast-tests/cros/remote/typec/typecunigraf"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     SprVoltages,
		Desc:     "Test negotiation for highest PDO reported by charger",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com"},
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		Fixture:      "typecUnigrafAndServo",
		Attr:         []string{"group:typec", "typec_unigraf274", "typec_informational"},
	})
}

func SprVoltages(ctx context.Context, s *testing.State) {
	d := s.DUT()

	// Get Unigraf controller from fixture.
	fixtData, ok := s.FixtValue().(*typecunigraf.FixtureData)
	if !ok {
		s.Fatal("Failed to get Unigraf controller from fixture")
	}
	unigrafctl := fixtData.Unigraf

	// Make sure Unigraf uses correct port.
	if err := unigrafctl.SetTestPort(ctx, 0); err != nil {
		s.Fatal("Failed to set testing port: ", err)
	}

	// Set unigraf as a power source
	if err := unigrafctl.SetInitPdState(ctx, unigraf.InitPdStateDfp); err != nil {
		s.Fatal("Failed to set power role to SRC: ", err)
	}
	defer unigrafctl.SetInitPdState(ctx, unigraf.InitPdStateDrp)

	// Replug the Unigraf.
	if err := unigrafctl.Replug(ctx); err != nil {
		s.Fatal("Failed to replug unigraf: ", err)
	}

	// Voltages are in mV
	voltages := []int{5000, 9000, 15000, 20000}
	for index, voltage := range voltages {
		pdoCnt := index + 1
		voltageBuf := voltage / 10
		s.Logf("Setting SrcPdoCount to %d for voltage %dV", pdoCnt, voltage)
		if err := unigrafctl.SetSrcPdoCount(ctx, int64(pdoCnt)); err != nil {
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
			} else if reportedVoltageDUT < voltage-voltageBuf || reportedVoltageDUT > voltage+voltageBuf {
				return errors.Wrapf(err, "DUT reported voltage %d, expected %d", reportedVoltageDUT, voltage)
			}

			if reportedVoltageUnigraf, err := unigrafctl.VbusVoltage(ctx); err != nil {
				return errors.Wrap(err, "failed to get unigraf voltage report")
			} else if reportedVoltageUnigraf < voltage-voltageBuf || reportedVoltageUnigraf > voltage+voltageBuf {
				return errors.Wrapf(err, "Unigraf reported voltage %d, expected %d", reportedVoltageUnigraf, voltage)
			}
			return nil
		}, &testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second}); err != nil {
			s.Fatal("DUT failed to negotiate voltage: ", err)
		}
	}
}
