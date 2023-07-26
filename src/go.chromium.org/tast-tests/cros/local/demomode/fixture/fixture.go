// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/oobe"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	// PostDemoModeOOBE is the name for the fixture that clicks through Demo Mode OOBE setup
	PostDemoModeOOBE = "postDemoModeOOBE"
	// PostDemoModeOOBECloudGaming is similar to PostDemoModeOOBE, except Cloud Gaming
	// customizations are enabled (i.e. the CloudGamingDevice feature is enabled).
	PostDemoModeOOBECloudGaming = "postDemoModeOOBECloudGaming"

	setUpTimeout    = 350 * time.Second
	tearDownTimeout = 150 * time.Second
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: PostDemoModeOOBE,
		Desc: "Has proceeded through Demo Mode setup flow from OOBE",
		Contacts: []string{
			"cros-demo-mode-eng@google.com",
			"jacksontadie@google.com",
		},
		Impl: &fixtureImpl{
			// This user has infinite idle time-out value for demo mode, thus will not end demo mode session in middle of test.
			enrollmentUser: "admin-tast",
		},
		SetUpTimeout:    setUpTimeout,
		TearDownTimeout: tearDownTimeout,
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: PostDemoModeOOBECloudGaming,
		Desc: "Has proceeded through Demo Mode setup flow from OOBE with Cloud Gaming customizations configured",
		Contacts: []string{
			"jacksontadie@google.com",
			"cros-demo-mode-eng@google.com",
		},
		Impl: &fixtureImpl{
			// This user has infinite idle time-out value for demo mode, thus will not end demo mode session in middle of test.
			enrollmentUser:  "admin-tast",
			enabledFeatures: []string{"CloudGamingDevice"},
		},
		SetUpTimeout:    setUpTimeout,
		TearDownTimeout: tearDownTimeout,
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey"},
	})
}

// fixtureImpl implements testing.FixtureImpl.
type fixtureImpl struct {
	// The user that the device enrolls into Demo Mode with. This allows us to
	// control which Organizational Unit the device enrolls into, thus the policies.
	enrollmentUser string
	// Additional features that should be enabled during Demo Mode setup
	enabledFeatures []string
}

var _ testing.FixtureImpl = &fixtureImpl{}

