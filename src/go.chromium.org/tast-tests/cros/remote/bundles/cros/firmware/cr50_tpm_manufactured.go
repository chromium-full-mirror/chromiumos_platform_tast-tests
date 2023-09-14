// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Cr50TPMManufactured,
		Desc: "Check if the TPM is manufactured",
		Contacts: []string{
			"chromeos-faft@google.com",
			"pf@semihalf.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      fixture.DevMode,
		Timeout:      5 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.GSCRWKeyIDProd(), hwdep.GSCUART()),
		SoftwareDeps: []string{"gsc"},
	})
}

func Cr50TPMManufactured(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servod")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 60*time.Second)
	defer cancel()
	s.Log("Capturing GSC log")
	if err := h.Servo.SetOnOff(ctx, servo.CR50UARTCapture, servo.On); err != nil {
		s.Fatal("Failed to capture GSC UART: ", err)
	}
	defer func(ctx context.Context) {
		if err := h.Servo.SetOnOff(ctx, servo.CR50UARTCapture, servo.Off); err != nil {
			s.Fatal("Failed to disable capture GSC UART: ", err)
		}
	}(cleanupCtx)
	// Read the UART stream just to make sure there isn't buffered data.
	if _, err := h.Servo.GetQuotedString(ctx, servo.CR50UARTStream); err != nil {
		s.Fatal("Failed to read UART: ", err)
	}
	s.Log("Rebooting GSC")
	if err := h.Servo.RunCR50Command(ctx, "reboot"); err != nil {
		s.Fatal("Failed to send reboot command: ", err)
	}
	// Wait a little at the end of the test to make sure the GSC finishes booting before the next test runs.
	defer func(ctx context.Context) {
		if err := s.DUT().WaitConnect(ctx); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}
	}(cleanupCtx)

	if err := h.Servo.PollForString(ctx, servo.CR50UARTStream, "tpm_manufactured: manufactured"); err != nil {
		s.Fatal(errors.Wrap(err, "GSC output parsing failed"))
	}
}
