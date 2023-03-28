// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/mafredri/cdp/rpcc"

	"chromiumos/tast/common/tape"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/arc/playstore"
	"chromiumos/tast/local/bundles/cros/arc/datamigration"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/lockscreen"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/screenshot"
	"chromiumos/tast/local/upstart"
	"chromiumos/tast/testing"
)

// These archived home data contains capybara.jpg in android-data/data/media/0/Pictures.
const (
	vmDataMigrationHomeDataPiArm  = "vm_data_migration_pi_arm64"
	vmDataMigrationHomeDataRvcArm = "vm_data_migration_rvc_arm64"
	vmDataMigrationFilename       = "capybara.jpg"
	vmDataMigrationTestTimeout    = 10 * time.Minute
)

type vmDataMigrationTestParams struct {
	poolID       string
	dataFileName string
	resume       bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         VMDataMigration,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Performs ARCVM /data migration with P or R virtio-fs /data and verifies Play Store can be launched and user data is migrated",
		Contacts:     []string{"arc-storage@google.com", "youkichihosoi@google.com", "momohatt@google.com"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"android_vm", "chrome"},
		Data:         []string{vmDataMigrationFilename},
		Timeout:      vmDataMigrationTestTimeout,
		VarDeps: []string{
			tape.ServiceAccountVar,
			"ui.signinProfileTestExtensionManifestKey",
		},
		Params: []testing.Param{{
			// Migrate from virtio-fs /data created on ARC P (for arm) without
			// interruption.
			Name: "p_to_r_arm",
			Val: vmDataMigrationTestParams{
				poolID:       tape.ArcDataMigrationUnmanaged,
				dataFileName: vmDataMigrationHomeDataPiArm,
				resume:       false,
			},
			ExtraAttr:         []string{"informational"},
			ExtraData:         []string{vmDataMigrationHomeDataPiArm},
			ExtraSoftwareDeps: []string{"arm"},
		}, {
			// Migrate from virtio-fs /data created on ARC R (for arm) without
			// interruption.
			Name: "r_to_r_arm",
			Val: vmDataMigrationTestParams{
				poolID:       tape.ArcDataMigrationUnmanaged,
				dataFileName: vmDataMigrationHomeDataRvcArm,
				resume:       false,
			},
			ExtraAttr:         []string{"informational"},
			ExtraData:         []string{vmDataMigrationHomeDataRvcArm},
			ExtraSoftwareDeps: []string{"arm"},
		}, {
			// Migrate from virtio-fs /data created on ARC R (for arm). The
			// migration will be interrupted once in the middle and resumed.
			Name: "r_to_r_arm_resume",
			Val: vmDataMigrationTestParams{
				poolID:       tape.ArcDataMigrationUnmanaged,
				dataFileName: vmDataMigrationHomeDataRvcArm,
				resume:       true,
			},
			ExtraAttr:         []string{"informational"},
			ExtraData:         []string{vmDataMigrationHomeDataRvcArm},
			ExtraSoftwareDeps: []string{"arm"},
		}},
	})
}

func VMDataMigration(ctx context.Context, s *testing.State) {
	params := s.Param().(vmDataMigrationTestParams)
	homeDataPath := s.DataPath(params.dataFileName)

	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	// Create an account manager and lease a test account for the duration of the test.
	accHelper, acc, err := tape.NewOwnedTestAccountManager(
		ctx,
		[]byte(s.RequiredVar(tape.ServiceAccountVar)),
		false,
		tape.WithTimeout(int32(vmDataMigrationTestTimeout.Seconds())),
		tape.WithPoolID(params.poolID))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accHelper.CleanUp(cleanupCtx)

	// Ensure to sign out before setting up the pre-migration state.
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to sign out: ", err)
	}

	// Set up the pre-migration data.
	// Fetch archived home data from the cloud storage and unarchive it under the test account's vault before signing in.
	cleanupFunc, err := datamigration.MountVaultWithArchivedHomeData(ctx, homeDataPath, acc.Username, acc.Password)
	if err != nil {
		s.Fatal("Failed to mount home with archived data: ", err)
	}
	defer cleanupFunc(cleanupCtx)

	creds := chrome.Creds{User: acc.Username, Pass: acc.Password}
	args := append(arc.DisableSyncFlags(), "--disable-arc-data-wipe")
	chromeOpts := []chrome.Option{
		chrome.GAIALogin(creds),
		chrome.ARCSupported(),
		chrome.KeepState(),
		chrome.UnRestrictARCCPU(),
		chrome.DisableFeatures("ArcEnableVirtioBlkForData"),
		chrome.EnableFeatures("ArcVmDataMigration"),
		chrome.RemoveNotification(false),
		chrome.ExtraArgs(args...),
	}

	signinAndMigrate(ctx, s, creds, chromeOpts, params.resume)

	reSignInAndVerifyMigration(ctx, s, creds, chromeOpts)
}

