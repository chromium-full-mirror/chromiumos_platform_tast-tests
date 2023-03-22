// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"strings"
	"time"

	"chromiumos/tast/common/servo"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/dut"
	"chromiumos/tast/errors"
	"chromiumos/tast/remote/firmware/fixture"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TabletModeNotification,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies the EC's tablet mode change notification is received by the AP (powerd)",
		BugComponent: "b:167191",
		Contacts:     []string{"chromeos-platform-power@google.com", "timvp@google.com", "cros-fw-engprod@google.com"},
		ServiceDeps:  []string{"tast.cros.security.BootLockboxService"},
		SoftwareDeps: []string{"chrome"},
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		Attr:         []string{"group:mainline", "informational", "group:firmware", "firmware_unstable"},
		Fixture:      fixture.NormalMode,
		Vars:         []string{"servo"},
		Timeout:      5 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.FormFactor(hwdep.Convertible, hwdep.Detachable)),
	})
}

// isDUTInTabletMode returns true if the DUT is in tablet mode, false otherwise.
func isDUTInTabletMode(ctx context.Context, s *testing.State, dut *dut.DUT) bool {
	// Verify powerd received the tablet mode change notification.
	testing.ContextLog(ctx, "Get the tabletmode value from powerd")
	// Expected output from the `dbus-send` command is:
	//    boolean true
	// OR
	//    boolean false
	out, err := dut.Conn().CommandContext(ctx, "dbus-send", "--system", "--print-reply=literal", "--type=method_call", "--dest=org.chromium.PowerManager", "/org/chromium/PowerManager", "org.chromium.PowerManager.GetTabletMode").Output()
	if err != nil {
		s.Fatal("Failed to retrieve dbus-send output: ", err)
	}
	words := strings.Fields(string(out))
	if len(words) != 2 {
		s.Fatal("Received unexpected output from dbus-send: ", out)
	}

	testing.ContextLog(ctx, "powerd tabletmode: ", words[1])
	return words[1] == "true"
}

// verifyPowerdTogglesTabletMode waits for powerd to toggle tabletmode and verifies it's the correct
// value. Returns an error otherwise.
func verifyPowerdTogglesTabletMode(ctx context.Context, s *testing.State, dut *dut.DUT, expectedMode bool) {
	s.Log("Verify powerd received the tablet mode change notification to: ", expectedMode)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		isTabletMode := isDUTInTabletMode(ctx, s, dut)
		if isTabletMode != expectedMode {
			return errors.Errorf("powerd failed to toggle tabletmode: isTabletMode=%t", isTabletMode)
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 250 * time.Millisecond}); err != nil {
		s.Fatal("Failed to get tablet mode status: ", err)
	}
}

func TabletModeNotification(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Performing cleanup")
		if err := pxy.Servo().RunECCommand(ctx, "tabletmode reset"); err != nil {
			s.Fatal("Failed to restore tabletmode to the original settings: ", err)
		}
	}(cleanupCtx)

	testing.ContextLog(ctx, "Enable tablet mode")
	if err := pxy.Servo().RunECCommand(ctx, "tabletmode on"); err != nil {
		s.Fatal("Failed to enable tablet mode: ", err)
	}
	// Give powerd time to process the notification and switch the DUT to tablet mode.
	verifyPowerdTogglesTabletMode(ctx, s, dut, true)

	// We aren't attempting to stress test the tablet mode switching capabilities, so
	// give powerd time to finish switching the DUT's state before toggling it again.
	testing.Sleep(ctx, 1*time.Second)

	testing.ContextLog(ctx, "Disable tablet mode")
	if err := pxy.Servo().RunECCommand(ctx, "tabletmode off"); err != nil {
		s.Fatal("Failed to disable tablet mode: ", err)
	}
	// Give powerd time to process the notification and switch the DUT to tablet mode.
	verifyPowerdTogglesTabletMode(ctx, s, dut, false)

	// We aren't attempting to stress test the tablet mode switching capabilities, so
	// give powerd time to finish switching the DUT's state before toggling it again.
	testing.Sleep(ctx, 1*time.Second)

	testing.ContextLog(ctx, "Enable tablet mode")
	if err := pxy.Servo().RunECCommand(ctx, "tabletmode on"); err != nil {
		s.Fatal("Failed to enable tablet mode: ", err)
	}
	// Give powerd time to process the notification and switch the DUT to tablet mode.
	verifyPowerdTogglesTabletMode(ctx, s, dut, true)
}
