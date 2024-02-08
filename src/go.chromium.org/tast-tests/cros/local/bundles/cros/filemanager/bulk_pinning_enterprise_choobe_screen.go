// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/oobe"
	"go.chromium.org/tast-tests/cros/local/policyutil"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BulkPinningEnterpriseChoobeScreen,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the the CHOOBE screen shows for enterprise users with the right policy",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"benreich@google.com",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"drivefs",
		},
		Attr: []string{
			"group:cbx",
			"cbx_feature_enabled",
			"cbx_stable",
		},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
			"accountmanager.managedusername",
			"accountmanager.managedpassword",
		},
		TestBedDeps: []string{tbdep.Cbx(true)},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-da6b2959-16bb-46d2-be77-1b8eb814168f",
		},
			pci.SearchFlag(&policy.DriveFileSyncAvailable{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.OsColorMode{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.ShowTouchpadScrollScreenEnabled{}, pci.VerifiedValue),
			pci.SearchFlag(&policy.ShowDisplaySizeScreenEnabled{}, pci.VerifiedValue),
		},
		Timeout: 5 * time.Minute,
		Params: []testing.Param{{
			Name: "unset",
			Val: bulkPinningPolicyTest{
				policy:  &policy.DriveFileSyncAvailable{Stat: policy.StatusUnset},
				visible: true,
			},
		}, {
			Name: "disabled",
			Val: bulkPinningPolicyTest{
				policy:  &policy.DriveFileSyncAvailable{Val: "disabled"},
				visible: false,
			},
		}, {
			Name: "visible",
			Val: bulkPinningPolicyTest{
				policy:  &policy.DriveFileSyncAvailable{Val: "visible"},
				visible: true,
			},
		}},
	})
}

type bulkPinningPolicyTest struct {
	policy  *policy.DriveFileSyncAvailable
	visible bool
}

func BulkPinningEnterpriseChoobeScreen(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	username := s.RequiredVar("accountmanager.managedusername")
	password := s.RequiredVar("accountmanager.managedpassword")

	param, ok := s.Param().(bulkPinningPolicyTest)
	if !ok {
		s.Fatal("Failed to extract policy params")
	}

	// CHOOBE is only shown to users if there are 3+ optional screens.
	// Optional screens "color theme", "touchpad scroll" and "display size" are
	// controlled by admin policy.
	// Let's enable them so CHOOBE is displayed.
	// Note: unsetting "OsColorMode" does not cause it to appear on CHOOBE - it's
	// still evaluated as "recommended".
	policies := []policy.Policy{
		&policy.OsColorMode{Stat: policy.StatusUnset},
		&policy.ShowTouchpadScrollScreenEnabled{Val: true},
		&policy.ShowDisplaySizeScreenEnabled{Val: true},
		param.policy,
	}
	fdms, err := policyutil.SetUpFakePolicyServer(ctx, s.OutDir(), username, policies)
	if err != nil {
		s.Fatal("Could not set set up fake policy server: ", err)
	}
	defer fdms.Stop(cleanupCtx)

	chromeOptions := []chrome.Option{
		chrome.EnableFeatures("FeatureManagementDriveFsBulkPinning"),
		chrome.DontSkipOOBEAfterLogin(),
		chrome.DMSPolicy(fdms.URL),
		chrome.GAIALogin(chrome.Creds{
			User: username,
			Pass: password,
		}),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		chrome.ExtraArgs("--vmodule=drivefs_pinning_manager=1"),
	}

	cr, err := chrome.New(ctx, chromeOptions...)
	if err != nil {
		s.Fatal("Connect to Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to create OOBE connection: ", err)
	}
	defer oobeConn.Close()

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create the signin profile test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	dfs, err := drivefs.NewDriveFs(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to wait for DriveFS to mount: ", err)
	}
	defer dfs.SaveLogsOnError(cleanupCtx, s.HasError)

	umaName := "FileBrowser.GoogleDrive.BulkPinning.CHOOBEScreenStage"
	histograms, err := metrics.RunAndWaitAll(ctx, tconn, 5*time.Minute, func(ctx context.Context) error {
		if param.visible {
			if err := advanceConsolidatedConsentScreenAndWaitTimeSpentListing(ctx, tconn, oobeConn); err != nil {
				return errors.Wrap(err, "failed to advance through the consolidated consent screen expecting bulk pinning visible")
			}
		} else {
			if err := oobe.AdvanceThroughConsolidatedConsentIfShown(ctx, oobeConn, tconn); err != nil {
				return errors.Wrap(err, "failed to advance through the consolidated consent screen expecting bulk pinning not visible")
			}
		}

		if err := oobeConn.Eval(ctx, "OobeAPI.advanceToScreen('choobe')", nil); err != nil {
			return errors.Wrap(err, "failed to advance to CHOOBE")
		}

		if err := expectChoobeScreen(ctx, tconn, oobeConn, param.visible); err != nil {
			return errors.Wrap(err, "failed to expect CHOOBE screen to be in the right state")
		}

		return nil
	}, umaName)
	if err != nil {
		s.Fatal("Failed to navigate the OOBE UI: ", err)
	}

	if len(histograms) != 1 {
		s.Fatalf("Failed to record histogram, got %d want 1", len(histograms))
	}
	// The integer 6 maps to `kSuccess`.
	// See https://source.chromium.org/chromium/chromium/src/+/main:chromeos/ash/components/drivefs/mojom/pin_manager_types.mojom
	wantStage := int64(6)
	if !param.visible {
		// In the event bulk pinning is disabled, the only metric that is emitted is
		// the `CHOOBEScreenStage` being in the state of `kStopped`.
		wantStage = 0
	}
	if histograms[0].Buckets[0].Min != wantStage {
		s.Fatalf("Failed to record correct histogram value for %q, got %d want %d", umaName, histograms[0].Buckets[0].Min, wantStage)
	}
}

