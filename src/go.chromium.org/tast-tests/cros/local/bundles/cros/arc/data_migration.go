// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/arcent"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/datamigration"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/retry"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// How to create archived home data to be used by this test:
//  1. Flash the previous version of ARC++ (e.g. ARC++ P).
//  2. Sign in with the specified test account (See arc.DataMigration.yaml for username/password).
//  3. Wait until ARC++ boots and uninstall all unnecessary apps. For managed test cases, make sure
//     that the app to be installed during the test is available on the Play Store.
//  4. (optional) Populate files under /data/ or install apps.
//  5. ssh to DUT and create .tbz2 file by
//     `cd /home/.shadow/<hash>/mount && tar --xattrs --selinux -cjf /tmp/<dest_file_name>.tbz2 .`
//     Only for R->T test cases, run `settings put secure user_setup_complete 0` right before taking
//     the snapshot to work around the Play Store reauthentication issue (b/285820960).
//  6. Upload the tbz2 file into gs://chromiumos-test-assets-public/tast/cros/arc/ and update
//     the .external file (See tast/local/bundles/cros/arc/data/data_migration_pi_x86_64.external).
const (
	homeDataNameNycX86                = "data_migration_nyc_x86_64"
	homeDataNamePiX86                 = "data_migration_pi_x86_64"
	homeDataNamePiArm                 = "data_migration_pi_arm64"
	homeDataNameRvcX86Virtiofs        = "data_migration_rvc_x86_64_virtiofs"
	homeDataNameManagedPiX86          = "data_migration_managed_pi_x86_64"
	homeDataNameManagedRvcX86Virtiofs = "data_migration_managed_rvc_x86_64_virtiofs"
	arcDataMigrationUnmanagedPool     = "arc_data_migration_unmanaged"
	arcDataMigrationManagedPool       = "arc_data_migration_managed"
	dataMigrationTestTimeout          = 10 * time.Minute
)

