// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"path/filepath"
	"time"

	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           VerifySettings,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies ARC++ settings work as intended",
		Contacts:       []string{"cros-arc-te@google.com", "arc-core@google.com", "jinrongwu@google.com", "mattlui@google.com"},
		// ChromeOS > Software > ARC++ > EngProd
		BugComponent: "b:1052117",
		Attr:         []string{"group:arc", "arc_core", "group:arc-functional"},
		SoftwareDeps: []string{"chrome", "gaia"},
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraAttr:         []string{"group:hw_agnostic"},
				ExtraSoftwareDeps: []string{"android_vm"},
			}},
		Timeout: chrome.GAIALoginTimeout + arc.BootTimeout + 120*time.Second,
		VarDeps: []string{ui.GaiaPoolDefaultVarName},
	})
}

func VerifySettings(ctx context.Context, s *testing.State) {
	// Give 30 seconds to clean up and dump out UI tree.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx,
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
		chrome.ARCSupported(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Optin to PlayStore and Close
	if err := optin.PerformAndClose(ctx, cr, tconn); err != nil {
		s.Fatal("Failed to optin to Play Store and Close: ", err)
	}

	recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn, cr)
	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "VerifySettings.webm"), s.HasError)

	// Setup ARC.
	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer d.Close(cleanupCtx)

	ui := uiauto.New(tconn)
	playStoreButton := nodewith.Name("Google Play Store").Role(role.Button)
	if _, err := ossettings.LaunchAtPageURL(ctx, tconn, cr, "apps", ui.Exists(playStoreButton)); err != nil {
		s.Fatal("Failed to launch apps settings page: ", err)
	}

	androidSettingsLink := nodewith.Name("Android Settings").Role(role.Link)
	if err := uiauto.Combine("Open Android Settings",
		ui.FocusAndWait(playStoreButton),
		ui.LeftClickUntil(playStoreButton, ui.Exists(androidSettingsLink)),
		ui.LeftClick(androidSettingsLink),
	)(ctx); err != nil {
		s.Fatal("Failed to Open Android Settings : ", err)
	}

	if err := checkAndroidSettings(ctx, d); err != nil {
		s.Fatal("Failed checking Android Settings: ", err)
	}
}

func checkAndroidSettings(ctx context.Context, arcDevice *androidui.Device) error {
	const (
		timeoutUI       = 30 * time.Second
		scrollClassName = "android.widget.ScrollView"
		locationIDT     = "android:id/switch_widget"
		locationIDPreT  = "com.android.settings:id/switch_widget"
	)

	// Scroll until system is visible.
	scrollLayout := arcDevice.Object(androidui.ClassName(scrollClassName), androidui.Scrollable(true))
	system := arcDevice.Object(androidui.ClassName("android.widget.TextView"), androidui.TextMatches("(?i)system"), androidui.Enabled(true))
	if err := scrollLayout.WaitForExists(ctx, timeoutUI); err == nil {
		if err := scrollLayout.ScrollTo(ctx, system); err != nil {
			return errors.Wrap(err, "failed to scroll to System")
		}
	}

	aboutDevice := arcDevice.Object(androidui.ClassName("android.widget.TextView"), androidui.TextMatches("(?i)about device"), androidui.Enabled(true))
	scrollLayout = arcDevice.Object(androidui.ClassName(scrollClassName), androidui.Scrollable(true))
	if err := scrollLayout.WaitForExists(ctx, timeoutUI); err == nil {
		testing.ContextLog(ctx, "Scroll to About device")
		if err := scrollLayout.ScrollTo(ctx, aboutDevice); err != nil {
			return errors.Wrap(err, "failed to scroll to About device")
		}
	}

	if err := aboutDevice.WaitForExists(ctx, timeoutUI); err != nil {
		return errors.Wrap(err, "failed finding About Device Text View")
	}

	if err := aboutDevice.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click About Device")
	}

	buildNumber := arcDevice.Object(androidui.ClassName("android.widget.TextView"), androidui.TextMatches("(?i)build number"), androidui.Enabled(true))
	// On T and potentially other Android flavors, `buildNumber` can be found at the end of the menu, scrolling on a best effort capacity.
	if err := scrollLayout.WaitForExists(ctx, timeoutUI); err == nil {
		scrollLayout.ScrollTo(ctx, buildNumber)
	}

	if err := buildNumber.WaitForExists(ctx, timeoutUI); err != nil {
		return errors.Wrap(err, "failed finding Build Number TextView")
	}

	backButton := arcDevice.Object(androidui.ClassName("android.widget.ImageButton"), androidui.Enabled(true))

	if err := backButton.WaitForExists(ctx, timeoutUI); err != nil {
		return errors.Wrap(err, "failed finding Back Button")
	}

	if err := backButton.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click Back Button")
	}

	if err := system.WaitForExists(ctx, timeoutUI); err != nil {
		return errors.Wrap(err, "failed finding System Text View")
	}

	if err := system.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click on System")
	}

	developerOptions := arcDevice.Object(androidui.ClassName("android.widget.TextView"), androidui.TextMatches("(?i)developer options"), androidui.Enabled(true))
	if err := developerOptions.WaitForExists(ctx, timeoutUI); err != nil {
		return errors.Wrap(err, "failed finding Developer Options")
	}

	if err := backButton.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click Back Button")
	}

	location := arcDevice.Object(androidui.ClassName("android.widget.TextView"), androidui.TextMatches("(?i)location"), androidui.Enabled(true))
	if err := location.WaitForExists(ctx, timeoutUI); err != nil {
		return errors.Wrap(err, "failed finding Location TextView")
	}

	if err := location.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click Location")
	}

	// Location preference is build with different UI elements based on Android version.
	version, err := arc.SDKVersion()
	if err != nil {
		return errors.Wrap(err, "failed to get ARC version")
	}
	locationID := locationIDPreT
	if version >= arc.SDKT {
		locationID = locationIDT
	}

	// locationStatus will check for toggle On/Off
	locationStatus, err := arcDevice.Object(androidui.ID(locationID)).IsChecked(ctx)
	if err != nil {
		return errors.Wrap(err, "Use location toggle cannot be found")
	}
	locationToggle := arcDevice.Object(androidui.ID(locationID))

	if locationStatus {
		// Turn Location Off.
		if err := locationToggle.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click Location toggle")
		}
	}

	turnOnlocation := arcDevice.Object(androidui.ClassName("android.widget.Button"), androidui.TextMatches("(?i)TURN ON LOCATION"), androidui.Enabled(true))
	if err := turnOnlocation.WaitForExists(ctx, timeoutUI); err == nil {
		if err := turnOnlocation.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click TURN ON LOCATION")
		}
	} else {
		if err := locationToggle.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click Location toggle button")
		}
	}

	// locationStatus will check for toggle On/Off
	locationStatus, err = arcDevice.Object(androidui.ID(locationID)).IsChecked(ctx)
	if err != nil {
		return err
	}
	if !locationStatus {
		return errors.New("Unable to Turn Location ON")
	}

	return nil
}
