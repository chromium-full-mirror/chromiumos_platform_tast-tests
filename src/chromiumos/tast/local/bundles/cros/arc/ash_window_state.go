// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/chrome/ash"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AshWindowState,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that sending Ash WM event will change ARC app window state correctly",
		Contacts:     []string{"arc-framework+tast@google.com", "takise@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Window Management
		BugComponent: "b:537272",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

func AshWindowState(ctx context.Context, s *testing.State) {
	a := s.FixtValue().(*arc.PreData).ARC

	cr := s.FixtValue().(*arc.PreData).Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// Force Chrome to be in clamshell mode to test window state related functionalities.
	// TODO(xdai): Test window state functionalities in tablet mode as well.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to disable tablet mode: ", err)
	}
	defer cleanup(cleanupCtx)

	// Start the Settings app.
	const pkg = "com.android.settings"
	act, err := arc.NewActivity(a, pkg, ".Settings")
	if err != nil {
		s.Fatal("Failed to create new activity: ", err)
	}

	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatal("Failed to start the Settings activity: ", err)
	}

	for _, test := range []struct {
		wmEvent             ash.WMEventType
		expectedWindowState ash.WindowStateType
	}{
		// TODO(b/264965692) Investigate what regressed between RVC and TM.
		// We first set to a known state and then test fullscreen and return to
		// normal again.  This is because as per the remote shell documentation, the
		// compositor can choose to ignore a fullscreen event.
		// https://source.chromium.org/chromium/chromium/src/+/main:third_party/wayland-protocols/unstable/remote-shell/remote-shell-unstable-v2.xml;drc=d654e85745b56e94ce5ecdf306e476a252bc1404;l=262

		//  Initial state
		{ash.WMEventNormal, ash.WindowStateNormal},

		// Fullscreen
		{ash.WMEventFullscreen, ash.WindowStateFullscreen},

		// Reset for other tests
		{ash.WMEventNormal, ash.WindowStateNormal},

		// Remaining tests
		{ash.WMEventMaximize, ash.WindowStateMaximized},
		{ash.WMEventSnapLeft, ash.WindowStateLeftSnapped},
		{ash.WMEventSnapRight, ash.WindowStateRightSnapped},
		{ash.WMEventMinimize, ash.WindowStateMinimized},
	} {
		s.Logf("Sending event %s to Settings app", test.wmEvent)

		if state, err := ash.SetARCAppWindowState(ctx, tconn, pkg, test.wmEvent); err != nil {
			s.Errorf("Failed to set window state to %s for Settings app: %v", test.expectedWindowState, err)
		} else if state != test.expectedWindowState {
			s.Errorf("Unexpected window state: got %s; want %s", state, test.expectedWindowState)
		}
	}
}
