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
		Contacts:     []string{"timvp@google.com", "cros-fw-engprod@google.com"},
		ServiceDeps:  []string{"tast.cros.security.BootLockboxService"},
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational", "group:firmware", "firmware_unstable"},
		Fixture:      fixture.NormalMode,
		Vars:         []string{"servo"},
		Timeout:      5 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.FormFactor(hwdep.Convertible, hwdep.Chromeslate, hwdep.Detachable)),
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

	return words[1] == "true"
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
	isTabletMode := isDUTInTabletMode(ctx, s, dut)
	if !isTabletMode {
		s.Fatal("powerd failed to update tabletmode status: ", isTabletMode)
	}

	testing.ContextLog(ctx, "Disable tablet mode")
	if err := pxy.Servo().RunECCommand(ctx, "tabletmode off"); err != nil {
		s.Fatal("Failed to disable tablet mode: ", err)
	}
	// Verify powerd received the tablet mode change notification.
	isTabletMode = isDUTInTabletMode(ctx, s, dut)
	if isTabletMode {
		s.Fatal("powerd failed to update tabletmode status: ", isTabletMode)
	}

	testing.ContextLog(ctx, "Enable tablet mode")
	if err := pxy.Servo().RunECCommand(ctx, "tabletmode on"); err != nil {
		s.Fatal("Failed to enable tablet mode: ", err)
	}
	// Verify powerd received the tablet mode change notification.
	isTabletMode = isDUTInTabletMode(ctx, s, dut)
	if !isTabletMode {
		s.Fatal("powerd failed to update tabletmode status: ", isTabletMode)
	}
}