type dataMigrationTestParams struct {
	poolID       string
	dataFileName string
	managed      bool
	bootTimeout  time.Duration
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataMigration,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Boots ARC with /data created on the previous version of ARC and verifies Play Store can install an app",
		Contacts:     []string{"arc-storage@google.com", "momohatt@google.com", "niwa@google.com"},
		// ChromeOS > Software > ARC++ > Storage
		BugComponent: "b:516669",
		// "no_qemu" is added for excluding betty from the target board list.
		// TODO(b/179636279): Remove "no_qemu" after making the test pass on betty.
		SoftwareDeps: []string{"chrome", "no_qemu"},
		Timeout:      dataMigrationTestTimeout,
		VarDeps:      []string{tape.ServiceAccountVar},
		Params: []testing.Param{{
			// Launch ARC P with /data created on ARC N (for x86).
			Name: "n_to_p_x86",
			Val: dataMigrationTestParams{
				poolID:       arcDataMigrationUnmanagedPool,
				dataFileName: homeDataNameNycX86,
				managed:      false,
			},
			ExtraAttr:         []string{"group:mainline", "informational"},
			ExtraData:         []string{homeDataNameNycX86},
			ExtraSoftwareDeps: []string{"android_p", "amd64"},
		}, {
			// Launch ARC R with /data created on ARC P (for x86).
			Name: "p_to_r_x86",
			Val: dataMigrationTestParams{
				poolID:       arcDataMigrationUnmanagedPool,
				dataFileName: homeDataNamePiX86,
				managed:      false,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
			ExtraData: []string{homeDataNamePiX86},
			ExtraSoftwareDeps: []string{
				"android_r",
				"amd64",
			},
		}, {
			// Launch ARC R with /data created on ARC P (for arm).
			Name: "p_to_r_arm",
			Val: dataMigrationTestParams{
				poolID:       arcDataMigrationUnmanagedPool,
				dataFileName: homeDataNamePiArm,
				managed:      false,
				// Use a longer timeout as ARCVM will boot with virtio-fs /data,
				// which is much slower than virtio-blk /data on ARM.
				bootTimeout: 4 * time.Minute,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
			ExtraData: []string{homeDataNamePiArm},
			ExtraSoftwareDeps: []string{
				"android_r",
				"arm",
			},
		}, {
			// Launch ARC T with virtio-fs /data created on ARC R (for x86).
			Name: "r_to_t_x86_virtiofs",
			Val: dataMigrationTestParams{
				poolID:       arcDataMigrationUnmanagedPool,
				dataFileName: homeDataNameRvcX86Virtiofs,
				managed:      false,
			},
			// Use arc_core so that this test case runs on ARC variant boards.
			ExtraAttr: []string{"group:arc", "arc_core", "group:arc-functional"},
			ExtraData: []string{homeDataNameRvcX86Virtiofs},
			ExtraSoftwareDeps: []string{
				"android_vm_t",
				"amd64",
				"no_arcvm_virtio_blk_data",
			},
		}, {
			// Launch ARC R with /data created on ARC P for managed user(for x86).
			Name: "managed_p_to_r_x86",
			Val: dataMigrationTestParams{
				poolID:       arcDataMigrationManagedPool,
				dataFileName: homeDataNameManagedPiX86,
				managed:      true,
			},
			ExtraAttr: []string{"group:mainline", "informational"},
			ExtraData: []string{homeDataNameManagedPiX86},
			ExtraSearchFlags: []*testing.StringPair{
				pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
			},
			ExtraSoftwareDeps: []string{
				"android_r",
				"amd64",
			},
		}, {
			// Launch ARC T with virtio-fs /data created on ARC R for managed user (for x86).
			Name: "managed_r_to_t_x86_virtiofs",
			Val: dataMigrationTestParams{
				poolID:       arcDataMigrationManagedPool,
				dataFileName: homeDataNameManagedRvcX86Virtiofs,
				managed:      true,
			},
			// Use arc_core so that this test case runs on ARC variant boards.
			ExtraAttr: []string{"group:arc", "arc_core", "group:arc-functional"},
			ExtraData: []string{homeDataNameManagedRvcX86Virtiofs},
			ExtraSearchFlags: []*testing.StringPair{
				pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
			},
			ExtraSoftwareDeps: []string{
				"android_vm_t",
				"amd64",
				"no_arcvm_virtio_blk_data",
			},
		}},
	})
}

// DataMigration checks regressions for the following bugs:
// b/173835269 Can't download or install some apps after P->R data migration.
// b/183690158 vold hangs while processing fixupAllAppDirs() if there are thousands of files to fix.
//
//	(Home data data_migration_pi_* contains 5000 dirs under
//	 /sdcard/Android/data/com.android.vending/files/ for reproducing this bug.)
//
// b/190293594 GMSCore for Pi is picked up on ARC R after P->R upgrade.
func DataMigration(ctx context.Context, s *testing.State) {
	params := s.Param().(dataMigrationTestParams)
	homeDataPath := s.DataPath(params.dataFileName)

	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: 2,
		DoRetries:   true,
		Errorf:      s.Errorf,
		Logf:        s.Logf}

	if err := testing.Poll(ctx, func(ctx context.Context) (retErr error) {
		return tryDataMigration(ctx, s.RequiredVar(tape.ServiceAccountVar), params, homeDataPath, rl, s.OutDir())
	}, nil); err != nil {
		s.Fatal("Failed to verify data migration flow: ", err)
	}
}

