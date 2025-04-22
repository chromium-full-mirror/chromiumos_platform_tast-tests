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
	"go.chromium.org/tast/core/ctxutil"
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
		Vars:         []string{"servo"},
		VarDeps:      []string{"typec.UnigrafUri"},
		Attr:         []string{"group:typec", "typec_unigraf274", "typec_informational"},
	})
}

func SprVoltages(ctx context.Context, s *testing.State) {
	d := s.DUT()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Set up unigraf
	unigrafURI := s.RequiredVar("typec.UnigrafUri")
	unigrafctl, err := unigraf.New(ctx, unigrafURI)
	if err != nil {
		s.Fatal("Failed to allocate unigraf device: ", err)
	}
	defer unigrafctl.Close(cleanupCtx)

	// Set up servo
	if servoSpec, present := s.Var("servo"); present {
		pxy, err := servo.NewProxy(ctx, servoSpec, d.KeyFile(), d.KeyDir())
		if err != nil {
			s.Fatal("Failed to setup servo proxy: ", err)
		}
		if err := pxy.Servo().ServoCcSnk(ctx); err != nil {
			s.Fatal("Failed to set servo CC to off: ", err)
		}
		defer pxy.Servo().ServoCcDrp(cleanupCtx)

		// On Unigraf setup, ethernet is connected by servo, wait for the connection to resume.
		connectCtx, connectCtxCancel := context.WithTimeout(ctx, 30*time.Second)
		if err := d.WaitConnect(connectCtx); err != nil {
			s.Fatal("DUT not reachable in time: ", err)
		}
		connectCtxCancel()
	}

	// Set unigraf as a power source
	if err := unigrafctl.SetInitPdState(ctx, unigraf.InitPdStateDfp); err != nil {
		s.Fatal("Failed to set power role to SRC: ", err)
	}
	defer unigrafctl.SetInitPdState(cleanupCtx, unigraf.InitPdStateDrp)

	// Replug the Unigraf.
	if err := unigrafctl.Replug(ctx); err != nil {
		s.Fatal("Failed to replug unigraf: ", err)
	}

	// Voltages are in mV
	voltages := []int{5000, 9000, 15000, 20000}
	for index, voltage := range voltages {
		pdoCnt := index + 1
		s.Logf("Setting SrcPdoCount to %d for voltage %dV", pdoCnt, voltage)
		if err := unigrafctl.SetSrcPdoCount(ctx, int64(pdoCnt)); err != nil {
			s.Fatalf("Failed to set SrcPdoCount to %d: %v", pdoCnt, err)
		}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			voltageBuf := voltage / 10
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
