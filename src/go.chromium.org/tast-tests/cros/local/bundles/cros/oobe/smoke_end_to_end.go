// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package oobe

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/oobe"
	"go.chromium.org/tast-tests/cros/local/testenv"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type param struct {
	isAddPersonFlow bool
	usePreprod      bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SmokeEndToEnd,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Smoke test that goes through OOBE, Login and Onboarding using the automation tools",
		Contacts: []string{
			"cros-oobe@google.com",
			"bohdanty@google.com",
			"rrsilva@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1263090", // ChromeOS > Software > OOBE
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "non_meets_device"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
			"ui.gaiaPoolDefault",
		},
		Timeout: chrome.GAIALoginTimeout + 5*time.Minute,
		Params: []testing.Param{{
			ExtraAttr: []string{"group:criticalstaging"},
			Val:       param{isAddPersonFlow: false, usePreprod: false},
		}, {
			Name:      "add_person_flow",
			ExtraAttr: []string{"group:criticalstaging"},
			Val:       param{isAddPersonFlow: true, usePreprod: false},
		}, {
			ExtraAttr: []string{"group:testenv_preprod"},
			Name:      "preprod",
			Val:       param{isAddPersonFlow: false, usePreprod: true},
		}, {
			ExtraAttr: []string{"group:testenv_preprod"},
			Name:      "preprod_add_person_flow",
			Val:       param{isAddPersonFlow: true, usePreprod: true},
		}},
	})
}

