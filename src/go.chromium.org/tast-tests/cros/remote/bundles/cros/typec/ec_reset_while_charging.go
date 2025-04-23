// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typec

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/typecutils"
	"go.chromium.org/tast-tests/cros/common/usbutils/unigraf"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECResetWhileCharging,
		Desc: "Check that DUT is charging after EC reset",
		// ChromeOS > Platform > Technologies > USB
		BugComponent: "b:958036",
		VarDeps:      []string{"typec.UnigrafUri"},
		Contacts:     []string{"chromeos-usb-champs@google.com", "bszpila@google.com"},
		Attr:         []string{"group:typec", "typec_unigraf274", "typec_informational"},
		Timeout:      3 * time.Minute,
	})
}

func ECResetWhileCharging(ctx context.Context, s *testing.State) {
	d := s.DUT()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	unigrafURI := s.RequiredVar("typec.UnigrafUri")
	unigrafctl, err := unigraf.New(ctx, unigrafURI)
	if err != nil {
		s.Fatal("Failed to allocate unigraf device: ", err)
	}
	defer unigrafctl.Close(cleanupCtx)

	if err := unigrafctl.SetTestPort(ctx, 0); err != nil {
		s.Fatal("Failed to set testing port: ", err)
	}
	s.Log("Unigraf testing port was set to port 0")

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
