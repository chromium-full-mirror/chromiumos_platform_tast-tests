// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bluetooth"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           PairNewDeviceFromBluetoothSettings,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Checks that the pairing dialog can be opened from the Bluetooth Settings sub-page",
		Contacts: []string{
			"cros-device-enablement@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent:    "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr:            []string{"group:bluetooth", "bluetooth_floss", "group:release-health", "release-health_bt"},
		SoftwareDeps:    []string{"chrome"},
		Fixture:         "bluetoothEnabledWithFloss",
		TestBedDeps:     []string{tbdep.BluetoothStateNormal},
		VariantCategory: `{"name": "BT_Chipset_Kernel"}`,
	})
}

// PairNewDeviceFromBluetoothSettings tests that a user can successfully open
// the pairing dialog from the "Pair new device" button on the Bluetooth
// Settings sub-page.
func PairNewDeviceFromBluetoothSettings(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(bluetooth.HasTconn).Tconn()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	bt := s.FixtValue().(bluetooth.HasBluetoothImpl).BluetoothImpl()

	app, err := ossettings.NavigateToBluetoothSettingsSubpage(ctx, tconn, bt)
	if err != nil {
		s.Fatal("Failed to show the Bluetooth Settings sub-page: ", err)
	}
	defer app.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	ui := uiauto.New(tconn)

	if err := ui.LeftClickUntil(
		ossettings.BluetoothPairNewDeviceButton,
		ui.Exists(ossettings.BluetoothPairNewDeviceModal),
	)(ctx); err != nil {
		s.Fatal("Failed to open the \"Pair new device\" dialog: ", err)
	}
}