func SmokeEndToEnd(ctx context.Context, s *testing.State) {
	// Smoke End To End covers two flows:
	// - "Normal" OOBE flow from the Welcome screen
	//
	// - "Add Person" flow from the login screen. In this flow, Chrome is first
	//   launched to create a test user and then relaunched with 'KeepState' so
	//   it lands on the login screen.

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, time.Second*10)
	defer cancel()

	// Run against the preprod of Google frontend.
	if s.Param().(param).usePreprod {
		envOpts := []testenv.Option{
			testenv.RedirectMap(map[string]string{
				"gfe-prod": "gfe-preprod",
			}),
			testenv.PortalDetection(false), // Disable portal detection that may conflict with a mid DNS server during network validation.
		}
		env, err := testenv.NewPreprodEnv(ctx, envOpts...)
		if err != nil {
			s.Fatal("Failed to init the preprod env: ", err)
		}
		if err := env.Start(ctx); err != nil {
			s.Fatal("Failed to set up the preprod env: ", err)
		}
		defer env.Close(cleanupCtx)
	}

	isAddPersonFlow := s.Param().(param).isAddPersonFlow
	if isAddPersonFlow {
		// Create user on the device.
		cr, err := chrome.New(ctx)
		if err != nil {
			s.Fatal("Failed to start Chrome and create a new user: ", err)
		}
		cr.Close(ctx)
	}

	options := []chrome.Option{
		chrome.DontSkipOOBEAfterLogin(),
		chrome.DeferLogin(),
		chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	}

	// Keep the user that was previously added for the 'AddPerson' flow.
	if isAddPersonFlow {
		options = append(options, chrome.KeepState())
	}

	options = append(options, chrome.ExtraArgs("--skip-multidevice-screen"))

	cr, err := chrome.New(ctx, options...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
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

	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)

	focusedButton := nodewith.State(state.Focused, true).Role(role.Button)

	// Click on the 'Add Person' button if we are on the login screen, otherwise step through
	// the OOBE screens that are shown during the first boot.
	if isAddPersonFlow {
		err := ui.LeftClick(nodewith.Role(role.Button).Name("Add Person"))(ctx)
		if err != nil {
			s.Fatal("Failed to click on the Add Person button")
		}
	} else {
		s.Log("Waiting for the welcome screen")
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.WelcomeScreen.isVisible()"); err != nil {
			s.Fatal("Failed to wait for the welcome screen to be visible: ", err)
		}

		if err := uiauto.Combine("click next on the welcome screen",
			ui.WaitUntilExists(focusedButton),
			ui.LeftClick(focusedButton),
		)(ctx); err != nil {
			s.Fatal("Failed to click welcome screen next button: ", err)
		}

		shouldSkipNetworkScreen := false
		if err := oobeConn.Eval(ctx, "OobeAPI.screens.NetworkScreen.shouldSkip()", &shouldSkipNetworkScreen); err != nil {
			s.Fatal("Failed to evaluate whether to skip Network screen: ", err)
		}

		if shouldSkipNetworkScreen {
			s.Log("Skipping the network screen")
		} else {
			s.Log("Waiting for the network screen")
			if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.NetworkScreen.isVisible()"); err != nil {
				s.Fatal("Failed to wait for the network screen to be visible: ", err)
			}
			//(TODO, https://crbug.com/1291153): Switch to focused button.
			nextButton := nodewith.Name("Next").Role(role.Button)
			if err := ui.LeftClickUntil(nextButton, ui.Gone(nextButton))(ctx); err != nil {
				s.Fatal("Failed to click network page next button: ", err)
			}
		}
	}
	// TODO(b/255972416) - Remove this.
	// Even though clicking on the 'Add Person' shows the User Creation screen first, the
	// flow in performGAIALogin (gaia.go) has been hardcoded to inject JS into the page
	// and click on the next button to land on the Gaia screen. It does this whenever it
	// sees the URL used for the 'Add Person' flow (chrome://oobe/gaia-signin).
	if !isAddPersonFlow {
		s.Log("Waiting for the user creation screen")
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.UserCreationScreen.isVisible()"); err != nil {
			s.Fatal("Failed to wait for the user creation screen to be visible: ", err)
		}

		if err := oobeConn.Eval(ctx, "OobeAPI.screens.UserCreationScreen.selectPersonalUser()", nil); err != nil {
			s.Fatal("Failed to select personal account: ", err)
		}

		nextButton := nodewith.Name("Next").Role(role.Button)
		if err := uiauto.Combine("click next on the user creation screen",
			ui.WaitUntilEnabled(nextButton),
			ui.LeftClick(nextButton),
		)(ctx); err != nil {
			s.Fatal("Failed to click user creation screen next button: ", err)
		}

		s.Log("Waiting for the Gaia Info screen")
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.GaiaInfoScreen.isVisible()"); err != nil {
			s.Fatal("Failed to wait for the Gaia Info screen to be visible: ", err)
		}

		if err := uiauto.Combine("click next on the Gaia Info screen",
			ui.WaitUntilExists(focusedButton),
			ui.LeftClick(focusedButton),
		)(ctx); err != nil {
			s.Fatal("Failed to click Gaia Info screen next button: ", err)
		}

		s.Log("Waiting for the Gaia screen")
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.GaiaScreen.isVisible()"); err != nil {
			s.Fatal("Failed to wait for the login screen to be visible: ", err)
		}
	}

	if err := cr.ContinueLogin(ctx); err != nil {
		s.Fatal("Failed to continue login: ", err)
	}

	s.Log("Waiting for the consolidated consent screen")
	if err := oobe.AdvanceThroughConsolidatedConsentIfShown(ctx, oobeConn, tconn); err != nil {
		s.Fatal("Failed to advance through consolidated consent screen: ", err)
	}

	s.Log("Waiting for the sync screen")
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.SyncScreen.isReadyForTesting()"); err != nil {
		s.Fatal("Failed to wait for the sync creation screen to be visible: ", err)
	}
	if err := uiauto.Combine("click next on the sync screen",
		ui.WaitUntilExists(focusedButton),
		ui.LeftClick(focusedButton),
	)(ctx); err != nil {
		s.Fatal("Failed to continue on the sync screen: ", err)
	}

	shouldSkipFingerprint := false
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.FingerprintScreen.shouldSkip()", &shouldSkipFingerprint); err != nil {
		s.Fatal("Failed to evaluate whether to skip fingerprint screen: ", err)
	}

	if shouldSkipFingerprint {
		s.Log("Skipping the fingerprint screen")
	} else {
		s.Log("Waiting for the fingerprint screen")
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.FingerprintScreen.isVisible()"); err != nil {
			s.Fatal("Failed to wait for the fingerprint screen to be visible: ", err)
		}

		if err := uiauto.Combine("click next on the fingerprint screen",
			ui.WaitUntilExists(focusedButton),
			ui.LeftClick(focusedButton),
		)(ctx); err != nil {
			s.Fatal("Failed to skip on the fingerprint screen: ", err)
		}
	}

	cmdRunner := hwseclocal.NewCmdRunner()
	cryptohome := hwsec.NewCryptohomeClient(cmdRunner)
	supportsLE := false
	if supportsLE, err = cryptohome.SupportsLECredentials(ctx); err != nil {
		s.Fatal("Failed to get supported policies: ", err)
	}

	isInTabletMode := false
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.PinSetupScreen.isInTabletMode()", &isInTabletMode); err != nil {
		s.Fatal("Failed to evaluate whether the device in the table mode: ", err)
	}

	if supportsLE || isInTabletMode {
		s.Log("Waiting for the pin setup screen")
		var pinSkipButton string
		if err := oobeConn.Eval(ctx, "OobeAPI.screens.PinSetupScreen.getSkipButtonName()", &pinSkipButton); err != nil {
			s.Fatal("Failed to get pin setup skip button name: ", err)
		}
		skipButton := nodewith.Role(role.Button).Name(pinSkipButton)
		if err := ui.LeftClick(skipButton)(ctx); err != nil {
			s.Fatal("Failed to click pin setup skip button: ", err)
		}
	} else {
		s.Log("Skipping the pin setup screen")
	}

	shouldSkipAssistant := false
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.AssistantScreen.shouldSkip()", &shouldSkipAssistant); err != nil {
		s.Fatal("Failed to evaluate whether to skip assistant screen: ", err)
	}

	// TODO(b/279775853): Remove assistant handling logic after we remove it from OOBE.
	if shouldSkipAssistant {
		s.Log("Skipping the assistant screen")
	} else {
		s.Log("Waiting for the assistant screen")
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.AssistantScreen.isReadyForTesting()"); err != nil {
			s.Fatal("Failed to wait for the assistant screen to be visible: ", err)
		}
		var assistantSkipButton string
		if err := oobeConn.Eval(ctx, "OobeAPI.screens.AssistantScreen.getSkipButtonName()", &assistantSkipButton); err != nil {
			s.Fatal("Failed to get assistant next button name: ", err)
		}
		var previousUserFlowShown bool
		if err := oobeConn.Eval(ctx, "OobeAPI.screens.AssistantScreen.isPreviousUserFlowShown()", &previousUserFlowShown); err != nil {
			s.Fatal("Failed to get which assitant flow we currently show: ", err)
		}
		if previousUserFlowShown {
			s.Log("Showing assistant flow for the existing assistant user")
		} else {
			s.Log("Showing assistant flow for a new assistant user")
		}
		skipButton := nodewith.Role(role.Button).Name(assistantSkipButton)
		if err := uiauto.Combine("click skip on the assistant screen",
			ui.WaitUntilExists(skipButton),
			ui.LeftClick(skipButton),
		)(ctx); err != nil {
			s.Fatal("Failed to click assistant skip button: ", err)
		}
		if previousUserFlowShown {
			if err := uiauto.Combine("click skip on the assistant screen for the existing assistant user",
				ui.WaitUntilExists(skipButton),
				ui.LeftClick(skipButton),
			)(ctx); err != nil {
				s.Fatal("Failed to click assistant skip button: ", err)
			}
		} else {
			var newUserSecondScreenShown bool
			if err := oobeConn.Eval(ctx, "OobeAPI.screens.AssistantScreen.isVisible()", &newUserSecondScreenShown); err != nil {
				s.Fatal("Failed to get whether assistant screen is still shown: ", err)
			}
			if newUserSecondScreenShown {
				if err := uiauto.Combine("click skip on the second assistant screen for a new assistant user",
					ui.WaitUntilExists(skipButton),
					ui.LeftClick(skipButton),
				)(ctx); err != nil {
					s.Fatal("Failed to click assistant skip button: ", err)
				}
			}
		}
	}

	shouldSkipSmartPrivacyProtection := false
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.SmartPrivacyProtectionScreen.shouldSkip()", &shouldSkipSmartPrivacyProtection); err != nil {
		s.Fatal("Failed to evaluate whether to skip smart privacy protection screen: ", err)
	}

	if shouldSkipSmartPrivacyProtection {
		s.Log("Skipping the smart privacy protection screen")
	} else {
		s.Log("Waiting for the smart privacy protection screen")
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.SmartPrivacyProtectionScreen.isReadyForTesting()"); err != nil {
			s.Fatal("Failed to wait for the smart privacy protection screen to be visible: ", err)
		}
		var smartPrivacyNoThanksButtonName string
		if err := oobeConn.Eval(ctx, "OobeAPI.screens.SmartPrivacyProtectionScreen.getNoThanksButtonName()", &smartPrivacyNoThanksButtonName); err != nil {
			s.Fatal("Failed to get smart privacy protection no thanks button name: ", err)
		}
		noThanks := nodewith.Role(role.Button).Name(smartPrivacyNoThanksButtonName)
		if err := uiauto.Combine("click no thanks on the smart privacy protection screen",
			ui.WaitUntilExists(noThanks),
			ui.LeftClick(noThanks),
		)(ctx); err != nil {
			s.Fatal("Failed to click smart privacy protection no thanks button: ", err)
		}
	}

	if isInTabletMode {
		s.Log("Waiting for the gesture navigation screen")
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.GestureNavigation.isVisible()"); err != nil {
			s.Fatal("Failed to wait for the gesture navigation screen to be visible: ", err)
		}
		var gestureNavigationSkipButtonName string
		if err := oobeConn.Eval(ctx, "OobeAPI.screens.GestureNavigation.getSkipButtonName()", &gestureNavigationSkipButtonName); err != nil {
			s.Fatal("Failed to get gesture navigation skip button name: ", err)
		}
		gestureNavigationSkipButton := nodewith.Role(role.Button).Name(gestureNavigationSkipButtonName)
		if err := uiauto.Combine("click skip on the gesture navigation screen",
			ui.WaitUntilExists(gestureNavigationSkipButton),
			ui.LeftClick(gestureNavigationSkipButton),
		)(ctx); err != nil {
			s.Fatal("Failed to skip on the gesture navigation screen: ", err)
		}
	}

	shouldSkipChoobe := false
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.ChoobeScreen.requestShouldSkip()", nil); err != nil {
		s.Fatal("Failed to request the evaluation of the `shouldSkip()` method of CHOOBE screen: ", err)
	}

	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.ChoobeScreen.isShouldSkipReceived()"); err != nil {
		s.Fatal("Failed to wait for the reception of `ShouldSkip` value of CHOOBE screen: ", err)
	}

	if err := oobeConn.Eval(ctx, "OobeAPI.screens.ChoobeScreen.updatedShouldSkip()", &shouldSkipChoobe); err != nil {
		s.Fatal("Failed to evaluate whether to skip CHOOBE screen: ", err)
	}

	isTouchpadScrollButtonVisible := false
	isDisplaySizeButtonVisible := false
	isThemeSelectionButtonVisible := false
	if !shouldSkipChoobe {
		s.Log("Waiting for CHOOBE screen")
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.ChoobeScreen.isVisible()"); err != nil {
			s.Fatal("Failed to wait for CHOOBE screen screen to be visible: ", err)
		}

		// Get the visibility of screen buttons on the CHOOBE screen and click visible screens.
		if err := oobeConn.Eval(ctx, "OobeAPI.screens.ChoobeScreen.isTouchpadScrollScreenVisible()", &isTouchpadScrollButtonVisible); err != nil {
			s.Fatal("Failed to evaluate whether the touchpad scroll button on CHOOBE screen is visible: ", err)
		}

		if isTouchpadScrollButtonVisible {
			if err := oobeConn.Eval(ctx, "OobeAPI.screens.ChoobeScreen.clickTouchpadScrollScreen()", nil); err != nil {
				s.Fatal("Failed to click the touchpad scroll button on CHOOBE screen: ", err)
			}
		}

		if err := oobeConn.Eval(ctx, "OobeAPI.screens.ChoobeScreen.isDisplaySizeScreenVisible()", &isDisplaySizeButtonVisible); err != nil {
			s.Fatal("Failed to evaluate whether the display size button on CHOOBE screen is visible: ", err)
		}
		if isDisplaySizeButtonVisible {
			if err := oobeConn.Eval(ctx, "OobeAPI.screens.ChoobeScreen.clickDisplaySizeScreen()", nil); err != nil {
				s.Fatal("Failed to click the touchpad scroll button on CHOOBE screen: ", err)
			}
		}

		if err := oobeConn.Eval(ctx, "OobeAPI.screens.ChoobeScreen.isThemeSelectionScreenVisible()", &isThemeSelectionButtonVisible); err != nil {
			s.Fatal("Failed to evaluate whether the theme selection button on CHOOBE screen is visible: ", err)
		}
		if isThemeSelectionButtonVisible {
			if err := oobeConn.Eval(ctx, "OobeAPI.screens.ChoobeScreen.clickThemeSelectionScreen()", nil); err != nil {
				s.Fatal("Failed to click the touchpad scroll button on CHOOBE screen: ", err)
			}
		}

		if err := oobeConn.Eval(ctx, "OobeAPI.screens.ChoobeScreen.clickNext()", nil); err != nil {
			s.Fatal("Failed to click next on the CHOOBE screen: ", err)
		}
	} else {
		s.Log("CHOOBE screen skipped")
	}

	var shouldSkipTouchpadScroll bool
	if !shouldSkipChoobe {
		shouldSkipTouchpadScroll = !isTouchpadScrollButtonVisible
	} else {
		if err := oobeConn.Eval(ctx, "OobeAPI.screens.ChoobeTouchpadScrollScreen.requestShouldSkip()", nil); err != nil {
			s.Fatal("Failed to request the evaluation of the `shouldSkip()` method of touchpad scroll screen: ", err)
		}

		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.ChoobeTouchpadScrollScreen.isShouldSkipReceived()"); err != nil {
			s.Fatal("Failed to wait for the reception of `ShouldSkip` value of touchpad scroll screen: ", err)
		}

		if err := oobeConn.Eval(ctx, "OobeAPI.screens.ChoobeTouchpadScrollScreen.updatedShouldSkip()", &shouldSkipTouchpadScroll); err != nil {
			s.Fatal("Failed to evaluate whether to skip touchpad scroll screen: ", err)
		}

	}
	if shouldSkipTouchpadScroll {
		s.Log("Skipping the touchpad scroll screen")
	} else {
		s.Log("Waiting for the touchpad scroll screen")
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.ChoobeTouchpadScrollScreen.isVisible()"); err != nil {
			s.Fatal("Failed to wait for the touchpad scroll screen to be visible: ", err)
		}
		if err := uiauto.Combine("click next on the touchpad scroll screen",
			ui.WaitUntilExists(focusedButton),
			ui.LeftClick(focusedButton),
		)(ctx); err != nil {
			s.Fatal("Failed to continue on the touchpad scroll screen: ", err)
		}
	}

	var shouldSkipDisplaySize bool
	if !shouldSkipChoobe {
		shouldSkipDisplaySize = !isDisplaySizeButtonVisible
	} else {
		if err := oobeConn.Eval(ctx, "OobeAPI.screens.ChoobeDisplaySizeScreen.shouldSkip()", &shouldSkipDisplaySize); err != nil {
			s.Fatal("Failed to evaluate whether to skip display size screen: ", err)
		}
	}
	if shouldSkipDisplaySize {
		s.Log("Skipping the display size screen")
	} else {
		s.Log("Waiting for the display size screen")
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.ChoobeDisplaySizeScreen.isVisible()"); err != nil {
			s.Fatal("Failed to wait for the  display size screen to be visible: ", err)
		}
		if err := uiauto.Combine("click next on the display size screen",
			ui.WaitUntilExists(focusedButton),
			ui.LeftClick(focusedButton),
		)(ctx); err != nil {
			s.Fatal("Failed to continue on the display size screen: ", err)
		}
	}

	shouldSkipThemeSelection := false
	if !shouldSkipChoobe {
		shouldSkipThemeSelection = !isThemeSelectionButtonVisible
	}
	if shouldSkipThemeSelection {
		s.Log("Skipping the theme selection screen")
	} else {
		s.Log("Waiting for the theme selection screen")
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.ThemeSelectionScreen.isVisible()"); err != nil {
			s.Fatal("Failed to wait for the theme selection screen to be visible: ", err)
		}
		if err := uiauto.Combine("click next on the theme selection screen",
			ui.WaitUntilExists(focusedButton),
			ui.LeftClick(focusedButton),
		)(ctx); err != nil {
			s.Fatal("Failed to continue on the theme selection screen: ", err)
		}
	}

	shouldSkipMarketingOptIn := false
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.MarketingOptInScreen.shouldSkip()", &shouldSkipMarketingOptIn); err != nil {
		s.Fatal("Failed to evaluate whether to skip marketing opt-in screen: ", err)
	}

	if shouldSkipMarketingOptIn {
		s.Log("Skipping marketing optin screen")
	} else {
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.MarketingOptInScreen.isVisible()"); err != nil {
			s.Fatal("Failed to wait for the marketing opt-in screen to be visible: ", err)
		}
		if err := uiauto.Combine("click next on the marketing-optin screen",
			ui.WaitUntilExists(focusedButton),
			ui.LeftClick(focusedButton),
		)(ctx); err != nil {
			s.Fatal("Failed to continue on the marketing opt-in screen: ", err)
		}
	}

	if err := cr.WaitForOOBEConnectionToBeDismissed(ctx); err != nil {
		s.Fatal("Failed to wait for OOBE to be dismissed: ", err)
	}
}