// Run through Demo Mode setup flow from OOBE.
//
// TODO(b/231472901): Deduplicate the shared code between Demo Mode and normal
// OOBE Tast tests.
func (f *fixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	resetTPMAndSystemState(ctx, s)

	cr, err := chrome.New(ctx,
		chrome.NoLogin(),
		chrome.ARCSupported(),
		chrome.EnableFeatures(f.enabledFeatures...),
		chrome.DontSkipOOBEAfterLogin(),
		chrome.ExtraArgs("--demo-mode-enrolling-username="+f.enrollmentUser),
		chrome.ExtraArgs("--arc-start-mode=always-start"),
		// Download test version of components (most importantly demo-mode-resources and demo-mode-app),
		// to catch issues before they reach prod
		// TODO(b/263269444): Consider running a version of these tests against the prod components as well
		chrome.ExtraArgs("--component-updater=test-request"),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	clearUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer cr.Close(clearUpCtx)

	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to create OOBE connection: ", err)
	}
	defer oobeConn.Close()

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create the signin profile test API connection: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(clearUpCtx, s.OutDir(), s.HasError, tconn, "oobe_failure")

	ui := uiauto.New(tconn).WithTimeout(50 * time.Second)

	findAndClickButton := func(buttonApiMethod string) {
		var buttonName string
		if err := oobeConn.Eval(ctx, "OobeAPI.screens."+buttonApiMethod, &buttonName); err != nil {
			s.Fatal("Failed to get button name by calling "+buttonApiMethod+": ", err)
		}

		button := nodewith.Role(role.Button).Name(buttonName)
		if err := uiauto.Combine("Click button with name: "+buttonName,
			ui.WaitUntilExists(button),
			ui.LeftClick(button),
		)(ctx); err != nil {
			s.Fatal("Failed to click button with name: "+buttonName+" - error: ", err)
		}
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard handle: ", err)
	}
	defer kb.Close(ctx)

	if err := kb.Accel(ctx, "Ctrl+Alt+D"); err != nil {
		s.Fatal("Failed to enter Demo Setup dialogue with ctrl + alt + D: ", err)
	}

	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.WelcomeScreen.demoModeConfirmationDialog.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the demo confirmation dialog to be visible: ", err)
	}
	findAndClickButton("WelcomeScreen.getDemoModeOkButtonName()")

	shouldSkipNetworkScreen := false
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.NetworkScreen.shouldSkip()", &shouldSkipNetworkScreen); err != nil {
		s.Fatal("Failed to evaluate whether to skip network screen: ", err)
	}
	if shouldSkipNetworkScreen {
		s.Log("NetworkScreen.shouldSkip() is true; skipped")
	} else {
		s.Log("Proceeding through network screen")
		if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.NetworkScreen.isVisible()"); err != nil {
			s.Fatal("Failed to wait for the network screen to be visible: ", err)
		}
		// TODO(crbug.com/1291153): Switch to focused button.
		nextButton := nodewith.Name("Next").Role(role.Button)
		if err := ui.LeftClickUntil(nextButton, ui.Gone(nextButton))(ctx); err != nil {
			s.Fatal("Failed to click network page next button: ", err)
		}
	}

	s.Log("Proceeding through demo preferences screen")
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.DemoPreferencesScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the demo preferences screen to be visible: ", err)
	}
	retailerNameInput := nodewith.Role(role.TextField).Name("Retailer Name")
	storeNumberInput := nodewith.Role(role.TextField).Name("Store Number")
	if err := uiauto.Combine("Enter Retailer Name And Store Number",
		ui.LeftClickUntil(retailerNameInput, ui.WaitUntilExists(retailerNameInput.Editable().Focused())),
		kb.TypeAction("Tast Retailer"),
		kb.AccelAction("tab"),
		ui.WaitUntilExists(storeNumberInput.Editable().Focused()),
		kb.TypeAction("1234"),
	)(ctx); err != nil {
		s.Fatal("Failed to enter Retailer Name or Store Number: ", err)
	}
	findAndClickButton("DemoPreferencesScreen.getDemoPreferencesNextButtonName()")

	// Connect to session manager now for post-setup session login, before clicking
	// final accept button and entering non-interactive part of demo setup
	sm, err := session.NewSessionManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to session manager: ", err)
	}
	sw, err := sm.WatchSessionStateChanged(ctx, "started")
	if err != nil {
		s.Fatal("Failed to watch for session manager D-Bus signals: ", err)
	}
	defer sw.Close(ctx)
	// Click Consolidated Consent accept to finish interactive part of setup
	s.Log("Proceeding through consolidated consent screen")
	if err := oobe.AdvanceThroughConsolidatedConsentIfShown(ctx, oobeConn, tconn); err != nil {
		s.Fatal("Failed to advance through consolidated consent screen: ", err)
	}

	s.Log("Waiting for SessionStateChanged \"started\" D-Bus signal from session_manager")
	select {
	case <-sw.Signals:
		s.Log("Got SessionStateChanged signal. Demo Session has started")
	case <-ctx.Done():
		s.Fatal("Didn't get SessionStateChanged signal: ", ctx.Err())
	}

	return nil
}

func (f *fixtureImpl) Reset(ctx context.Context) error {
	return nil
}

func (f *fixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *fixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *fixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	resetTPMAndSystemState(ctx, s)
}

// resetTPMAndSystemState resets TPM, which can take a few seconds.
func resetTPMAndSystemState(ctx context.Context, s *testing.FixtState) {
	r := hwsec.NewCmdRunner()
	helper, err := hwsec.NewHelper(r)
	if err != nil {
		s.Fatal("Helper creation error: ", err)
	}
	s.Log("Start to reset TPM")
	if err := helper.EnsureTPMAndSystemStateAreReset(ctx); err != nil {
		s.Fatal("Failed to ensure resetting TPM: ", err)
	}
	s.Log("TPM is confirmed to be reset")
}
