// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/battery"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/arc/vm"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// idleTestEntry is used as sub-tests entry.
type idleTestEntry struct {
	desc string
	fn   func(ctx context.Context, s *testing.State)
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Doze,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks memory stat during Doze state on / off",
		Contacts:     []string{"arcvm-eng@google.com", "sstan@chromium.org"},
		// ChromeOS > Software > ARC++ > ARCVM
		BugComponent: "b:537221",
		SoftwareDeps: []string{"chrome", "android_vm", "gaia"},
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      7 * time.Minute,
		VarDeps:      []string{ui.GaiaPoolDefaultVarName},
		Params: []testing.Param{{
			Val: idleTestEntry{
				desc: "verify device can be doze state",
				fn:   dozeModeAvaliable,
			},
		}},
	})
}

func Doze(ctx context.Context, s *testing.State) {
	entry := s.Param().(idleTestEntry)
	s.Run(ctx, entry.desc, entry.fn)
}

func dozeModeAvaliable(ctx context.Context, s *testing.State) {
	// Shorten the context to make room for cleanup jobs.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	args := append(arc.DisableSyncFlags(), "--disable-sync")
	cr, err := chrome.New(
		ctx,
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
		chrome.ARCSupported(),
		// TODO(b/308366449): `ignore_battery_for_test` parameter may not work on some ARC version,
		// in that case the test need to run without charging cable connection.
		chrome.EnableFeatures("ArcIdleManager:ignore_battery_for_test/true/delay_ms/0"),
		chrome.ExtraArgs(args...),
	)
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	// Make sure DUT in clamshell mode.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure if tablet mode is enabled: ", err)
	}
	defer cleanup(ctx)

	// Optin is needed to enable the PlayStore.
	if err := optin.PerformAndClose(ctx, cr, tconn); err != nil {
		s.Fatal("Failed to optin to Play Store: ", err)
	}
	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	socketPath, err := vm.SocketPath()
	if err != nil {
		s.Fatal("Failed to get arcvm socket path: ", err)
	}

	if err := battery.SetAcOnline(ctx, socketPath, false); err != nil {
		s.Fatalf("Failed to disable aconline for %s: %v", socketPath, err)
	}
	defer battery.SetAcOnline(ctx, socketPath, true)

	s.Log("Wait ARC get into doze when no app running")
	// If no ARC app launch for a while, the ARC should be in the doze state.
	if err := waitForDoze(ctx, a, true); err != nil {
		s.Fatal("Failed to wait ARC doze: ", err)
	}

	s.Log("Launch PlayStore and wait ARC exit doze")
	// If any ARC app launched, it should exit the doze state at the end.
	if err := launcher.LaunchApp(tconn, apps.PlayStore.Name)(ctx); err != nil {
		s.Fatal("Failed to launch PlayStore from launcher: ", err)
	}
	defer optin.ClosePlayStore(ctx, tconn)

	if err := waitForDoze(ctx, a, false); err != nil {
		s.Fatal("Failed to wait ARC leave doze: ", err)
	}
}

func waitForDoze(ctx context.Context, a *arc.ARC, isDoze bool) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		state, err := a.DeviceIdleState(ctx)
		if err != nil {
			return testing.PollBreak(err)
		}
		if isDoze != state.IsDoze() {
			return errors.Errorf("ARC device idle doze state still %v", state)
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Minute, Interval: 5 * time.Second}); err != nil {
		return err
	}
	return nil
}
