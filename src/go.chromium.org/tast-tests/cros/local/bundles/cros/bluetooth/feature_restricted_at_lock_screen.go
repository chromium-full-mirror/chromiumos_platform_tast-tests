// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           FeatureRestrictedAtLockScreen,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies users are not able to use the quick settings Bluetooth feature at the lock screen",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		// ChromeOS > Software > System Services > Connectivity > Bluetooth
		BugComponent: "b:1131776",
		Attr:         []string{"group:bluetooth"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name:      "floss_disabled",
			Fixture:   "bluetoothEnabledWithBlueZ",
			ExtraAttr: []string{"bluetooth_sa"},
		}, {
			Name:      "floss_enabled",
			Fixture:   "bluetoothEnabledWithFloss",
			ExtraAttr: []string{"bluetooth_floss"},
		}},
	})
}

// FeatureRestrictedAtLockScreen verifies users are not able to use the quick settings Bluetooth feature at the lock screen.
func FeatureRestrictedAtLockScreen(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create a keyboard: ", err)
	}
	defer kb.Close(ctx)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if err := lockscreen.Lock(ctx, tconn); err != nil {
		s.Fatal("Failed to lock the screen: ", err)
	}
	defer lockscreen.EnterPassword(cleanupCtx, tconn, cr.Creds().User, cr.Creds().Pass, kb)

	if err := quicksettings.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to show Quick Settings: ", err)
	}
	defer quicksettings.Hide(cleanupCtx, tconn)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	// Users are not supposed to pair Bluetooth devices at lock screen.
	// This attempt should be restricted by the disabled Bluetooth feature pod.
	restricted, err := quicksettings.TileRestricted(ctx, tconn, quicksettings.FeatureTileBluetooth)
	if err != nil {
		s.Fatal("Failed to check if Bluetooth is restricted: ", err)
	}
	if !restricted {
		s.Fatal("Bluetooth failed to be greyed-out")
	}
}
