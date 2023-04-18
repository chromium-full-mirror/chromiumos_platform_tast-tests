// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/lockscreen"
	"chromiumos/tast/local/chrome/uiauto/quicksettings"
	"chromiumos/tast/local/input"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FeaturePodRestrictedAtLockScreen,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies users are not supposed to use the Bluetooth feature pod at lock screen",
		Contacts: []string{
			"cros-connectivity@google.com",
			"cros-conn-test-team@google.com",
			"vivian.tsai@cienet.com",
			"cienet-development@googlegroups.com",
		},
		// ChromeOS > Software > System Services > Connectivity > Bluetooth
		BugComponent: "b:1131776",
		Attr:         []string{"group:bluetooth", "bluetooth_flaky"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.ChromeLoggedIn,
	})
}

// FeaturePodRestrictedAtLockScreen verifies users are not supposed to use the Bluetooth feature pod at lock screen.
func FeaturePodRestrictedAtLockScreen(ctx context.Context, s *testing.State) {
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
	restricted, err := quicksettings.PodRestricted(ctx, tconn, quicksettings.SettingPodBluetooth)
	if err != nil {
		s.Fatal("Failed to check if Bluetooth is restricted: ", err)
	}
	if !restricted {
		s.Fatal("Bluetooth failed to be greyed-out")
	}
}
