// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apps

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/apps/pre"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/helpapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	policyFixt "go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type testParams struct {
	isOOBE           bool
	fieldTrialConfig chrome.FieldTrialConfigMode
}

func init() {
	testing.AddTest(&testing.Test{
		Func: LaunchHelpAppOnManagedDevice,
		Desc: "Launch Help App on a managed device",
		Contacts: []string{
			"showoff-eng@google.com",
		},
		BugComponent: "b:690873",
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Params: []testing.Param{
			{
				Name:              "oobe_stable_fieldtrial_testing_config_off",
				ExtraHardwareDeps: hwdep.D(pre.AppsStableModels),
				Val: testParams{
					isOOBE:           true,
					fieldTrialConfig: chrome.FieldTrialConfigDisable,
				},
				ExtraAttr: []string{"group:mainline"},
			}, {
				Name:              "oobe_stable_fieldtrial_testing_config_on",
				ExtraHardwareDeps: hwdep.D(pre.AppsStableModels),
				Val: testParams{
					isOOBE:           true,
					fieldTrialConfig: chrome.FieldTrialConfigEnable,
				},
				ExtraAttr: []string{"group:mainline", "group:chrome_uprev_cbx"},
			}, {
				Name:              "oobe_unstable",
				ExtraHardwareDeps: hwdep.D(pre.AppsUnstableModels),
				Val: testParams{
					isOOBE: true,
				},
				// b:238260020 - disable aged (>1y) unpromoted informational tests
				// ExtraAttr:         []string{"group:mainline", "informational"},
			}, {
				Name:              "logged_in_stable_fieldtrial_testing_config_off",
				ExtraHardwareDeps: hwdep.D(pre.AppsStableModels),
				Fixture:           "chromePolicyLoggedIn",
				Val: testParams{
					isOOBE:           false,
					fieldTrialConfig: chrome.FieldTrialConfigDisable,
				},
				ExtraAttr: []string{"group:mainline"},
			}, {
				Name:              "logged_in_stable_fieldtrial_testing_config_on",
				ExtraHardwareDeps: hwdep.D(pre.AppsStableModels),
				Fixture:           "chromePolicyLoggedIn",
				Val: testParams{
					isOOBE:           false,
					fieldTrialConfig: chrome.FieldTrialConfigEnable,
				},
				ExtraAttr: []string{"group:mainline", "group:chrome_uprev_cbx"},
			}, {
				Name:              "logged_in_unstable",
				ExtraHardwareDeps: hwdep.D(pre.AppsUnstableModels),
				Fixture:           "chromePolicyLoggedIn",
				Val: testParams{
					isOOBE: false,
				},
				// b:238260020 - disable aged (>1y) unpromoted informational tests
				// ExtraAttr:         []string{"group:mainline", "informational"},
			},
		}})
}

// LaunchHelpAppOnManagedDevice verifies launching Showoff on a managed device.
func LaunchHelpAppOnManagedDevice(ctx context.Context, s *testing.State) {
	params := s.Param().(testParams)
	isOOBE := params.isOOBE

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	var cr *chrome.Chrome
	if isOOBE {
		// Using fakedms and login
		// Start FakeDMS.
		tmpdir, err := os.MkdirTemp("", "fdms-")
		if err != nil {
			s.Fatal("Failed to create fdms temp dir: ", err)
		}
		defer os.RemoveAll(tmpdir)

		testing.ContextLogf(ctx, "FakeDMS starting in %s", tmpdir)
		fdms, err := fakedms.New(ctx, tmpdir)
		if err != nil {
			s.Fatal("Failed to start FakeDMS: ", err)
		}
		defer fdms.Stop(cleanupCtx)

		if err := fdms.WritePolicyBlob(policy.NewBlob()); err != nil {
			s.Fatal("Failed to write policies to FakeDMS: ", err)
		}

		cr, err = chrome.New(
			ctx,
			chrome.FakeLogin(chrome.Creds{User: policyFixt.Username, Pass: policyFixt.Password}),
			chrome.DMSPolicy(fdms.URL), chrome.DontSkipOOBEAfterLogin(),
			chrome.ExtraArgs("--allow-empty-passwords-in-tests"),
			chrome.EnableFeatures("HelpAppFirstRun"),
			chrome.FieldTrialConfig(params.fieldTrialConfig),
		)
		if err != nil {
			s.Fatal("Failed to connect to Chrome: ", err)
		}
		defer cr.Close(cleanupCtx)
	} else {
		cr = s.FixtValue().(chrome.HasChrome).Chrome()
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn)
	helpCtx := helpapp.NewContext(cr, tconn)
	// In OOBE stage, help app should not be launched on a managed device after login.
	if isOOBE {
		isAppLaunched, err := helpCtx.Exists(ctx)
		if err != nil {
			s.Fatal("Failed to verify help app existence: ", err)
		}

		if isAppLaunched {
			s.Error("Help app should not be launched after oobe on a managed device")
		}
	} else {
		// Once the help app has been launched, perks should not be shown on a managed device.
		if err := helpCtx.Launch()(ctx); err != nil {
			s.Fatal("Failed to launch help app: ", err)
		}

		// Check that loadTimeData is correctly set.
		loadTimeData, err := helpCtx.GetLoadTimeData(ctx)
		if err != nil {
			s.Fatal("Failed to get help app's load time data")
		}

		if !loadTimeData.IsManagedDevice {
			s.Error("Help app incorrectly set isManagedDevice to false")
		}

		// Check if perks tab is shown.
		isPerkShown, err := ui.IsNodeFound(ctx, helpapp.PerksTabFinder)
		if err != nil {
			s.Fatal("Failed to check perks visibility: ", err)
		}

		if isPerkShown {
			s.Error("Perks should not be shown on managed devices")
		}
	}
}
