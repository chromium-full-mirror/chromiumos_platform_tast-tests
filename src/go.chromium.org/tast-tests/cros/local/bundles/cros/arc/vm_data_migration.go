// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"bytes"
	"context"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mafredri/cdp/rpcc"
	goselinux "github.com/opencontainers/selinux/go-selinux"
	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/datamigration"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/filesystem"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// These archived home data contains capybara.jpg in android-data/data/media/0/Pictures.
	// The file also has xattr user.arc.test set to "xattr-test".
	vmDataMigrationHomeDataPiArm  = "vm_data_migration_pi_arm64"
	vmDataMigrationHomeDataRvcArm = "vm_data_migration_rvc_arm64"

	vmDataMigrationTestImageFilename = "capybara.jpg"
	vmDataMigrationTestTimeout       = 10 * time.Minute

	vmDataMigrationProjinheritTestFilePath = "data/media/0/Pictures/projinherittest"
	inodeFlagPROJINHERIT                   = 0x20000000
	inodeFlagCASEFOLD                      = 0x40000000
	mediaRwIDOnHost                        = 656383
)

type vmDataMigrationTestParams struct {
	poolID       string
	dataFileName string
	resume       bool
}

type vmDataMigrationFileAttributes struct {
	uid          uint32
	gid          uint32
	permission   fs.FileMode
	selinuxLabel string
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
		SoftwareDeps: []string{
			"android_vm",
			"arcvm_data_migration",
			"chrome",
			// ARCVM /data migration currently does not support LVM-enabled devices.
			"no_lvm_stateful_partition",
		},
		Data:    []string{vmDataMigrationTestImageFilename},
		Timeout: vmDataMigrationTestTimeout,
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

// VMDataMigration implements the test scenario of arc.VMDataMigration.
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
		chrome.RemoveNotification(false),
		chrome.ExtraArgs(args...),
	}

	testImageAttrs := signinAndMigrate(ctx, s, creds, chromeOpts, params.resume)

	// Check that the file attributes are correctly migrated by checking the
	// following:
	// * The file attributes of |vmDataMigrationTestImageFilename| (those
	//   included in |vmDataMigrationFileAttributes|, quota project ID, and an
	//   additional user xattr) are correctly set.
	// * PROJINHERIT inode flag of |vmDataMigrationProjinheritTestFilePath| is
	//   set.
	// * CASEFOLD inode flag is set for directories under /data/media.
	// Since some attributes could be modified by ARC, this should be done
	// before restarting ARC.
	verifyMigratedFileAttributes(ctx, s, creds.User, testImageAttrs)

	// Users following the UX flow will reboot the device after the migration,
	// but we only restart Chrome to simplify the test.
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to sign out: ", err)
	}

	reSignInAndVerifyMigration(ctx, s, creds, chromeOpts)
}