func signinAndMigrate(ctx context.Context, s *testing.State, creds chrome.Creds, chromeOpts []chrome.Option, resume bool) {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer func() {
		if cr != nil {
			cr.Close(cleanupCtx)
		}
	}()

	a, err := arc.New(ctx, s.OutDir())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	// Make sure that virtio-blk /data is disabled.
	isVirtioBlk, err := a.IsVirtioBlkDataEnabled(ctx)
	if err != nil {
		s.Fatal("Failed to check if virtio-blk /data is disabled: ", err)
	}
	if isVirtioBlk {
		s.Fatal("virtio-blk /data is not disabled")
	}

	// Check that the pre-migration Android data has the image file.
	if err := verifyAndroidFile(ctx, a, s.DataPath(vmDataMigrationFilename)); err != nil {
		s.Fatal("Failed to verify the file to be migrated: ", err)
	}

	// Connect to Test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Click the desktop notification and enter the migration screen. This will
	// restart Chrome at the end.
	if err := enterMigrationScreen(ctx, cr, tconn); err != nil {
		if err := screenshot.Capture(ctx, filepath.Join(s.OutDir(), "enter-migration-screen-failed.png")); err != nil {
			testing.ContextLog(ctx, "Failed to take a screenshot: ", err)
		}
		s.Fatal("Failed to enter migration screen: ", err)
	}

	// Reconnect to Chrome and Test API.
	if err := cr.Reconnect(ctx); err != nil {
		s.Fatal("Failed to reconnect: ", err)
	}
	if tconn, err = cr.TestAPIConn(ctx); err != nil {
		s.Fatal("Failed to reconnect to test API: ", err)
	}
	// Go through the migration UX flow. Abort the migration halfway for resume test cases.
	if err := proceedMigrationScreens(ctx, cr, tconn, resume /* abort */); err != nil {
		if err := screenshot.Capture(ctx, filepath.Join(s.OutDir(), "proceed-migration-screen-failed.png")); err != nil {
			testing.ContextLog(ctx, "Failed to take a screenshot: ", err)
		}
		s.Fatal("Failed to go through migration screen for new migrations: ", err)
	}

	if resume {
		cr.Close(ctx)

		if cr, err = reSignInChrome(ctx, s, creds, chromeOpts); err != nil {
			s.Fatal("Failed to re-sign in: ", err)
		}

		// When the migration ended halfway in the previous session, Chrome will
		// be immediately restarted after the re-sign in to start the UX flow
		// for resuming migration. Calling PrepareForRestart() here prevents
		// connecting to the old Chrome process.
		if err := chrome.PrepareForRestart(); err != nil {
			s.Fatal("Failed to prepare for restart after re-sign in: ", err)
		}

		// Reconnect to Chrome and Test API.
		if err := cr.Reconnect(ctx); err != nil {
			s.Fatal("Failed to reconnect for resume: ", err)
		}
		if tconn, err = cr.TestAPIConn(ctx); err != nil {
			s.Fatal("Failed to create Test API connection: ", err)
		}

		// Go through the migration UX flow for resumed migrations.
		if err := proceedMigrationScreens(ctx, cr, tconn, false /* abort */); err != nil {
			if err := screenshot.Capture(ctx, filepath.Join(s.OutDir(), "proceed-resume-migration-screen-failed.png")); err != nil {
				testing.ContextLog(ctx, "Failed to take a screenshot: ", err)
			}
			s.Fatal("Failed to go through migration screen for resumed migrations: ", err)
		}
	}

	// Users following the UX flow will reboot the device here, but we only
	// restart Chrome to simplify the test.
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to sign out: ", err)
	}
}