// tryDataMigration attempts a migration or returns a retirable error.
func tryDataMigration(ctx context.Context, serviceAccount string, params dataMigrationTestParams, homeDataPath string, rl *retry.Loop, outDir string) error {
	const (
		// One of the apps reported by b/173835269.
		appToInstall        = "com.roblox.client"
		provisioningTimeout = 5 * time.Minute
	)

	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Minute)
	defer cancel()

	// Create an account manager and lease a test account for the duration of the test.
	accHelper, acc, err := tape.NewOwnedTestAccountManager(ctx, []byte(serviceAccount), false, tape.WithTimeout(int32(dataMigrationTestTimeout.Seconds())), tape.WithPoolID(params.poolID))
	if err != nil {
		return rl.Exit("create an account manager and lease an account", err)
	}
	defer accHelper.CleanUp(cleanupCtx)

	// Ensure to sign out before executing MountVaultWithArchivedHomeData().
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		return rl.Exit("sign out", err)
	}

	// Unarchive the home data under vault before signing in.
	cleanupFunc, err := datamigration.MountVaultWithArchivedHomeData(ctx, homeDataPath, acc.Username, acc.Password)
	if err != nil {
		return rl.Exit("mount home with archived data", err)
	}
	defer cleanupFunc(cleanupCtx)

	args := append(arc.DisableSyncFlags(), "--disable-arc-data-wipe")

	creds := chrome.Creds{User: acc.Username, Pass: acc.Password}
	opts := []chrome.Option{
		chrome.GAIALogin(creds),
		chrome.ARCSupported(),
		chrome.KeepState(),
		chrome.UnRestrictARCCPU(),
		chrome.DisableFeatures("ArcEnableVirtioBlkForData"),
		chrome.ExtraArgs(args...),
	}

	if params.managed {
		fdms, err := arcent.SetupPolicyServerWithArcApps(ctx, outDir, creds.User, []string{}, arcent.InstallTypeAvailable, arcent.PlayStoreModeBlockList)
		if err != nil {
			return rl.Retry("setup fake policy server", err)
		}
		defer fdms.Stop(cleanupCtx)
		opts = append(opts, chrome.DMSPolicy(fdms.URL))
	}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return rl.Retry("start Chrome", err)
	}
	defer cr.Close(cleanupCtx)

	// Use the default boot timeout if not specified already.
	if params.bootTimeout == 0 {
		params.bootTimeout = arc.BootTimeout
	}
	a, err := arc.NewWithTimeout(ctx, outDir, params.bootTimeout, cr.NormalizedUser())
	if err != nil {
		return rl.Retry("start ARC", err)
	}
	defer a.Close(ctx)

	systemSdkVersion, err := checkSdkVersionsInPackagesXML(ctx, a, acc.Username)
	if err != nil {
		return rl.Exit("check SDK version in packages.xml", err)
	}

	if err := a.WaitForProvisioning(ctx, provisioningTimeout); err != nil {
		return rl.Retry("wait for ARC provisioning", err)
	}

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		return rl.Exit("initializing UI Automator", err)
	}
	defer d.Close(cleanupCtx)

	// Connect to Test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return rl.Retry("connect to test API", err)
	}

	installFailed := false
	recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn)
	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder,
		filepath.Join(outDir, fmt.Sprintf("recording-%d.mp4", rl.Attempts)),
		func() bool { return installFailed })

	// Regression check for b/173835269.
	testing.ContextLog(ctx, "Installing app ", appToInstall)
	var playOpt playstore.Options
	playOpt.TryLimit = -1
	playOpt.DefaultUITimeout = 5 * time.Second
	playOpt.ShortUITimeout = 5 * time.Second

	installCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := playstore.InstallApp(installCtx, a, d, appToInstall, &playOpt); err != nil {
		screenshotName := fmt.Sprintf("install-failed-%d.png", rl.Attempts)
		testing.ContextLog(ctx, "Taking a screenshot as "+screenshotName)
		path := filepath.Join(outDir, screenshotName)
		if err := screenshot.Capture(cleanupCtx, path); err != nil {
			testing.ContextLog(ctx, "Failed to take a screenshot: ", err)
		}

		installFailed = true
		return rl.Retry("install app", err)
	}

	// Regression check for b/190293594.
	if err := checkGmsCoreVersion(ctx, a, systemSdkVersion); err != nil {
		return rl.Exit("verify GMS Core version", err)
	}

	return nil
}

