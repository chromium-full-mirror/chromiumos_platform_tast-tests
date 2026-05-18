// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/bluetooth/facade"
	"go.chromium.org/tast-tests/cros/local/bluetooth/facade/common"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const androidVMT string = "android_vm_t"

const apkName = "customized_arc_app_release_20240704.apk"

func init() {
	testing.AddTest(&testing.Test{
		Func: EnableBluetoothWithArcApp,
		Desc: "Verify that user can turn Bluetooth on with an ARC++ app",
		Contacts: []string{
			"cros-device-enablement@google.com",
			"chromeos-connectivity-engprod@google.com",
		},
		BugComponent:    "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		LifeCycleStage:  testing.LifeCycleOwnerMonitored,
		Attr:            []string{"group:bluetooth", "bluetooth_floss", "group:release-health", "release-health_bt"},
		SoftwareDeps:    []string{"chrome", "arc"},
		TestBedDeps:     []string{tbdep.BluetoothStateNormal},
		Data:            []string{apkName},
		Fixture:         "arcBootedWithBluetoothFloss",
		VariantCategory: `{"name": "BT_Chipset_Kernel"}`,
		Params: []testing.Param{{
			Name:              "android_vm_t",
			ExtraSoftwareDeps: []string{androidVMT},
			Val:               androidVMT,
		}},
		Timeout: 3*time.Minute + apputil.InstallationTimeout,
	})
}

// EnableBluetoothWithArcApp verifies that user can turn Bluetooth on with an ARC++ app.
func EnableBluetoothWithArcApp(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arc.PreData).Chrome

	androidDep := s.Param().(string)
	btFacade, err := facade.NewBluetoothFacade(ctx, common.BluetoothStackTypeFloss)
	if err != nil {
		s.Fatalf("Failed to initialize %s bluetooth facade: %v", common.BluetoothStackTypeFloss, err)
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
		appName                   = "Test Bluetooth ARC app for Tast"
		pkgName                   = "com.example.testbluetootharcappfortast"
		idPrefix                  = pkgName + ":id/"
		bluetoothStatusID         = idPrefix + "txt_bt_activation"
		turnOffBluetoothButtonID  = idPrefix + "btn_turn_off_bt"
		turnOnBluetoothButtonID   = idPrefix + "btn_turn_on_bt"
		permissionStatusID        = idPrefix + "txt_bt_permission"
		requestPermissionButtonID = idPrefix + "btn_request_bt"

		defaultUITimeout = 15 * time.Second
	)

	recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn, cr)
	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "screen_recording.webm"), s.HasError)

	app, err := apputil.NewApp(ctx, kb, tconn, a, d, appName, pkgName)
	if err != nil {
		s.Fatal("Failed to create the instance of app: ", err)
	}

	if err := a.Install(ctx, s.DataPath(apkName)); err != nil {
		s.Fatal("Failed to install the APK: ", err)
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

	// The texts are different across Android versions.
	var allowBluetoothObj *ui.Object
	switch androidDep {
	case androidVMT:
		allowBluetoothObj = d.Object(ui.Text("Allow"))
	default:
		s.Fatal("Unsupported ARC type: ", androidDep)
	}

	requestPermissionObj := d.Object(ui.ID(requestPermissionButtonID), ui.Text("Request"))
	permissionStatusObj := d.Object(ui.ID(permissionStatusID), ui.Text("OK"))
	turnOnBluetoothObj := d.Object(ui.ID(turnOnBluetoothButtonID), ui.Text("Turn On"))
	turnOffBluetoothObj := d.Object(ui.ID(turnOffBluetoothButtonID), ui.Text("Turn Off"))
	bluetoothOnStatusObj := d.Object(ui.ID(bluetoothStatusID), ui.Text("Bluetooth: On"))
	bluetoothOffStatusObj := d.Object(ui.ID(bluetoothStatusID), ui.Text("Bluetooth: Off"))

	if err := uiauto.Combine("requesting Bluetooth permission",
		// According to Android guidelines, Bluetooth permission is needed to ensure user awareness and consent.
		apputil.FindAndClick(requestPermissionObj, defaultUITimeout),
		// User consent of Bluetooth feature is only required on Android 12 (API level 31) or higher.
		// That means, android_vm_t (API level 32) will have this prompt, but not for android_r (API level 30).
		apputil.ClickIfExist(allowBluetoothObj, defaultUITimeout),
		apputil.WaitForExists(permissionStatusObj, defaultUITimeout),
	)(ctx); err != nil {
		s.Fatalf("Failed to request Bluetooth permission from ARC++ app %q: %v", app.AppName, err)
	}

	if err := uiauto.Combine("turning Bluetooth on",
		// This status does not sync with the Bluetooth state on DUT when the app is just launched
		// ensuring it is "OFF" to proceed since we have turned the Bluetooth off earlier.
		apputil.WaitForExists(bluetoothOffStatusObj, defaultUITimeout),
		apputil.FindAndClick(turnOnBluetoothObj, defaultUITimeout),
		apputil.FindAndClick(allowBluetoothObj, defaultUITimeout),
		apputil.WaitForExists(bluetoothOnStatusObj, defaultUITimeout),
	)(ctx); err != nil {
		s.Fatalf("Failed to turn Bluetooth on from ARC++ app %q: %v", app.AppName, err)
	}

	// Verify that ARC++ app can turn ChromeOS Bluetooth on.
	if isBtOn, err := btFacade.IsPoweredOn(ctx); err != nil {
		s.Fatal("Failed to check if Bluetooth is ON: ", err)
	} else if !isBtOn {
		s.Fatal("Bluetooth is not ON")
	}

	if err := uiauto.Combine("turning Bluetooth off",
		apputil.FindAndClick(turnOffBluetoothObj, defaultUITimeout),
		apputil.FindAndClick(allowBluetoothObj, defaultUITimeout),
		apputil.WaitForExists(bluetoothOffStatusObj, defaultUITimeout),
	)(ctx); err != nil {
		s.Fatalf("Failed to turn Bluetooth off from ARC++ app %q: %v", app.AppName, err)
	}

	// Verify that ARC++ app cannot turn ChromeOS Bluetooth off.
	if isBtOn, err := btFacade.IsPoweredOn(ctx); err != nil {
		s.Fatal("Failed to check if Bluetooth is ON: ", err)
	} else if !isBtOn {
		s.Fatal("Bluetooth is not ON")
	}
}
