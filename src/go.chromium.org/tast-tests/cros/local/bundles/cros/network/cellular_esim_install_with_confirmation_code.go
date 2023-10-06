// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/hermesconst"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/hermes"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/stork"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CellularESimInstallWithConfirmationCode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests the add eSIM profile via activation code flow in the success and failure cases",
		Contacts: []string{
			"cros-connectivity@google.com",
			"hsuregan@google.com",
		},
		BugComponent: "b:1131774", // ChromeOS > Software > System Services > Connectivity > Cellular
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:cellular", "cellular_sim_test_esim"},
		Fixture:      "cellularTestESIM",
		Timeout:      9 * time.Minute,
	})
}

func CellularESimInstallWithConfirmationCode(ctx context.Context, s *testing.State) {
	euicc, slot, err := hermes.GetEUICC(ctx, true)
	if err != nil {
		s.Fatal("Failed to get test euicc: ", err)
	}

	// Remove any existing profiles on test euicc
	if err := euicc.ResetMemory(ctx); err != nil {
		s.Fatal("Failed to reset test euicc: ", err)
	}
	defer euicc.ResetMemory(ctx)
	s.Log("Reset test euicc completed")

	if err := euicc.DBusObject.Call(ctx, hermesconst.EuiccMethodUseTestCerts, true).Err; err != nil {
		s.Fatal("Failed to set use test cert on eUICC: ", err)
	}
	s.Log("Set to use test cert on euicc completed")

	// Start a Chrome instance that will fetch policies from the FakeDMS.
	chromeOpts := []chrome.Option{
		chrome.EnableFeatures("UseStorkSmdsServerAddress"),
	}
	if slot == 1 {
		s.Log("Append CellularUseSecondEuicc feature flag")
		chromeOpts = append(chromeOpts, chrome.EnableFeatures("CellularUseSecondEuicc"))
	}

	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}

	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	mdp, err := ossettings.OpenMobileDataSubpage(ctx, tconn, cr)
	if err != nil {
		s.Fatal("Failed to open mobile data subpage: ", err)
	}

	confirmationCode := "0909"
	incorrectConfirmationCode := "9090"
	maxConfirmationCodeAttempts := 2
	confirmationCodeInput := nodewith.NameRegex(regexp.MustCompile("Confirmation code")).Focusable().First()
	activationCode, cleanupFunc, err := stork.FetchStorkProfileWithCustomConfirmationCode(ctx, confirmationCode, maxConfirmationCodeAttempts)
	if err != nil {
		s.Fatal("Failed to fetch Stork profile: ", err)
	}
	defer cleanupFunc(ctx)

	s.Log("Fetched Stork profile with activation code: ", activationCode)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open the keyboard: ", err)
	}
	defer kb.Close(ctx)

	if err := ossettings.AddESimWithActivationCode(ctx, tconn, string(activationCode)); err != nil {
		s.Fatal("Failed to add esim profile with correct activation code: ", err)
	}
	if err := uiauto.Combine("Select confirmation code input to enter incorrect code",
		mdp.WithTimeout(3*time.Minute).WaitUntilExists(confirmationCodeInput),
		mdp.LeftClick(confirmationCodeInput),
	)(ctx); err != nil {
		s.Fatal("Enter incorrect confirmation code user journey fails: ", err)
	}

	if err := kb.Type(ctx, incorrectConfirmationCode); err != nil {
		s.Fatal("Failed to type incorrect confirmation code: ", err)
	}

	var incorrectActivationCodeSubtext = nodewith.NameContaining("Unable to connect to this profile.").Role(role.StaticText)
	if err := uiauto.Combine("Verify that incorrect confirmation code subtext shows",
		mdp.LeftClick(ossettings.ConfirmButton.Focusable()),
		mdp.WithTimeout(3*time.Minute).WaitUntilExists(incorrectActivationCodeSubtext),
	)(ctx); err != nil {
		s.Fatal("Failed to verify that incorrect confirmation code subtext shows: ", err)
	}

	if err := uiauto.Combine("Re-select and highlight incorrect confirmation code",
		mdp.WaitUntilExists(confirmationCodeInput.Focusable()),
		mdp.DoubleClick(confirmationCodeInput),
	)(ctx); err != nil {
		s.Fatal("Failed to highlight incorrect confirmation code: ", err)
	}

	if err := kb.Type(ctx, confirmationCode); err != nil {
		s.Fatal("Failed to type confirmation code: ", err)
	}

	if err := uiauto.Combine("Exit add cellular eSIM flow after using correct confirmation code",
		mdp.LeftClick(ossettings.ConfirmButton.Focusable()),
		mdp.WithTimeout(3*time.Minute).WaitUntilExists(ossettings.NetworkAddedText),
		mdp.LeftClick(ossettings.DoneButton.Focusable()),
	)(ctx); err != nil {
		s.Fatal("Correct activation code and confirmation code user journey fails: ", err)
	}

	if err := ossettings.VerifyTestESimProfile(ctx, tconn); err != nil {
		s.Fatal("Failed to verify newly installed stork profile: ", err)
	}
}