// expectChoobeScreen either enumerates the list of potential CHOOBE screens
// or ensures the bulk pinning button is visible. CHOOBE is shown only if there
// are >2 eligible screens to show, if less than that each screen will show up
// individually.
func expectChoobeScreen(ctx context.Context, tconn *chrome.TestConn, oobeConn *chrome.Conn, visible bool) error {
	if err := oobeConn.WaitForExprFailOnErrWithTimeout(ctx, "OobeAPI.screens.ChoobeScreen.isReadyForTesting()", 30*time.Second); err == nil {
		if err := oobeConn.Eval(ctx, "OobeAPI.screens.ChoobeScreen.clickDrivePinningScreen()", nil); err == nil && !visible || err != nil && visible {
			return errors.Wrap(err, "failed to click drive pinning screen or clicked drive pinning screen when it shouldn't be visible")
		}
		return nil
	}

	ui := uiauto.New(tconn)
	title := nodewith.Role(role.Heading).Ancestor(nodewith.HasClass("step"))
	for {
		info, err := ui.WithTimeout(30*time.Second).Info(ctx, title)
		if err != nil && !visible {
			testing.ContextLog(ctx, "✅ Screen doesn't have a title. OOBE flow has ended")
			return nil
		}

		testing.ContextLogf(ctx, "🖼 Current screen title is %q", info.Name)

		var screenName string
		if err := oobeConn.Eval(ctx, "OobeAPI.getCurrentScreenName()", &screenName); err != nil {
			return errors.Wrap(err, "failed to get current OOBE screen name")
		}

		if screenName == "drive-pinning" && !visible {
			return errors.New("failed due to finding drive pinning screen when it shouldn't be visible")
		} else if screenName == "drive-pinning" && visible {
			return nil
		}

		if err := ui.WithTimeout(30*time.Second).LeftClickUntil(
			nodewith.Role(role.Button).Name("Next"),
			ui.Gone(title.Name(info.Name)),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to leave current OOBE screen")
		}
	}
}

// advanceConsolidatedConsentScreenAndWaitTimeSpentListing advances through the
// consolidated consent screen and waits for the `BulkPinning.TimeSpentListing`
// UMA to get emitted which indicates the file enumeration has finished.
func advanceConsolidatedConsentScreenAndWaitTimeSpentListing(ctx context.Context, tconn *chrome.TestConn, oobeConn *chrome.Conn) error {
	umaName := "FileBrowser.GoogleDrive.BulkPinning.TimeSpentListing"
	histograms, err := metrics.RunAndWaitAll(ctx, tconn, 30*time.Second, func(ctx context.Context) error {
		return oobe.AdvanceThroughConsolidatedConsentIfShown(ctx, oobeConn, tconn)
	}, umaName)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve histogram data")
	}

	if len(histograms) != 1 {
		return errors.Errorf("failed to record histogram, got %d want 1", len(histograms))
	}
	if histograms[0].Sum == 0 {
		testing.ContextLogf(ctx, "Recorded a histogram value of 0 for %q", umaName)
	}
	return nil
}
