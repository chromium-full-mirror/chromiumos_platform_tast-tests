// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	vmDataMigrationStateTimeout = 10 * time.Minute
	vmDataMigrationBootTimeout  = 4 * time.Minute

	vmDataMigrationStateTestImageFilename = "capybara.jpg"
)

type vmDataMigrationStateTestParams struct {
	preMigration bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     VMDataMigrationState,
		Desc:     "Verifies ARCVM /data volume types (virtio-fs vs virtio-blk) for pre-migration and newly created profiles on migration-eligible boards",
		Contacts: []string{"arc-storage@google.com", "niwa@google.com", "youkichihosoi@google.com"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{
			"android_vm",
			"arcvm_data_migration",
			"chrome",
			"no_lvm_stateful_partition",
		},
		Data: []string{
			vmDataMigrationStateTestImageFilename,
		},
		Timeout: vmDataMigrationStateTimeout,
		VarDeps: []string{ui.GaiaPoolDefaultVarName},
		Params: []testing.Param{{
			Name: "pre_migration",
			Val: vmDataMigrationStateTestParams{
				preMigration: true,
			},
		}, {
			Name: "new_profile",
			Val: vmDataMigrationStateTestParams{
				preMigration: false,
			},
		}},
	})
}

// VMDataMigrationState tests ARCVM /data volume behaviors on migration-eligible boards.
// 1. pre_migration: Verifies that pre-migration users keep their existing ARC /data on virtio-fs upon subsequent logins.
// 2. new_profile: Verifies that newly created profiles start on a virtio-blk volume.
func VMDataMigrationState(ctx context.Context, s *testing.State) {
	params := s.Param().(vmDataMigrationStateTestParams)
	if params.preMigration {
		testPreMigrationUserData(ctx, s)
	} else {
		testNewProfileData(ctx, s)
	}
}

func testPreMigrationUserData(ctx context.Context, s *testing.State) {
	// Step 1: Initial sign-in with ArcEnableVirtioBlkForData and ArcVmDataMigration disabled to simulate a pre-migration user.
	creds, normalizedUser := signInPreMigrationData(ctx, s)

	// Step 2: Restart Chrome (simulating sign-out and device restart).
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to restart ui job: ", err)
	}

	// Step 3: Re-sign in with standard default features and verify virtio-fs /data and existing files are preserved.
	reSignInAndVerifyPreMigrationData(ctx, s, creds, normalizedUser)
}

func signInPreMigrationData(ctx context.Context, s *testing.State) (chrome.Creds, string) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx,
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
		chrome.ARCSupported(),
		chrome.DisableFeatures("ArcEnableVirtioBlkForData"),
		chrome.DisableFeatures("ArcVmDataMigration"),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	if err := optin.Perform(ctx, cr, tconn); err != nil {
		s.Fatal("Failed to optin to Play Store: ", err)
	}

	a, err := arc.NewWithTimeout(ctx, s.OutDir(), vmDataMigrationBootTimeout, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	// Verify that virtio-blk /data is disabled initially.
	isVirtioBlk, err := arc.IsVirtioBlkDataEnabled(ctx)
	if err != nil {
		s.Fatal("Failed to check if virtio-blk /data is disabled: ", err)
	}
	if isVirtioBlk {
		s.Fatal("virtio-blk /data is unexpectedly enabled on initial boot")
	}

	// Push a test file to Android /data.
	testImageFile := s.DataPath(vmDataMigrationStateTestImageFilename)
	androidDestPath := filepath.Join("/storage/emulated/0/Pictures", vmDataMigrationStateTestImageFilename)
	if err := a.PushFile(ctx, testImageFile, androidDestPath); err != nil {
		s.Fatal("Failed to push test image file: ", err)
	}

	// Verify the file content can be read from Android.
	if err := verifyMigrationStateImageContent(ctx, a, testImageFile); err != nil {
		s.Fatal("Failed to verify image file content after initial push: ", err)
	}

	return cr.Creds(), cr.NormalizedUser()
}