// signinAndMigrate signs in to the test account and performs the migration.
// Also returns the file attributes of the |vmDataMigrationTestImageFilename| in
// the pre-migration /data.
func signinAndMigrate(ctx context.Context, s *testing.State, creds chrome.Creds, chromeOpts []chrome.Option, resume bool) vmDataMigrationFileAttributes {
	// The offset for UID and GID shift in virtio-fs /data.
	const (
		androidUIDOffset = 655360
		androidGIDOffset = 655360
	)

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

	// Use a longer timeout for arc.New, as booting with virtio-fs /data takes
	// much longer than usual in some devices.
	a, err := arc.NewWithTimeout(ctx, s.OutDir(), 4*time.Minute, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	// Make sure that virtio-blk /data is disabled.
	isVirtioBlk, err := arc.IsVirtioBlkDataEnabled(ctx)
	if err != nil {
		s.Fatal("Failed to check if virtio-blk /data is disabled: ", err)
	}
	if isVirtioBlk {
		s.Fatal("virtio-blk /data is not disabled")
	}

	// Check that the pre-migration Android data has the image file and some
	// file attributes are correctly set.
	if err := verifyPreMigrationAndroidData(ctx, a, creds.User, s.DataPath(vmDataMigrationTestImageFilename)); err != nil {
		s.Fatal("Failed to verify the pre-migration Android /data: ", err)
	}

	// Set up some files in the migration source with some inode flags set, as
	// inode flags are not preserved throughout vault archive and unarchive.
	if err := createFilesWithInodeFlags(ctx, creds.User); err != nil {
		s.Fatal("Failed to create file with inode flags: ", err)
	}

	// Quota project ID is not preserved throughout vault archive and unarchive,
	// so it could be 0 for all files at this point unless Android has fixed
	// them up. Set quota project ID of |vmDataMigrationTestImageFilename| to
	// |androidQuotaProjectIdForImage| so that we can test the migration of a
	// non-zero quota project ID.
	if err := setQuotaProjectIDForTestImageFile(ctx, creds.User); err != nil {
		s.Fatal("Failed to set quota project ID for the test image file: ", err)
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

	// Preserve the file attributes of a file in the pre-migration data. This
	// will be used to check the correctness of the file attributes in the
	// migrated (virtio-blk) /data.
	attrs, err := getAndroidFileAttributes(ctx, creds.User)
	if err != nil {
		s.Fatal("Failed to get file attributes of pre-migration file: ", err)
	}
	// Because UID and GID are shifted in virtio-fs /data but not in virtio-blk
	// /data, we need to shift them back.
	attrs.uid -= androidUIDOffset
	attrs.gid -= androidGIDOffset
	s.Logf("Pre-migration file attributes: UID=%d, GID=%d, permission=%o, SELinux=%s",
		attrs.uid, attrs.gid, attrs.permission, attrs.selinuxLabel)

	// Go through the migration UX flow. Abort the migration halfway for resume test cases.
	if err := proceedMigrationScreens(ctx, cr, tconn, resume /* abort */); err != nil {
		if err := screenshot.Capture(ctx, filepath.Join(s.OutDir(), "proceed-migration-screen-failed.png")); err != nil {
			testing.ContextLog(ctx, "Failed to take a screenshot: ", err)
		}
		s.Fatal("Failed to go through migration screen for new migrations: ", err)
	}

	if !resume {
		return attrs
	}

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

	return attrs
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
	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	if err := a.WaitForProvisioning(ctx, provisioningTimeout); err != nil {
		s.Fatal("Failed to wait for ARC provisioning: ", err)
	}

	// Check that virtio-blk /data is enabled.
	isVirtioBlk, err := arc.IsVirtioBlkDataEnabled(ctx)
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

	// Check that the content of the image file is correctly migrated.
	if err := verifyAndroidImageFileContent(ctx, a, s.DataPath(vmDataMigrationTestImageFilename)); err != nil {
		s.Error("Failed to verify the content of the image file: ", err)
	}

	// Check that Play Store can be launched successfully.
	if err := apps.Launch(ctx, tconn, apps.PlayStore.ID); err != nil {
		s.Fatal("Failed to launch Play Store: ", err)
	}
	if err := playstore.VerifyPlayStoreWindowPresent(ctx, tconn, time.Minute); err != nil {
		s.Fatal("Failed to ensure Play Store window is present: ", err)
	}
}

func createFilesWithInodeFlags(ctx context.Context, username string) error {
	androidDataDir, err := arc.AndroidDataDir(ctx, username)
	if err != nil {
		return errors.Wrap(err, "failed to get android-data dir")
	}

	// Regression check for b/274833188. Create a file with KEEP_SIZE option of
	// fallocate so that it will have EOFBLOCKS flag on host kernel version 5.4
	// or older. The flag caused migration failure.
	if err := createFileWithEOFBLOCKS(ctx, androidDataDir); err != nil {
		return errors.Wrap(err, "failed to create a file with EOFBLOCKS flag")
	}

	if err := createDirWithPROJINHERIT(ctx, androidDataDir); err != nil {
		return errors.Wrap(err, "failed to create a dir with PROJINHERIT flag")
	}

	return nil
}

// createFileWithEOFBLOCKS creates a file that will have EOFBLOCKS flag set on
// boards with host kernel version 5.4 or older.
func createFileWithEOFBLOCKS(ctx context.Context, androidDataDir string) error {
	const (
		filePathFromAndroidData = "data/media/0/Pictures/eofblockstest"
		fileSize                = 4096
	)

	file, err := os.Create(filepath.Join(androidDataDir, filePathFromAndroidData))
	if err != nil {
		return errors.Wrap(err, "failed to create file")
	}
	defer file.Close()

	// Change UID and GID appropriately so that this file won't be skipped by
	// the migrator.
	if err := file.Chown(mediaRwIDOnHost /* uid */, mediaRwIDOnHost /* gid */); err != nil {
		return errors.Wrap(err, "failed to chown file")
	}

	// On kernel version 5.4 or older, extending a file by calling fallocate()
	// with KEEP_SIZE option attaches EOFBLOCKS flag to the file.
	if err := unix.Fallocate(int(file.Fd()), unix.FALLOC_FL_KEEP_SIZE, 0, fileSize); err != nil {
		return errors.Wrap(err, "failed to fallocate file")
	}

	return nil
}

func createDirWithPROJINHERIT(ctx context.Context, androidDataDir string) error {
	path := filepath.Join(androidDataDir, vmDataMigrationProjinheritTestFilePath)
	if err := os.Mkdir(path, 0755); err != nil {
		return errors.Wrap(err, "failed to mkdir")
	}
	if err := os.Chown(path, mediaRwIDOnHost /* uid */, mediaRwIDOnHost /* gid */); err != nil {
		return errors.Wrap(err, "failed to chown dir")
	}
	flags, err := filesystem.InodeFlags(path)
	if err != nil {
		return errors.Wrap(err, "failed to get inode flags")
	}
	return filesystem.SetInodeFlags(path, inodeFlagPROJINHERIT|flags)
}

func setQuotaProjectIDForTestImageFile(ctx context.Context, username string) error {
	androidDataDir, err := arc.AndroidDataDir(ctx, username)
	if err != nil {
		return errors.Wrap(err, "failed to get android-data dir")
	}
	imageFilePath := filepath.Join(androidDataDir, "data/media/0/Pictures", vmDataMigrationTestImageFilename)
	return filesystem.SetQuotaProjectID(ctx, imageFilePath, arc.ProjectIDExtMediaImage)
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

	// This might return ErrConnClosing as it restarts Chrome.
	err := ui.DoDefault(enterMigrationScreenButton)(ctx)
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

	if err := ui.WaitUntilGone(progressBarNode)(ctx); err != nil {
		return errors.Wrap(err, "migration timed out")
	}

	finishMessage := nodewith.Name(migrationFinishedScreenTitleText).Role(role.StaticText)
	if err := ui.Exists(finishMessage)(ctx); err != nil {
		return errors.Wrap(err, "failed to check migration success screen")
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
	defer keyboard.Close(ctx)

	if err = lockscreen.UnlockWithPassword(ctx, tconn, creds.User, creds.Pass, keyboard, 10*time.Second, chrome.LoginTimeout); err != nil {
		return nil, errors.Wrap(err, "failed to unlock user")
	}

	return cr, nil
}

func verifyPreMigrationAndroidData(ctx context.Context, a *arc.ARC, username, expectedImageDataPath string) error {
	if err := verifyAndroidImageFileContent(ctx, a, expectedImageDataPath); err != nil {
		return errors.Wrap(err, "failed to verify the content of the image file")
	}
	if err := verifyAndroidFileXattr(ctx, username); err != nil {
		return errors.Wrap(err, "failed to verify the xattr of the file")
	}
	return nil
}

func verifyMigratedFileAttributes(ctx context.Context, s *testing.State, username string, attrs vmDataMigrationFileAttributes) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	diskPath, err := arc.GetVirtioBlkDataDiskPath(ctx, username)
	if err != nil || diskPath == "" {
		s.Fatal("Failed to get disk path for virtio-blk /data: ", err)
	}
	cleanupFunc, err := arc.MountVirtioBlkDataDiskImageReadOnlyWithoutSync(ctx, username, diskPath)
	if err != nil {
		s.Fatal("Failed to make Android /data directory available on host: ", err)
	}
	defer cleanupFunc(cleanupCtx)

	// Check that the xattr set to the image file is correctly migrated.
	if err := verifyAndroidFileXattr(ctx, username); err != nil {
		s.Error("Failed to verify the xattr of the file: ", err)
	}

	androidDataDir, err := arc.AndroidDataDir(ctx, username)
	if err != nil {
		s.Fatal("Failed to get path of Android /data directory: ", err)
	}

	// Check that PROJINHERIT flag is correctly migrated.
	flags, err := filesystem.InodeFlags(filepath.Join(androidDataDir, vmDataMigrationProjinheritTestFilePath))
	if err != nil {
		s.Fatal("Failed to get inode flags: ", err)
	}
	if flags&inodeFlagPROJINHERIT == 0 {
		s.Error("PROJINHERIT inode flag was not migrated")
	}
	// Any directories under /data/media should have CASEFOLD flag set.
	if flags&inodeFlagCASEFOLD == 0 {
		s.Error("CASEFOLD inode flag is not set in a directory under /data/media")
	}

	imageFilePath := filepath.Join(androidDataDir, "data/media/0/Pictures", vmDataMigrationTestImageFilename)
	projectID, err := filesystem.QuotaProjectID(ctx, imageFilePath)
	if err != nil {
		s.Fatalf("Failed to get quota project ID of %s: %s", imageFilePath, err)
	}
	if projectID != arc.ProjectIDExtMediaImage {
		s.Errorf("Unexpected quota project ID: got %d, expected %d", projectID, arc.ProjectIDExtMediaImage)
	}

	// Check the other file attributes are correctly migrated.
	newAttrs, err := getAndroidFileAttributes(ctx, username)
	if err != nil {
		s.Fatal("Failed to get file attributes: ", err)
	}
	if newAttrs.uid != attrs.uid {
		s.Errorf("Unexpected UID: got %d, expected %d", newAttrs.uid, attrs.uid)
	}
	if newAttrs.gid != attrs.gid {
		s.Errorf("Unexpected GID: got %d, expected %d", newAttrs.gid, attrs.gid)
	}
	if newAttrs.permission != attrs.permission {
		s.Errorf("Unexpected permission: got %o, expected %o", newAttrs.permission, attrs.permission)
	}
	if newAttrs.selinuxLabel != attrs.selinuxLabel {
		s.Errorf("Unexpected SELinux label: got %s, expected %s", newAttrs.selinuxLabel, attrs.selinuxLabel)
	}
}

func verifyAndroidImageFileContent(ctx context.Context, a *arc.ARC, expectedDataPath string) error {
	expected, err := ioutil.ReadFile(expectedDataPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s", expectedDataPath)
	}

	androidPath := filepath.Join("/storage/emulated/0/Pictures", vmDataMigrationTestImageFilename)
	actual, err := a.ReadFile(ctx, androidPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s", androidPath)
	}
	if !bytes.Equal(actual, expected) {
		return errors.Errorf("content mismatch between %s and the original file", androidPath)
	}
	return nil
}

func verifyAndroidFileXattr(ctx context.Context, username string) error {
	const (
		expectedXattrKey   = "user.arc.test"
		expectedXattrValue = "xattr-test"
	)

	androidDataDir, err := arc.AndroidDataDir(ctx, username)
	if err != nil {
		return errors.Wrap(err, "failed to get path of Android /data directory")
	}
	imageFilePath := filepath.Join(androidDataDir, "data/media/0/Pictures", vmDataMigrationTestImageFilename)

	testXattrValue := ""
	if err := filesystem.GetXattr(imageFilePath, expectedXattrKey, &testXattrValue); err != nil {
		return errors.Wrap(err, "failed to get xattr of the test file")
	}
	if testXattrValue != expectedXattrValue {
		return errors.Errorf("unexpected xattr: got %s, expected %s", testXattrValue, expectedXattrValue)
	}
	return nil
}

// getAndroidFileAttributes returns the file attributes of
// android-data/data/media/0/Pictures/|vmDataMigrationTestImageFilename| under
// the given user's root cryptohome directory.
func getAndroidFileAttributes(ctx context.Context, username string) (vmDataMigrationFileAttributes, error) {
	androidDataDir, err := arc.AndroidDataDir(ctx, username)
	if err != nil {
		return vmDataMigrationFileAttributes{}, errors.Wrap(err, "failed to get path of Android /data directory")
	}
	hostPath := filepath.Join(androidDataDir, "data/media/0/Pictures", vmDataMigrationTestImageFilename)

	fileInfo, err := os.Stat(hostPath)
	if err != nil {
		return vmDataMigrationFileAttributes{}, errors.Wrap(err, "failed to stat the test file from the host")
	}
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return vmDataMigrationFileAttributes{}, errors.New("failed to get the stat of the test file")
	}

	label, err := goselinux.FileLabel(hostPath)
	if err != nil {
		return vmDataMigrationFileAttributes{}, errors.Wrapf(err, "failed to get SELinux label of %s", hostPath)
	}

	return vmDataMigrationFileAttributes{stat.Uid, stat.Gid, fileInfo.Mode().Perm(), label}, nil
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