// checkSdkVersionsInPackagesXML checks if system SDK version is higher than data SDK version and
// returns system SDK version.
func checkSdkVersionsInPackagesXML(ctx context.Context, a *arc.ARC, username string) (int, error) {
	const (
		packagesXMLPath = "/data/system/packages.xml"
	)

	// SDK version of ARC running currently.
	systemVersionRegexp := regexp.MustCompile(`\<version sdkVersion="(\d+)"`)
	systemVersion := 0

	// SDK version of ARC /data.
	dataVersionRegexp := regexp.MustCompile(`\<version volumeUuid="\w+" sdkVersion="(\d+)"`)
	dataVersion := 0

	testing.ContextLogf(ctx, "Checking SDK versions in %s", packagesXMLPath)

	rootCryptDir, err := cryptohome.SystemPath(ctx, username)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get the cryptohome directory for the user")
	}

	// /home/root/<hash>/android-data/data/system/packages.xml
	b, err := a.ReadXMLFile(ctx, filepath.Join(rootCryptDir, "android-data", packagesXMLPath))

	for _, l := range strings.Split(string(b), "\n") {
		m := systemVersionRegexp.FindStringSubmatch(l)
		if m != nil {
			systemVersion, _ = strconv.Atoi(m[1])
		}
		m = dataVersionRegexp.FindStringSubmatch(l)
		if m != nil {
			dataVersion, _ = strconv.Atoi(m[1])
		}
		if systemVersion > 0 && dataVersion > 0 {
			break
		}
	}

	foundVersionsText := fmt.Sprintf("Found system SDK version: %d, data SDK verson: %d",
		systemVersion, dataVersion)
	testing.ContextLog(ctx, foundVersionsText)
	if systemVersion <= 0 || dataVersion <= 0 {
		return 0, errors.Wrapf(err, "failed to get system SDK version or data SDK version in packages.xml (%s)",
			foundVersionsText)
	}
	if systemVersion <= dataVersion {
		return 0, errors.Wrapf(err, "system SDK version should be higher than data SDK version (%s)",
			foundVersionsText)
	}
	return systemVersion, nil
}

// checkGmsCoreVersion checks if ARC is using an expected version of GMSCore.
func checkGmsCoreVersion(ctx context.Context, a *arc.ARC, systemSdkVersion int) error {
	// Regexp for matching a GMSCore version string in logcat.
	// e.g. "com.google.android.gms@212013032@21.20.13 (100800-374639054)"
	gmscoreVersionRegexp := regexp.MustCompile(
		`com\.google\.android\.gms@\d+@\d+\.\d+\.\d+ \((\d{6})-\d+\)`)

	out, err := a.Command(ctx, "logcat", "-d").Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to run logcat")
	}

	var fullVersionString string
	var versionCode int
	for _, line := range strings.Split(string(out), "\n") {
		m := gmscoreVersionRegexp.FindStringSubmatch(line)
		if len(m) == 2 {
			fullVersionString = m[0]
			versionCode, _ = strconv.Atoi(m[1])
			break
		}
	}
	if len(fullVersionString) == 0 {
		return errors.New("failed to find GMSCore version string in logcat")
	}
	testing.ContextLogf(ctx, "ARC is using GMSCore of version %q", fullVersionString)

	// Checks "variant" in GMSCore version string. Reference: go/gmscore-decoder-ring
	var expectedVariant int
	switch systemSdkVersion {
	case 28:
		// Skip checking variant for ARC P.
		return nil
	case 30:
		expectedVariant = 15 // PROD_RVC
	case 31:
		expectedVariant = 19 // PROD_SC
	case 33:
		// Skip checking variant for ARC T. In R->T upgrade, GMSCore version is not updated
		// because prod-RVC GMSCore is compatible with T (b/284954303).
		return nil
	default:
		return errors.Errorf("unexpected system SDK version: %d", systemSdkVersion)
	}
	variant := versionCode / 10000
	if variant != expectedVariant {
		return errors.Errorf("ARC is using GMSCore of unexpected variant: got %d; want %d. Found GMSCore version: %q",
			variant, expectedVariant, fullVersionString)
	}

	return nil
}