func reSignInAndVerifyPreMigrationData(ctx context.Context, s *testing.State, creds chrome.Creds, normalizedUser string) {
	const provisioningTimeout = 5 * time.Minute

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Re-sign in with standard options without disabling ArcEnableVirtioBlkForData or ArcVmDataMigration.
	// This simulates a pre-migration user logging into the updated ChromeOS version.
	cr, err := chrome.New(ctx,
		chrome.GAIALogin(creds),
		chrome.ARCSupported(),
		chrome.KeepState(),
		chrome.UnRestrictARCCPU(),
		chrome.RemoveNotification(false),
		chrome.DisableFeatures("DeferArcActivationUntilUserSessionStartUpTaskCompletion"),
		chrome.ExtraArgs(arc.DisableSyncFlags()...),
	)
	if err != nil {
		s.Fatal("Failed to re-sign in: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Launch Play Store to trigger ARC activation.
	if err := apps.Launch(ctx, tconn, apps.PlayStore.ID); err != nil {
		s.Fatal("Failed to launch Play Store: ", err)
	}

	// Wait for ARC to start and boot.
	a, err := arc.NewWithTimeout(ctx, s.OutDir(), vmDataMigrationBootTimeout, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	if err := a.WaitForProvisioning(ctx, provisioningTimeout); err != nil {
		s.Fatal("Failed to wait for ARC provisioning: ", err)
	}

	if err := playstore.VerifyPlayStoreWindowPresent(ctx, tconn, time.Minute); err != nil {
		s.Fatal("Failed to ensure Play Store window is present: ", err)
	}

	// Verify that virtio-blk /data is NOT enabled (user must remain on virtio-fs /data).
	isVirtioBlk, err := arc.IsVirtioBlkDataEnabled(ctx)
	if err != nil {
		s.Fatal("Failed to check if virtio-blk /data is enabled: ", err)
	}
	if isVirtioBlk {
		s.Fatal("Pre-migration user was unexpectedly migrated to virtio-blk /data")
	}

	// Check that the host-side android-data directory still exists.
	androidDataDir, err := arc.AndroidDataDir(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get android-data dir: ", err)
	}
	if _, err := os.Stat(filepath.Join(androidDataDir, "data/data")); err != nil {
		s.Error("Expected host-side android-data/data/data to exist, but got: ", err)
	}

	// Check that the test image file is still preserved and intact.
	testImageFile := s.DataPath(vmDataMigrationStateTestImageFilename)
	if err := verifyMigrationStateImageContent(ctx, a, testImageFile); err != nil {
		s.Error("Failed to verify that pre-migration image file is preserved: ", err)
	}
}

func verifyMigrationStateImageContent(ctx context.Context, a *arc.ARC, expectedDataPath string) error {
	expected, err := os.ReadFile(expectedDataPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s", expectedDataPath)
	}

	androidPath := filepath.Join("/storage/emulated/0/Pictures", vmDataMigrationStateTestImageFilename)
	actual, err := a.ReadFile(ctx, androidPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s", androidPath)
	}
	if !bytes.Equal(actual, expected) {
		return errors.Errorf("content mismatch between %s and original file", androidPath)
	}
	return nil
}

func testNewProfileData(ctx context.Context, s *testing.State) {
	const provisioningTimeout = 5 * time.Minute

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Step 1: Sign in as a new user with standard default features.
	cr, err := chrome.New(ctx,
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
		chrome.ARCSupported(),
		chrome.UnRestrictARCCPU(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	// Step 2: Opt in to Play Store.
	if err := optin.Perform(ctx, cr, tconn); err != nil {
		s.Fatal("Failed to optin to Play Store: ", err)
	}

	// Step 3: Boot ARC and wait for provisioning.
	a, err := arc.NewWithTimeout(ctx, s.OutDir(), vmDataMigrationBootTimeout, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	if err := a.WaitForProvisioning(ctx, provisioningTimeout); err != nil {
		s.Fatal("Failed to wait for ARC provisioning: ", err)
	}

	// Step 4: Verify that virtio-blk /data is enabled for newly created profiles.
	isVirtioBlk, err := arc.IsVirtioBlkDataEnabled(ctx)
	if err != nil {
		s.Fatal("Failed to check if virtio-blk /data is enabled: ", err)
	}
	if !isVirtioBlk {
		s.Fatal("virtio-blk /data is not enabled for newly created profile")
	}

	// Step 5: Verify that the virtio-blk disk image exists.
	diskPath, err := arc.GetVirtioBlkDataDiskPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get disk path for virtio-blk /data: ", err)
	}
	if diskPath == "" {
		s.Fatal("Virtio-blk disk image path was not found")
	}

	// Step 6: Verify that host-side android-data directory does not contain virtio-fs /data.
	androidDataDir, err := arc.AndroidDataDir(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get android-data dir: ", err)
	}
	if _, err := os.Stat(filepath.Join(androidDataDir, "data/data")); err == nil {
		s.Error("Expected host-side android-data/data/data to not exist for virtio-blk user")
	} else if !os.IsNotExist(err) {
		s.Error("Unexpected error checking host-side android-data: ", err)
	}
}