func reSignInAndVerifyMigration(ctx context.Context, s *testing.State, creds chrome.Creds, chromeOpts []chrome.Option) {
	const provisioningTimeout = 5 * time.Minute

	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := reSignInChrome(ctx, s, creds, chromeOpts)
	if err != nil {
		s.Fatal("Failed to re-sign in: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Check that ARC can start.
	a, err := arc.New(ctx, s.OutDir())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	if err := a.WaitForProvisioning(ctx, provisioningTimeout); err != nil {
		s.Fatal("Failed to wait for ARC provisioning: ", err)
	}

	// Check that virtio-blk /data is enabled.
	isVirtioBlk, err := a.IsVirtioBlkDataEnabled(ctx)
	if err != nil {
		s.Fatal("Failed to check if virtio-blk /data is enabled: ", err)
	}
	if !isVirtioBlk {
		s.Fatal("virtio-blk /data is not enabled")
	}

	// Check that pre-migration data doesn't exist.
	if err := verifyHostDataRemoved(ctx, cr); err != nil {
		s.Error("Failed to verify that the host-side /data is removed: ", err)
	}

	// Check that the image file is correctly migrated.
	if err := verifyAndroidFile(ctx, a, s.DataPath(vmDataMigrationFilename)); err != nil {
		s.Error("Failed to verify the migrated file: ", err)
	}

	// Check that Play Store can be launched successfully.
	if err := apps.Launch(ctx, tconn, apps.PlayStore.ID); err != nil {
		s.Fatal("Failed to launch Play Store: ", err)
	}
	if err := playstore.VerifyPlayStoreWindowPresent(ctx, tconn, time.Minute); err != nil {
		s.Fatal("Failed to ensure Play Store window is present: ", err)
	}
}

func enterMigrationScreen(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn) error {
	// UX strings for the pre-migration-screen phase.
	const (
		notificationTitleText          = "ChromeOS update for Android apps"
		notificationButtonText         = "Update now"
		enterMigrationScreenButtonText = "Start update now"
	)

	testing.ContextLog(ctx, "Entering migration screen")

	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)

	notificationDialog := nodewith.HasClass("AshNotificationView").NameStartingWith(notificationTitleText)
	expandButton := nodewith.Name("Expand notification").Role(role.Button).Ancestor(notificationDialog)
	// The notification is collapsed sometimes, expand it.
	if err := uiauto.IfSuccessThen(ui.WaitUntilExists(expandButton), ui.DoDefault(expandButton))(ctx); err != nil {
		return errors.Wrap(err, "failed to expand update notification")
	}
	updateButton := nodewith.Name(notificationButtonText).Role(role.Button).Ancestor(notificationDialog)
	if err := ui.DoDefault(updateButton)(ctx); err != nil {
		return err
	}

	confirmationDialog := nodewith.HasClass("ArcVmDataMigrationConfirmationDialog")
	enterMigrationScreenButton := nodewith.Name(enterMigrationScreenButtonText).Role(role.Button).Ancestor(confirmationDialog)

	// Prepare for Chrome restart triggered by clicking |enterMigrationScreenButton|.
	if err := chrome.PrepareForRestart(); err != nil {
		return errors.Wrap(err, "failed to prepare for Chrome restart")
	}

	// Insert a short sleep so that the following LeftClick will not be ignored
	// by Chrome's unintended click protection.
	// TODO(b/274892285): Disable the protection for Tast test or improve uiauto
	// so that we don't need a sleep here.
	testing.Sleep(ctx, time.Second)

	// This might return ErrConnClosing as it restarts Chrome.
	err := ui.LeftClick(enterMigrationScreenButton)(ctx)
	if err != nil && !errors.Is(err, rpcc.ErrConnClosing) {
		return errors.Wrap(err, "failed to click migration confirmation button")
	}
	return nil
}

func proceedMigrationScreens(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, abort bool) error {
	// UX strings for the migration screens.
	const (
		migrationProgressScreenTitleText = "Installing updates"
		migrationFinishedScreenTitleText = "Finished updating!"
	)

	testing.ContextLog(ctx, "Going through migration screen")

	ui := uiauto.New(tconn).WithTimeout(time.Minute)

	startMigrationButton := nodewith.ClassName("action-button").Role(role.Button)
	if err := ui.DoDefault(startMigrationButton)(ctx); err != nil {
		return err
	}

	start := time.Now()

	inProgressMessage := nodewith.Name(migrationProgressScreenTitleText).Role(role.StaticText)
	if err := ui.WaitUntilExists(inProgressMessage)(ctx); err != nil {
		return err
	}

	// Since we assume that the migration takes at least around 20 seconds for
	// the pre-migration data used in this test, the progress bar should show
	// during the migration.
	progressBarNode := nodewith.Role(role.ProgressIndicator)
	if err := ui.WaitUntilExists(progressBarNode)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for the progress bar to be shown")
	}

	if abort {
		// Restart Chrome as soon as the progress bar is shown. The presence of
		// the progress bar should indicate that the migration has started.
		// NOTE: It is theoretically possible that the migration finishes before
		// this Chrome restart takes place, but that should be unlikely as the
		// migration is assumed to take at least around 20 seconds for the
		// pre-migration data used in this test.
		if err := upstart.RestartJob(ctx, "ui"); err != nil {
			return errors.Wrap(err, "failed to restart Chrome")
		}
		return nil
	}

	finishMessage := nodewith.Name(migrationFinishedScreenTitleText).Role(role.StaticText)
	if err := ui.WaitUntilExists(finishMessage)(ctx); err != nil {
		return err
	}

	testing.ContextLogf(ctx, "Completed migration in %f sec", time.Since(start).Seconds())
	return nil
}

