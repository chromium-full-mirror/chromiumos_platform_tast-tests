// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/bluetooth/facade"
	"go.chromium.org/tast-tests/cros/local/bluetooth/facade/common"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
)

const (
	androidP   string = "android_p"
	androidR   string = "android_r"
	androidVMT string = "android_vm_t"
)

// Match the old version string of "1.x.x", "2.0.x" or "2.1.x".
var regexpOldVersionOfBluetoothApp = regexp.MustCompile(`^(1\.\d+|2\.(0|1))\.\d+`)

type testParam struct {
	stackType  common.BluetoothStackType
	androidDep string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         EnableBluetoothWithArcApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that user can turn Bluetooth on with an ARC++ app",
		Contacts: []string{
			// "cros-connectivity@google.com",
			// "chromeos-connectivity-engprod@google.com",
			"kinwang.lao@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		// ChromeOS > Software > System Services > Connectivity > Bluetooth
		BugComponent:   "b:1131776",
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Attr:           []string{"group:bluetooth"},
		SoftwareDeps:   []string{"chrome", "arc"},
		Params: []testing.Param{{
			Name:              "android_p_bluez",
			Fixture:           "arcBootedWithPlayStoreAndBluetoothBlueZ",
			ExtraAttr:         []string{"bluetooth_flaky"},
			ExtraSoftwareDeps: []string{androidP},
			Val: &testParam{
				stackType:  common.BluetoothStackTypeBluez,
				androidDep: androidP,
			},
		}, {
			Name:              "android_p_floss",
			Fixture:           "arcBootedWithPlayStoreAndBluetoothFloss",
			ExtraAttr:         []string{"bluetooth_floss_flaky"},
			ExtraSoftwareDeps: []string{"bluetooth_floss", androidP},
			Val: &testParam{
				stackType:  common.BluetoothStackTypeFloss,
				androidDep: androidP,
			},
		}, {
			Name:              "android_r_bluez",
			Fixture:           "arcBootedWithPlayStoreAndBluetoothBlueZ",
			ExtraAttr:         []string{"bluetooth_flaky"},
			ExtraSoftwareDeps: []string{androidR},
			Val: &testParam{
				stackType:  common.BluetoothStackTypeBluez,
				androidDep: androidR,
			},
		}, {
			Name:              "android_r_floss",
			Fixture:           "arcBootedWithPlayStoreAndBluetoothFloss",
			ExtraAttr:         []string{"bluetooth_floss_flaky"},
			ExtraSoftwareDeps: []string{"bluetooth_floss", androidR},
			Val: &testParam{
				stackType:  common.BluetoothStackTypeFloss,
				androidDep: androidR,
			},
		}, {
			Name:              "android_vm_t_bluez",
			Fixture:           "arcBootedWithPlayStoreAndBluetoothBlueZ",
			ExtraAttr:         []string{"bluetooth_flaky"},
			ExtraSoftwareDeps: []string{androidVMT},
			Val: &testParam{
				stackType:  common.BluetoothStackTypeBluez,
				androidDep: androidVMT,
			},
		}, {
			Name:              "android_vm_t_floss",
			Fixture:           "arcBootedWithPlayStoreAndBluetoothFloss",
			ExtraAttr:         []string{"bluetooth_floss_flaky"},
			ExtraSoftwareDeps: []string{"bluetooth_floss", androidVMT},
			Val: &testParam{
				stackType:  common.BluetoothStackTypeFloss,
				androidDep: androidVMT,
			},
		}},
		Timeout: 3*time.Minute + apputil.InstallationTimeout,
	})
}

// EnableBluetoothWithArcApp verifies that user can turn Bluetooth on with an ARC++ app.
func EnableBluetoothWithArcApp(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arc.PreData).Chrome

	androidDep := s.Param().(*testParam).androidDep
	stackType := s.Param().(*testParam).stackType
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
		pkgName = "com.manjul.bluetoothsdp"

		idPrefix          = pkgName + ":id/"
		bluetoothSwitchID = idPrefix + "on_off_switch"

		defaultUITimeout = 15 * time.Second
		retryTimes       = 3
	)

	recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn)
	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "screen_recording.webm"), s.HasError)

	// We do not provide a name when installing the app since it is not required.
	// Further, the name as shown in the Play Store may not match the actual name of the app itself.
	// To handle this edge-case we determine the correct name to use based on the version of the app that we install.
	app, err := apputil.NewApp(ctx, kb, tconn, a, d, "" /* appName */, pkgName)
	if err != nil {
		s.Fatal("Failed to create the instance of app: ", err)
	}

	if err := uiauto.Retry(retryTimes, func(ctx context.Context) error {
		if err := app.Install(ctx); err != nil {
			if err := optin.ClosePlayStore(ctx, tconn); err != nil {
				s.Log("Failed to close Play Store: ", err)
			}
			return err
		}
		return nil
	})(ctx); err != nil {
		s.Fatal("Failed to install the bluetooth app: ", err)
	}

	appName, err := appNameBasedOnVersion(ctx, app)
	if err != nil {
		s.Fatal("Failed to get the app name: ", err)
	}
	app.AppName = appName

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
	turnOffBluetoothObj := d.Object(ui.ResourceID(bluetoothSwitchID), ui.Checked(true))
	denyNotificationObj := d.Object(ui.TextMatches("Don’t allow"))

	var (
		allowBluetoothObj *ui.Object
		denyLocationObj   *ui.Object
	)
	// The texts are different across Android versions.
	switch androidDep {
	case androidP:
		allowBluetoothObj = d.Object(ui.Text("ALLOW"))
		denyLocationObj = d.Object(ui.Text("DENY"))
	case androidR:
		allowBluetoothObj = d.Object(ui.Text("Allow"))
		denyLocationObj = d.Object(ui.Text("Deny"))
	case androidVMT:
		allowBluetoothObj = d.Object(ui.Text("Allow"))
		denyLocationObj = d.Object(ui.Text("Don’t allow"))
	default:
		s.Fatal("Unsupported ARC type: ", androidDep)
	}

	// Turning Bluetooth on from ARC++ app.
	if err := uiauto.Combine("turn Bluetooth on",
		// Check for potential notification permission popup and deny it as notifications are not needed
		// for upcoming test.
		apputil.ClickIfExist(denyNotificationObj, defaultUITimeout),
		// This switch does not sync with the Bluetooth state on DUT when the app is just launched,
		// ensuring it is "OFF" to proceed since we have turned the Bluetooth off earlier.
		apputil.ClickIfExist(turnOffBluetoothObj, defaultUITimeout),
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

// appNameBasedOnVersion returns the app name based on the version of the app.
//
// The method is included as the name of the app can be different across different versions and the version of the app available on the Play Store could be different as well.
// The known mapping of app name to app version is:
//   - Bluetooth Pair: 2.1.x and before
//   - Bluetooth Finder: 2.2.0 and later
//
// This method can be deprecated once the the app available on the Play Store is fixed.
func appNameBasedOnVersion(ctx context.Context, app *apputil.App) (string, error) {
	appVersion, err := app.GetVersion(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to get app version")
	}

	const (
		oldAppName = "Bluetooth Pair"
		newAppName = "Bluetooth Finder"
	)
	if regexpOldVersionOfBluetoothApp.MatchString(appVersion) {
		return oldAppName, nil
	}
	return newAppName, nil
}
