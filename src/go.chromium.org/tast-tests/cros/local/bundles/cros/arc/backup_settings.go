// Copyright 2024 The ChromiumOS Authors
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
		Func:           BackupSettings,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies ARC++ backup settings work as intended",
		Contacts:       []string{"arcvm-eng@google.com", "niwa@google.com", "jinrongwu@google.com"},
		// ChromeOS > Software > ARC++ > ARCVM
		BugComponent: "b:883059",
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

func BackupSettings(ctx context.Context, s *testing.State) {
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
	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "BackupSettings.webm"), s.HasError)

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

	if err := checkAndroidBackupSettings(ctx, d); err != nil {
		s.Fatal("Failed checking Android Backup Settings: ", err)
	}
}

func checkAndroidBackupSettings(ctx context.Context, arcDevice *androidui.Device) error {
	const (
		timeoutUI       = 30 * time.Second
		scrollClassName = "android.widget.ScrollView"
		newBackupID     = "android:id/switch_widget"
		oldBackupID     = "com.google.android.gms:id/switchWidget"
	)

	// Scroll until system is visible.
	scrollLayout := arcDevice.Object(androidui.ClassName(scrollClassName), androidui.Scrollable(true))
	system := arcDevice.Object(androidui.ClassName("android.widget.TextView"), androidui.TextMatches("(?i)system"), androidui.Enabled(true))
	if err := scrollLayout.WaitForExists(ctx, timeoutUI); err == nil {
		if err := scrollLayout.ScrollTo(ctx, system); err != nil {
			return errors.Wrap(err, "failed to scroll to System")
		}
	}

	if err := system.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click on System")
	}

	backup := arcDevice.Object(androidui.ClassName("android.widget.TextView"), androidui.TextMatches("(?i)backup"), androidui.Enabled(true))
	if err := backup.WaitForExists(ctx, timeoutUI); err != nil {
		return errors.Wrap(err, "failed finding Backup")
	}

	if err := backup.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click Backup")
	}

	turnOnBackupAndSkipPhotos := func() error {
		// Turn on backup. This test expects backup to be turned off.
		// Not finding the button is as critical as not being able to click it.
		backupToggleOn := arcDevice.Object(androidui.ClassName("android.widget.Button"), androidui.TextMatches("(?i)Turn on"), androidui.Enabled(true))
		if err := backupToggleOn.WaitForExists(ctx, time.Second*10); err != nil {
			return errors.Wrap(err, "Backup turn on button doesn't exist")
		}
		if err := backupToggleOn.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click Turn on button")
		}

		// Dismiss Google photos backup step in case it exists, otherwise just log this as not critical
		// (e.g. this is a valid option as not all devices may have Google photos installed).
		photosSkip := arcDevice.Object(androidui.ClassName("android.widget.Button"), androidui.TextMatches("(?i)Skip"), androidui.Enabled(true))
		if err := photosSkip.WaitForExists(ctx, time.Second*10); err != nil {
			testing.ContextLog(ctx, "Skip button is not there, Google photos may not be installed")
		} else if err := photosSkip.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click Skip button")
		}
		return nil
	}

	if err := turnOnBackupAndSkipPhotos(); err != nil {
		return err
	}

	oldBackupUI := false
	backupID := newBackupID
	// backupStatus will check for toggle on/off.
	backupStatus, err := arcDevice.Object(androidui.ID(newBackupID)).IsChecked(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Old backup UI")
		backupStatus, err = arcDevice.Object(androidui.ID(oldBackupID)).IsChecked(ctx)
		if err != nil {
			return err
		}
		oldBackupUI = true
		backupID = oldBackupID
	}
	backupToggle := arcDevice.Object(androidui.ID(backupID))

	if backupStatus {
		// Turn Backup OFF.
		if err := backupToggle.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click backup toggle")
		}

		turnOffBackup := arcDevice.Object(androidui.ClassName("android.widget.Button"), androidui.TextMatches("(?i)turn off & delete"), androidui.Enabled(true))
		if err := turnOffBackup.WaitForExists(ctx, timeoutUI); err != nil {
			return errors.Wrap(err, "failed to find turn off & delete button")
		}

		if err := turnOffBackup.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click turn off & delete button")
		}
	}

	if oldBackupUI {
		if err := backupToggle.Click(ctx); err != nil {
			return errors.Wrap(err, "failed to click backup toggle in Old UI")
		}
	} else {
		if err := turnOnBackupAndSkipPhotos(); err != nil {
			return err
		}
	}

	backupStatus, err = arcDevice.Object(androidui.ID(backupID)).IsChecked(ctx)
	if err != nil {
		return err
	}
	if !backupStatus {
		return errors.New("unable to Turn Backup ON")
	}
	return nil
}