func reSignInChrome(ctx context.Context, s *testing.State, creds chrome.Creds, chromeOpts []chrome.Option) (*chrome.Chrome, error) {
	opts := append(chromeOpts,
		chrome.NoLogin(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	)
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome")
	}

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to re-establish test API connection")
	}

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get keyboard")
	}
	defer keyboard.Close()

	if err = lockscreen.UnlockWithPassword(ctx, tconn, creds.User, creds.Pass, keyboard, 10*time.Second, chrome.LoginTimeout); err != nil {
		return nil, errors.Wrap(err, "failed to unlock user")
	}

	return cr, nil
}

func verifyHostDataRemoved(ctx context.Context, cr *chrome.Chrome) error {
	androidDataDir, err := arc.AndroidDataDir(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get android-data dir")
	}
	if _, err = os.Stat(filepath.Join(androidDataDir, "data/data")); err == nil {
		return errors.New("android-data/data/data still exists")
	}
	if !os.IsNotExist(err) {
		return errors.Wrap(err, "failed to check the non-existence of android-data/data/data")
	}
	return nil
}

func verifyAndroidFile(ctx context.Context, a *arc.ARC, expectedDataPath string) error {
	expected, err := ioutil.ReadFile(expectedDataPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s", expectedDataPath)
	}

	androidPath := filepath.Join("/storage/emulated/0/Pictures", vmDataMigrationFilename)

	return testing.Poll(ctx, func(ctx context.Context) error {
		actual, err := a.ReadFile(ctx, androidPath)
		if err != nil {
			return errors.Wrapf(err, "failed to read %s in Android", androidPath)
		}
		// TODO(b/268293237): Check that file attributes are migrated.
		if !bytes.Equal(actual, expected) {
			return errors.Errorf("content mismatch between %s in Android and the original file", androidPath)
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second})
}
