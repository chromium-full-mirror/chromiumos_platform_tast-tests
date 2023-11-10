// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/bluetooth/facade"
	"go.chromium.org/tast-tests/cros/local/bluetooth/facade/common"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
)

type testParam struct {
	stackType common.BluetoothStackType
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           EnableBluetoothWithArcApp,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verify that user can turn Bluetooth on with an ARC++ app",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"kinwang.lao@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		// ChromeOS > Software > System Services > Connectivity > Bluetooth
		BugComponent: "b:1131776",
		Attr:         []string{"group:bluetooth"},
		SoftwareDeps: []string{"chrome", "arc"},
		Params: []testing.Param{{
			Name:      "bluez",
			Fixture:   "arcBootedWithPlayStoreAndBluetoothBlueZ",
			ExtraAttr: []string{"bluetooth_flaky"},
			Val: testParam{
				stackType: common.BluetoothStackTypeBluez,
			},
		}, {
			Name:              "floss",
			Fixture:           "arcBootedWithPlayStoreAndBluetoothFloss",
			ExtraAttr:         []string{"bluetooth_floss_flaky"},
			ExtraSoftwareDeps: []string{"bluetooth_floss"},
			Val: testParam{
				stackType: common.BluetoothStackTypeFloss,
			},
		}},
		Timeout: 3*time.Minute + apputil.InstallationTimeout,
	})
}

// EnableBluetoothWithArcApp verifies that user can turn Bluetooth on with an ARC++ app.
func EnableBluetoothWithArcApp(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arc.PreData).Chrome

	stackType := s.Param().(testParam).stackType
	btFacade, err := facade.NewBluetoothFacade(ctx, stackType)
	if err != nil {
		s.Fatalf("Failed to initialize %s bluetooth facade: %v", stackType, err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard controller: ", err)
	}
	defer kb.Close(cleanupCtx)

	a := s.FixtValue().(*arc.PreData).ARC
	d := s.FixtValue().(*arc.PreData).UIDevice

	const (
		// The ARC++ app used for controlling Bluetooth.
		appName = "Bluetooth Pair"
		pkgName = "com.manjul.bluetoothsdp"

		idPrefix          = pkgName + ":id/"
		bluetoothSwitchID = idPrefix + "on_off_switch"

		defaultUITimeout = 15 * time.Second
	)

	app, err := apputil.NewApp(ctx, kb, tconn, a, d, appName, pkgName)
	if err != nil {
		s.Fatal("Failed to create the instance of app: ", err)
	}

	if err := app.Install(ctx); err != nil {
		s.Fatalf("Failed to install %q: %v", app.AppName, err)
	}

	s.Log("Turning Bluetooth off")
	if err := btFacade.SetPowered(ctx, false); err != nil {
		s.Fatal("Failed to turn Bluetooth off")
	}
	defer btFacade.SetPowered(cleanupCtx, true)

	if _, err = app.Launch(ctx); err != nil {
		s.Fatalf("Failed to launch %q: %v", app.AppName, err)
	}
	defer app.Close(cleanupCtx, cr, s.HasError, s.OutDir())

	if err := apputil.DismissMobilePrompt(ctx, tconn); err != nil {
		s.Fatal("Failed to dismiss 'designed for mobile' prompt: ", err)
	}

	turnOnBluetoothObj := d.Object(ui.ResourceID(bluetoothSwitchID), ui.Checked(false))
	// This text is different across architectures, "ALLOW" in ARC and "Allow" in ARCVM.
	allowBluetoothObj := d.Object(ui.TextMatches("(?i)Allow"))
	// This text is different across architectures, "DENY" in ARC, "Deny" in ARCVM or "Don’t allow".
	denyLocationObj := d.Object(ui.TextMatches("(?i)Deny|Don’t allow"))

	// Turning Bluetooth on from ARC++ app.
	if err := uiauto.Combine("turn Bluetooth on",
		apputil.FindAndClick(turnOnBluetoothObj, defaultUITimeout),
		apputil.FindAndClick(allowBluetoothObj, defaultUITimeout),
		// Deny location permission as scanning for nearby devices feature is not relevant for upcoming test.
		apputil.FindAndClick(denyLocationObj, defaultUITimeout),
	)(ctx); err != nil {
		s.Fatalf("Failed to turn Bluetooth on from ARC++ app %q: %v", app.AppName, err)
	}

	// Verify that ARC++ app can turn ChromeOS Bluetooth on.
	if isBtOn, err := btFacade.IsPoweredOn(ctx); err != nil {
		s.Fatal("Failed to check if Bluetooth is ON: ", err)
	} else if !isBtOn {
		s.Fatal("Bluetooth is not ON")
	}

	// Turning Bluetooth off from ARC++ app.
	turnOffBluetoothObj := d.Object(ui.ResourceID(bluetoothSwitchID), ui.Checked(true))
	if err := apputil.FindAndClick(turnOffBluetoothObj, defaultUITimeout)(ctx); err != nil {
		s.Fatal("Failed to click the Bluetooth toggle: ", err)
	}

	// Verify that ARC++ app cannot turn ChromeOS Bluetooth off.
	if isBtOn, err := btFacade.IsPoweredOn(ctx); err != nil {
		s.Fatal("Failed to check if Bluetooth is ON: ", err)
	} else if !isBtOn {
		s.Fatal("Bluetooth is not ON")
	}
}
