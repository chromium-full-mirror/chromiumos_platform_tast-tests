// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: GSCKBPress,
		Desc: "Verify GSC simulated keypress (kbpress command) interacts correctly with EC scan",
		Contacts: []string{
			"cros-hwsec@google.com",
			"wisniewskib@google.com",
		},
		BugComponent: "b:715469",
		TestBedDeps:  []string{tbdep.ServoComponent("ccd_ti50")},
		// TODO(b/513305476): enable after crrev.com/i/9347641
		// Attr:         []string{"group:firmware", "firmware_unstable"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.GSCCCDTestlabEnabled()),
		SoftwareDeps: []string{"gsc"},
		Fixture:      fixture.NormalMode,
		Timeout:      2 * time.Minute,
	})
}

func GSCKBPress(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	version, err := h.Servo.GSCVersionInfo(ctx)
	if err != nil {
		s.Fatal("Failed to get GSC version info: ", err)
	}
	if !version.IsTi50 {
		s.Log("Skipping test on non-Ti50 GSC")
		return
	}

	if err := h.OpenCCD(ctx, true, true); err != nil {
		s.Fatal("Failed to open CCD: ", err)
	}

	// Filter EC console channels to silence noisy background logs (like charger/thermal)
	s.Log("Filtering EC console channels to prevent log spam")
	if err := h.Servo.RunECCommand(ctx, "chan save"); err != nil {
		s.Fatal("Failed to save EC console channels: ", err)
	}
	defer h.Servo.RunECCommand(cleanupCtx, "chan restore")

	// Enable command (0x1) + keyboard (0x100) + keyscan (0x200) = 0x301
	if err := h.Servo.RunECCommand(ctx, "chan 0x301"); err != nil {
		s.Fatal("Failed to set EC console channels: ", err)
	}
	if err := h.Servo.RunECCommand(ctx, "ksstate on"); err != nil {
		s.Fatal("Failed to enable KB state printing: ", err)
	}
	defer h.Servo.RunECCommand(cleanupCtx, "ksstate off")

	s.Log("Enabling KB simulation for 10s on GSC via 'kbpress 2 10000'")
	// time (second param) is in ms
	if err := h.Servo.RunGSCCommand(ctx, "kbpress 2 10000"); err != nil {
		s.Fatal("Failed to run kbpress on GSC: ", err)
	}

	s.Log("Getting EC keyboard state via 'kbpress 1 1 1' & 'kbpress 1 1 0' on EC console")
	if err := h.Servo.RunECCommand(ctx, "kbpress 1 1 1"); err != nil {
		s.Fatal("Failed to run kbpress assertion on EC: ", err)
	}
	out, err := h.Servo.RunECCommandGetOutput(ctx, "kbpress 1 1 0", []string{`KB state:\s*([^\r\n\]]+)`})
	if err != nil {
		s.Fatal("Failed to run kbpress on EC: ", err)
	}

	kbState := out[0][1]
	s.Log("EC reported KB state: ", kbState)

	// Verify that the GSC response (03) is captured in the matrix state.
	if !strings.Contains(kbState, "03") {
		s.Fatalf("EC failed to detect GSC simulated keypress: got %q, want '03'", kbState)
	}
}
