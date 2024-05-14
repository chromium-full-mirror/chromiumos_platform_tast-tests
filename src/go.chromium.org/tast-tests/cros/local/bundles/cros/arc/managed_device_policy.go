// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/arcent"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/imagehelpers"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/externaldata"
	"go.chromium.org/tast-tests/cros/local/retry"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManagedDevicePolicy,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "This test ensure that managed policies are applied to Android",
		Contacts:     []string{"arc-commercial@google.com", "mhasank@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "play_store"},
		VarDeps: []string{
			arcent.LoginPoolVar,
		},
		Data: []string{"wallpaper_image.jpeg", "managed_device_policy_ca_cert.pem"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcCertificatesSyncMode{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.AudioCaptureAllowed{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DefaultGeolocationSetting{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DisableScreenshots{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.OpenNetworkConfiguration{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.PrintingEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.VideoCaptureAllowed{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.WallpaperImage{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.VpnConfigAllowed{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{
			{
				ExtraSoftwareDeps: []string{"android_container", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "betty",
				ExtraSoftwareDeps: []string{"android_container", "qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm", "no_android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "x",
				ExtraSoftwareDeps: []string{"android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "betty_vm",
				ExtraSoftwareDeps: []string{"android_vm", "qemu"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			}},
		Timeout: chrome.LoginTimeout + arc.BootTimeout + 3*time.Minute,
	})
}

const devicePolicyPkg = "org.chromium.arc.testapp.devicepolicy"

type arcPolicyFactory func() (policy.Policy, func(ctx context.Context), error)

func ManagedDevicePolicy(ctx context.Context, s *testing.State) {
	const (
		apk                      = "ArcDevicePolicyTest.apk"
		mainActivityCls          = devicePolicyPkg + ".MainActivity"
		disabledSystemPkg        = "com.google.android.deskclock"
		enabledAccessibilityPkg  = "com.google.android.marvin.talkback"
		disabledAccessibilityPkg = "com.google.android.apps.accessibility.auditor"
	)

	packages := []string{enabledAccessibilityPkg, disabledAccessibilityPkg}
	arcPolicyMap := map[string]arcPolicyFactory{
		"ArcCertificatesSyncMode":        staticPolicy(&policy.ArcCertificatesSyncMode{Val: 1 /*Enable sync*/}),
		"cameraDisabled":                 staticPolicy(&policy.VideoCaptureAllowed{Val: false}),
		"credentialsConfigDisabled":      staticPolicy(nil),
		"enabledSystemAppPackageNames":   staticPolicy(nil),
		"installUnknownSourcesDisabled":  staticPolicy(nil),
		"modifyAccountsDisabled":         staticPolicy(nil),
		"permittedAccessibilityServices": staticPolicy(nil),
		"printingDisabled":               staticPolicy(&policy.PrintingEnabled{Val: false}),
		"screenCaptureDisabled":          staticPolicy(&policy.DisableScreenshots{Val: true}),
		"setWallpaper": func() (policy.Policy, func(ctx context.Context), error) {
			return createWallpaperPolicy(ctx, s.DataPath("wallpaper_image.jpeg"))
		},
		"shareLocationDisabled":    staticPolicy(&policy.DefaultGeolocationSetting{Val: 2 /*BlockGeolocation*/}),
		"unmuteMicrophoneDisabled": staticPolicy(&policy.AudioCaptureAllowed{Val: false}),
		"vpnConfigDisabled":        staticPolicy(&policy.VpnConfigAllowed{Val: false}),
	}

	creds, err := credconfig.PickRandomCreds(s.RequiredVar(arcent.LoginPoolVar))
	if err != nil {
		s.Fatal("Failed to get login creds: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	login := chrome.GAIALogin(creds)

	caCert, err := ioutil.ReadFile(s.DataPath("managed_device_policy_ca_cert.pem"))
	if err != nil {
		s.Fatal("Failed to read ca cert: ", err)
	}

	arcEnabledPolicy := &policy.ArcEnabled{Val: true}
	arcPolicy := arcent.CreateArcPolicyWithApps(packages, arcent.InstallTypeForceInstalled, arcent.PlayStoreModeBlockList)
	// Adding a CaCert will cause credentialsConfigDisabled to be also configured.
	caCertPolicy := &policy.OpenNetworkConfiguration{
		Val: &policy.ONC{
			Certificates: []*policy.ONCCertificate{
				{
					GUID:      "{b3aae353-cfa9-4093-9aff-9f8ee2bf8c29}",
					TrustBits: []string{"Web"},
					Type:      "Authority",
					X509:      string(caCert),
				},
			},
		},
	}
	policies := []policy.Policy{arcEnabledPolicy, arcPolicy, caCertPolicy}
	fdms, err := arcent.SetUpFakePolicyServer(ctx, s.OutDir(), creds.User, policies, false /*affiliated*/)
	if err != nil {
		s.Fatal("Failed to setup fake policy server: ", err)
	}
	defer fdms.Stop(cleanupCtx)

	cr, err := chrome.New(
		ctx,
		login,
		chrome.ARCSupported(),
		chrome.UnRestrictARCCPU(),
		chrome.DMSPolicy(fdms.URL),
		chrome.ExtraArgs(append(arc.DisableSyncFlags(), "--vmodule=arc_policy_bridge=1")...))
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	a, err := arc.NewWithTimeout(ctx, s.OutDir(), arc.BootTimeout, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC by policy: ", err)
	}
	defer a.Close(cleanupCtx)
	if err := arcent.WaitForProvisioning(ctx, a, 1 /*attempt*/); err != nil {
		s.Fatal("Failed to wait for provisioning: ", err)
	}

	lastSyncTimeStamp, err := waitForPolicySync(ctx, a, cr.NormalizedUser(), -1 /*lastSyncTimeStamp*/)
	if err != nil {
		s.Fatal("Failed to get policy sync time: ", err)
	}

	if err := a.WaitForPackages(ctx, packages); err != nil {
		s.Fatal("Packages did not install in time: ", err)
	}

	s.Log("Installing test app")
	if err := a.Install(ctx, arc.APKPath(apk)); err != nil {
		s.Fatal("Failed installing app: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn)
	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "recording.webm"), s.HasError)

	s.Log("Testing policies without restrictions")
	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer d.Close(cleanupCtx)

	cleanup, err := launchApp(ctx, tconn, a, devicePolicyPkg, mainActivityCls)
	if err != nil {
		s.Fatal("Failed to launch the app: ", err)
	}
	defer cleanup(cleanupCtx)

	// No need to retry when testing policies without any changes.
	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: 1,
		DoRetries:   false,
		Errorf:      s.Errorf,
		Logf:        s.Logf}
	for policyName := range arcPolicyMap {
		if err := testPolicyEnforcement(ctx, tconn, a, d, policyName, true /*shouldSucceed*/, rl); err != nil {
			s.Fatalf("Test for policy %s failed: %v", policyName, err)
		}
	}

	s.Log("Updating policies to apply restrictions")
	arcPolicy.Val.EnabledSystemAppPackageNames = []string{disabledSystemPkg}
	arcPolicy.Val.InstallUnknownSourcesDisabled = true
	arcPolicy.Val.ModifyAccountsDisabled = true
	arcPolicy.Val.PermittedAccessibilityServices.Enabled = true
	arcPolicy.Val.PermittedAccessibilityServices.PackageNames = []string{enabledAccessibilityPkg}

	for policyName := range arcPolicyMap {
		newPolicy, cleanup, err := arcPolicyMap[policyName]()
		if err != nil {
			s.Fatalf("Failed to create %s policy: %v", policyName, err)
		}
		defer cleanup(cleanupCtx)
		if newPolicy != nil {
			policies = append(policies, newPolicy)
		}
	}

	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, policies); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	if _, err := waitForPolicySync(ctx, a, cr.NormalizedUser(), lastSyncTimeStamp); err != nil {
		s.Fatal("ARC policy not synced: ", err)
	}

	s.Log("Testing policies with restrictions")

	// It can take time for the policies to apply so retry the first policy a few times until it succeeds.
	firstTest := true
	for policyName := range arcPolicyMap {
		rl = &retry.Loop{Attempts: 1,
			MaxAttempts: 10,
			DoRetries:   firstTest,
			Errorf:      s.Errorf,
			Logf:        s.Logf}
		if err := testPolicyEnforcement(ctx, tconn, a, d, policyName, false /*shouldSucceed*/, rl); err != nil {
			s.Fatalf("Test for policy %s failed: %v", policyName, err)
		}
		firstTest = false
	}
}

func launchApp(ctx context.Context, tconn *chrome.TestConn, a *arc.ARC, appPackage, mainActivity string) (func(ctx context.Context), error) {
	testing.ContextLog(ctx, "Starting app")
	act, err := arc.NewActivity(a, appPackage, mainActivity)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create main activity")
	}
	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		act.Close(ctx)
		return nil, errors.Wrap(err, "failed to start main activity")
	}
	cleanup := func(ctx context.Context) {
		act.Close(ctx)
		act.Stop(ctx, tconn)
	}
	return cleanup, nil
}

func createWallpaperPolicy(ctx context.Context, imgPath string) (policy.Policy, func(ctx context.Context), error) {
	jpegBytes, err := imagehelpers.GetJPEGBytesFromFilePath(imgPath)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to read wallpaper image")
	}

	eds, err := externaldata.NewServer(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create external data server")
	}
	cleanup := func(ctx context.Context) { eds.Stop(ctx) }

	iurl, ihash := eds.ServePolicyData(jpegBytes)
	policy := &policy.WallpaperImage{Val: &policy.WallpaperImageValue{Url: iurl, Hash: ihash}}

	return policy, cleanup, nil
}

func staticPolicy(value policy.Policy) arcPolicyFactory {
	return func() (policy.Policy, func(ctx context.Context), error) {
		return value /*policy*/, func(ctx context.Context) {} /*cleanup*/, nil /*error*/
	}
}

func waitForPolicySync(ctx context.Context, a *arc.ARC, user string, lastSyncTimeStamp int64) (int64, error) {
	const policySyncTimeout = 30 * time.Second

	testing.ContextLog(ctx, "Waiting for ARC policy to sync")

	var currentSyncTimeStamp int64 = -1
	return currentSyncTimeStamp, arc.PollWithReadOnlyAndroidData(ctx, user, func(ctx context.Context) error {
		var err error
		currentSyncTimeStamp, err = getPolicySyncTimestamp(ctx, user)
		if err != nil {
			return errors.Wrap(err, "failed to get ARC policy sync time")
		}

		if currentSyncTimeStamp == lastSyncTimeStamp {
			return errors.New("ARC policy not yet synced")
		}

		return nil
	}, &testing.PollOptions{Timeout: policySyncTimeout, Interval: time.Second})
}

func getPolicySyncTimestamp(ctx context.Context, user string) (int64, error) {
	const (
		policyUpdateTimeout = 30 * time.Second
		dpcPrefPath         = "data/data/com.google.android.apps.work.clouddpc.arc/shared_prefs/prefs.xml"
	)
	syncTimestampRe := regexp.MustCompile(`<long\sname="sync_timestamp"\s+value="(\d+)"\s*/>`)

	androidDataDir, err := arc.AndroidDataDir(ctx, user)
	if err != nil {
		return -1, errors.Wrap(err, "failed to get android-data path")
	}

	dpcPrefFullPath := filepath.Join(androidDataDir, dpcPrefPath)
	prefsText, err := os.ReadFile(dpcPrefFullPath)
	if err != nil {
		return -1, err
	}

	timestamp := syncTimestampRe.FindStringSubmatch(string(prefsText))
	if timestamp == nil {
		return -1, errors.New("sync_timestamp not found")
	}

	// Epoch time in milliseconds.
	epoch, err := strconv.ParseInt(timestamp[1], 10 /*base*/, 64 /*int64*/)
	if err != nil {
		return -1, errors.Wrapf(err, "failed to parse timestamp %q", timestamp[1])
	}

	return epoch, nil
}

func testPolicyEnforcement(ctx context.Context, tconn *chrome.TestConn, a *arc.ARC, d *ui.Device, policy string, shouldSucceed bool, rl *retry.Loop) error {
	const (
		policiesListID = devicePolicyPkg + ":id/lstPolicies"
		testButtonID   = devicePolicyPkg + ":id/btnTest"
		errorTextID    = devicePolicyPkg + ":id/txtError"
		testTimeout    = 2 * time.Minute
	)

	localTestMap := map[string]func(ctx context.Context) (bool, string, error){
		"permittedAccessibilityServices": func(ctx context.Context) (bool, string, error) {
			return getPermittedAccessibilityServicesTestResult(ctx, tconn, a, d, shouldSucceed)
		},
	}

	testing.ContextLogf(ctx, "Testing policy %q and expecting success=%v", policy, shouldSucceed)
	return testing.Poll(ctx, func(ctx context.Context) error {
		var succeeded bool
		var errMessage string
		var err error
		tester, isTestLocal := localTestMap[policy]
		if isTestLocal {
			succeeded, errMessage, err = tester(ctx)
			if err != nil {
				return rl.Exit("get manual test result", err)
			}
		} else {
			if err := selectSpinnerItem(ctx, d, policiesListID, policy); err != nil {
				return err
			}

			btnTest := d.Object(ui.ID(testButtonID))
			if err := btnTest.Click(ctx); err != nil {
				return errors.Wrap(err, "failed to click test")
			}

			succeeded, err = getPolicyTestResult(ctx, d)
			if err != nil {
				return rl.Exit("get test result", err)
			}

			txtError := d.Object(ui.ID(errorTextID))
			errMessage, err = txtError.GetText(ctx)
			if err != nil {
				return rl.Exit("get error message", err)
			}
		}

		if succeeded == shouldSucceed {
			testing.ContextLog(ctx, "Policy test succeeded with expected result: ", shouldSucceed)
			return nil
		}

		return rl.Retry(fmt.Sprintf("get expected result for policy %q: %v, got %t, error: %s", policy, shouldSucceed, succeeded, errMessage), nil)
	}, &testing.PollOptions{Timeout: testTimeout, Interval: time.Second})
}

func getPermittedAccessibilityServicesTestResult(ctx context.Context, tconn *chrome.TestConn, a *arc.ARC, d *ui.Device, shouldSucceed bool) (succeeded bool, errMessage string, err error) {
	const (
		enabledPackageTitle         = "TalkBack"
		disabledPackageTitle        = "Accessibility Scanner"
		accessibilitySettingsIntent = "android.settings.ACCESSIBILITY_SETTINGS"
		settingsPackage             = "com.android.settings"
	)

	if err := a.SendIntentCommand(ctx, accessibilitySettingsIntent, "").Run(testexec.DumpLogOnError); err != nil {
		return false, "", err
	}

	closeSettingsApp := func() {
		if settingsWindow, err := ash.GetARCAppWindowInfo(ctx, tconn, settingsPackage); err == nil {
			settingsWindow.CloseWindow(ctx, tconn)
		}
	}
	defer closeSettingsApp()

	err = testing.Poll(ctx, func(ctx context.Context) (err error) {
		if err := waitForAccessibilityAppInState(ctx, d, enabledPackageTitle, true /*enabled*/); err != nil {
			return testing.PollBreak(err)
		}

		if err := waitForAccessibilityAppInState(ctx, d, disabledPackageTitle, shouldSucceed /*enabled*/); err != nil {
			closeSettingsApp()
			return err
		}

		return nil
	}, &testing.PollOptions{Interval: time.Second})
	if err == nil {
		return shouldSucceed, "", nil
	}
	return false, err.Error(), nil
}

func waitForAccessibilityAppInState(ctx context.Context, d *ui.Device, title string, enabled bool) error {
	const accessibilityAppWaitTimeout = 10 * time.Second

	testing.ContextLogf(ctx, "Waiting for %s app to be enabled=%t", title, enabled)
	appTitle := d.Object(ui.ID("android:id/title"), ui.ClassName("android.widget.TextView"), ui.Text(title), ui.Enabled(enabled))
	if err := appTitle.WaitForExists(ctx, accessibilityAppWaitTimeout); err != nil {
		return err
	}
	return nil
}

func getPolicyTestResult(ctx context.Context, d *ui.Device) (bool, error) {
	const (
		outputTextID   = devicePolicyPkg + ":id/txtOutput"
		resultWaitTime = 1 * time.Minute
	)

	resultRegex := regexp.MustCompile("true|false")

	succeeded := false
	testing.ContextLog(ctx, "Waiting for policy test result")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		txtOutput := d.Object(ui.ID(outputTextID))

		output, err := txtOutput.GetText(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get output")
		}

		if !resultRegex.MatchString(output) {
			return errors.New("Unexpected result :" + output)
		}

		succeeded, _ = strconv.ParseBool(output)
		return nil
	}, &testing.PollOptions{Timeout: resultWaitTime, Interval: 5 * time.Second}); err != nil {
		return false, err
	}

	return succeeded, nil
}

func selectSpinnerItem(ctx context.Context, d *ui.Device, spinnerId, itemText string) error {
	const defaultTimeout = 30 * time.Second

	spinner := d.Object(ui.ID(spinnerId))
	if err := spinner.WaitForExists(ctx, defaultTimeout); err != nil {
		return errors.Wrap(err, "failed to find the spinner")
	}

	if err := spinner.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to open the spinner")
	}

	item := d.Object(ui.Text(itemText))
	if err := item.WaitForExists(ctx, defaultTimeout); err != nil {
		return errors.Wrapf(err, "failed to find %s in the spinner", itemText)
	}

	if err := item.Click(ctx); err != nil {
		return errors.Wrapf(err, "failed to select %s in the spinner", itemText)
	}

	return nil
}
