// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VirtualDesks,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests the placement of an ARC app in a virtual desk",
		Contacts: []string{
			"chromeos-wm@google.com",
			"dandersson@chromium.org",
			"arc-framework+tast@google.com",
			"chromeos-sw-engprod@google.com",
		},
		// ChromeOS > Software > Window Management > Virtual Desks
		BugComponent: "b:1238200",
		Attr:         []string{"group:mainline", "group:hw_agnostic", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 30*time.Second,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
			Fixture:           "arcBooted",
			Val:               browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"android_container", "lacros"},
			Fixture:           "lacrosWithArcBooted",
			Val:               browser.TypeLacros,
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			Fixture:           "arcBooted",
			Val:               browser.TypeAsh,
		}, {
			Name:              "lacros_vm",
			ExtraSoftwareDeps: []string{"android_vm", "lacros"},
			Fixture:           "lacrosWithArcBooted",
			Val:               browser.TypeLacros,
		}},
	})
}

// deskContainsWindow returns true if a window whose name is windowName was found as a child of the desk identified by deskIndex.
func deskContainsWindow(ctx context.Context, tconn *chrome.TestConn, deskIndex int, finder *nodewith.Finder) (bool, error) {
	desksInfo, err := ash.GetDesksInfo(ctx, tconn)
	if err != nil {
		return false, errors.Wrap(err, "GetDesksInfoFailed failed")
	}
	if deskIndex >= len(desksInfo.DeskContainers) {
		return false, errors.Errorf("Desk index %d is out of range (%d desks)", deskIndex, len(desksInfo.DeskContainers))
	}
	deskContainerName := desksInfo.DeskContainers[deskIndex]

	// Find the given desk container first.
	deskContainer := nodewith.HasClass(deskContainerName)
	ui := uiauto.New(tconn)
	if err := ui.Exists(deskContainer)(ctx); err != nil {
		return false, errors.Wrapf(err, "failed to locate the given desk container: %s", deskContainerName)
	}

	// Find the given finder inside the desk container.
	return ui.IsNodeFound(ctx, finder.Ancestor(deskContainer))
}

func VirtualDesks(ctx context.Context, s *testing.State) {
	// Reserve few seconds for various cleanup
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	a := s.FixtValue().(*arc.PreData).ARC
	cr := s.FixtValue().(*arc.PreData).Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Make sure the device is in clamshell mode
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure in clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	// Explicitly start a browser window to test that switching to a new desk
	// doesn't cause it to change desks.
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	conn, err := br.NewConn(ctx, "about:blank")
	if err != nil {
		s.Fatal("Could not open the browser window: ", err)
	}
	defer conn.Close()

	ki, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get the keyboard: ", err)
	}

	// Trigger the keyboard shortcut to create a new desk, which also activates
	// the newly created desk.
	if err := ki.Accel(ctx, "Search+Shift+="); err != nil {
		s.Fatal("Failed to send the new desk accelerator: ", err)
	}

	s.Log("Starting the android settings app")

	// Create a Settings activity handle.
	act, err := arc.NewActivity(a, "com.android.settings", ".Settings")
	if err != nil {
		s.Fatal("Failed to create new activity: ", err)
	}
	defer act.Close(ctx)

	// Launch the activity.
	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatal("Failed start Settings activity: ", err)
	}

	// Put the activity in "normal" (non-maximized mode).
	if _, err := ash.SetARCAppWindowState(ctx, tconn, act.PackageName(), ash.WMEventNormal); err != nil {
		s.Fatal("Failed to set window state to Normal: ", err)
	}

	if err := ash.WaitForARCAppWindowState(ctx, tconn, act.PackageName(), ash.WindowStateNormal); err != nil {
		s.Fatal("Failed to wait for window state to become Normal: ", err)
	}

	window, err := ash.GetARCAppWindowInfo(ctx, tconn, act.PackageName())
	if err != nil {
		s.Fatal("Failed to get the window info of the ARC app: ", err)
	}
	windowFinder := nodewith.HasClass(window.Name).Role(role.Window)

	s.Log("Test setup complete. Beginning to verify desk window hierarchy")

	// The settings window should exist on the second desk, while the browser window
	// should be on the first desk.
	for _, tc := range []struct {
		desk   int
		finder *nodewith.Finder
		want   bool
	}{
		{
			desk:   1,
			finder: windowFinder,
			want:   true,
		},
		{
			desk:   0,
			finder: nodewith.NameContaining("about:blank").First(),
			want:   true,
		},
		{
			desk:   0,
			finder: windowFinder,
			want:   false,
		},
	} {
		if found, err := deskContainsWindow(ctx, tconn, tc.desk, tc.finder); err != nil {
			s.Error("deskContainsWindow Failed: ", err)
		} else if found != tc.want {
			if tc.want {
				s.Errorf("Failed to find %s under %d", tc.finder.Pretty(), tc.desk)
			} else {
				s.Errorf("%s should not be under %d", tc.finder.Pretty(), tc.desk)
			}
		}
	}
}
