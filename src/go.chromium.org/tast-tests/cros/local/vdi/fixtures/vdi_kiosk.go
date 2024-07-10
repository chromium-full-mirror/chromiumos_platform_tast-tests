// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixtures

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosproc"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/kioskmode"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	vdiApps "go.chromium.org/tast-tests/cros/local/vdi/apps"
	"go.chromium.org/tast-tests/cros/local/vdi/apps/citrix"
	"go.chromium.org/tast-tests/cros/local/vdi/apps/vmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	setUpTimeout    = kioskmode.SetupDuration + kioskmode.LaunchDuration + vdiApps.VDILoginTimeout
	resetTimeout    = chrome.ResetTimeout
	tearDownTimeout = kioskmode.CleanupDuration + time.Minute
	postTestTimeout = kioskmode.LaunchDuration + time.Minute
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.KioskCitrixLaunched,
		Desc: "Starts DUT fake enrolled in Kiosk mode with Citrix application installed, started and logged in",
		Contacts: []string{
			"kamilszare@google.com",
			"cros-engprod-muc@google.com",
		},
		BugComponent: "b:1170223", // ChromeOS > Software > Commercial (Enterprise) > EngProd
		Impl: &kioskFixtureState{
			vdiApplicationToStart: apps.Citrix,
			vdiConnector:          &citrix.Connector{},
			vdiServerKey:          "vdi.citrix_url",
			useTape:               true,
		},
		Vars: []string{
			tape.ServiceAccountVar,
			"vdi.citrix_url",
			"ui.signinProfileTestExtensionManifestKey",
			"uidetection.key_type",
			"uidetection.key",
			"uidetection.server",
		},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PostTestTimeout: postTestTimeout,
		Data:            citrix.CitrixData,
		Parent:          fixture.FakeDMSEnrolled,
	})

	testing.AddFixture(&testing.Fixture{
		Name: fixture.KioskLacrosCitrixLaunched,
		Desc: "Starts DUT fake enrolled in Kiosk mode with Citrix application installed, started and logged in",
		Contacts: []string{
			"kamilszare@google.com",
			"cros-engprod-muc@google.com",
		},
		BugComponent: "b:1170223", // ChromeOS > Software > Commercial (Enterprise) > EngProd
		Impl: &kioskFixtureState{
			vdiApplicationToStart:      apps.Citrix,
			vdiConnector:               &citrix.Connector{},
			vdiServerKey:               "vdi.citrix_url",
			useTape:                    true,
			extraPublicAccountPolicies: []policy.Policy{&policy.LacrosAvailability{Val: "lacros_only"}},
			lacros:                     true,
		},
		Vars: []string{
			tape.ServiceAccountVar,
			"vdi.citrix_url",
			"ui.signinProfileTestExtensionManifestKey",
			"uidetection.key_type",
			"uidetection.key",
			"uidetection.server",
		},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PostTestTimeout: postTestTimeout,
		Data:            citrix.CitrixData,
		Parent:          fixture.FakeDMSEnrolled,
	})

	testing.AddFixture(&testing.Fixture{
		Name: fixture.KioskVmwareLaunched,
		Desc: "Starts DUT fake enrolled in Kiosk mode with Vmware application installed, started and logged in",
		Contacts: []string{
			"kamilszare@google.com",
			"cros-engprod-muc@google.com",
		},
		BugComponent: "b:1170223", // ChromeOS > Software > Commercial (Enterprise) > EngProd
		Impl: &kioskFixtureState{
			vdiApplicationToStart: apps.VMWare,
			vdiConnector:          &vmware.Connector{},
			vdiServerKey:          "vdi.vmware_url",
			vdiUsernameKey:        "vdi.vmware_username",
			vdiPasswordKey:        "vdi.vmware_password",
			useTape:               false,
		},
		Vars: []string{
			"vdi.vmware_url",
			"vdi.vmware_username",
			"vdi.vmware_password",
			"ui.signinProfileTestExtensionManifestKey",
			"uidetection.key_type",
			"uidetection.key",
			"uidetection.server",
		},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PostTestTimeout: postTestTimeout,
		Data:            vmware.VmwareData,
		Parent:          fixture.FakeDMSEnrolled,
	})
}

type kioskFixtureState struct {
	// cr is a connection to an already-started Chrome instance that loads
	// policies from FakeDMS.
	cr *chrome.Chrome
	// kiosk is a reference to a kiosk sessions providing a clean way to tear
	// down the session.
	kiosk *kioskmode.Kiosk
	// fdms is the already running DMS server from the parent fixture.
	fdms *fakedms.FakeDMS
	// keyboard is a reference to keyboard to be release in the TearDown.
	keyboard *input.KeyboardEventWriter
	// vdiApplicationToStart is the VDI application that is launched.
	vdiApplicationToStart apps.App
	// VdiApplication
	vdiConnector vdiApps.VDIInt
	// vdiServerKey is a key to retrieve VDI server url.
	vdiServerKey string
	// vdiUsernameKey is a key to retrieve user name to access VDI app.
	vdiUsernameKey string
	// vdiPasswordKey is a key to retrieve VDI user password.
	vdiPasswordKey string
	// vdiConfig holds the vdi configuration.
	vdiConfig vdiApps.VDILoginConfig
	// accountsConfiguration holds the Kiosk accounts configuration that is
	// prepared in SetUp when creating Kiosk session and in TearDown when
	// cleaning policies preventing Kiosk from autostart - since
	// kioskmode.AutoLaunch() is used.
	accountsConfiguration policy.DeviceLocalAccounts
	// useTape is a flag to use Tape for leasing a vdi account.
	useTape bool
	// tapeAccountManager is used for cleaning up Tape.
	tapeAccountManager *tape.GenericAccountManager
	// extraPublicAccountPolicies holds a policies that will be applied.
	extraPublicAccountPolicies []policy.Policy
	// lacros is a flag indicating whether fixture implementation suppose to run Lacros.
	lacros bool
	// signinTestExtensionManifestKey is the manifest key of the test extension used to interact with
	// Chrome in the sign in screen.
	signinTestExtensionManifestKey string
}

func (v *kioskFixtureState) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	fdms, ok := s.ParentValue().(*fakedms.FakeDMS)
	if !ok {
		s.Fatal("Parent is not a FakeDMS fixture")
	}
	v.fdms = fdms
	v.signinTestExtensionManifestKey = s.RequiredVar("ui.signinProfileTestExtensionManifestKey")

	vdiAccountID := "vdi_kiosk@managedchrome.com"
	accountType := policy.AccountTypeKioskApp
	kioskAppPolicy := policy.DeviceLocalAccountInfo{
		AccountID:   &vdiAccountID,
		AccountType: &accountType,
		KioskAppInfo: &policy.KioskAppInfo{
			AppId: &v.vdiApplicationToStart.ID,
		}}

	v.accountsConfiguration = policy.DeviceLocalAccounts{
		Val: []policy.DeviceLocalAccountInfo{
			kioskAppPolicy,
		},
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
	defer cancel()

	kiosk, cr, err := kioskmode.New(
		ctx,
		fdms,
		v.signinTestExtensionManifestKey,
		kioskmode.CustomLocalAccounts(&v.accountsConfiguration),
		kioskmode.PublicAccountPolicies(vdiAccountID, v.extraPublicAccountPolicies...),
		kioskmode.AutoLaunch(vdiAccountID),
	)
	if err != nil {
		path := filepath.Join(s.OutDir(), "kiosk_fixture_failure.png")
		if err := screenshot.Capture(ctx, path); err != nil {
			s.Error("Failed to take screenshot: ", err)
		}
		s.Fatal("Failed to start Chrome in kiosk mode: ", err)
	}
	defer func(ctx context.Context) {
		if s.HasError() {
			if err := kiosk.Close(ctx); err != nil {
				s.Error("Failed to close kiosk: ", err)
			}
		}
	}(cleanupCtx)

	if err := kiosk.WaitLaunchLogs(ctx); err != nil {
		s.Fatal("Failed to launch Kiosk: ", err)
	}

	if v.lacros {
		// If we run Lacros flavor fail fast if Lacros is not up.
		testConn, err := cr.TestAPIConn(ctx)
		_, err = lacrosproc.Root(ctx, testConn)
		if err != nil {
			s.Fatal("Failed to get lacros proc: ", err)
		}
	}

	v.cr = cr
	v.kiosk = kiosk

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "vdi_kiosk_fixt_ui_tree")

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get a keyboard")
	}
	// Keep it fo closing.
	v.keyboard = kb

	detector := uidetection.New(tconn,
		s.RequiredVar("uidetection.key_type"),
		s.RequiredVar("uidetection.key"),
		s.RequiredVar("uidetection.server"))
	v.vdiConnector.Init(s, tconn, detector, kb)

	var vdiUsername, vdiPassword string
	vdiServer := s.RequiredVar(v.vdiServerKey)

	if v.useTape {
		// Create an account manager and lease a vdi test account for the specified timeout as there are several sets in the scope of this fixture.
		accHelper, acc, err := tape.NewGenericAccountManager(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)), tape.WithTimeout(30*60), tape.WithPoolID(tape.Citrix))
		if err != nil {
			s.Fatal("Failed to create an account manager and lease a Citrix account: ", err)
		}
		v.tapeAccountManager = accHelper
		vdiUsername = CitrixUsernamePrefix + acc.Username
		vdiPassword = acc.Password
	} else {
		vdiUsername = s.RequiredVar(v.vdiUsernameKey)
		vdiPassword = s.RequiredVar(v.vdiPasswordKey)
	}

	v.vdiConfig = vdiApps.VDILoginConfig{
		Server:   vdiServer,
		Username: vdiUsername,
		Password: vdiPassword,
	}

	if err := v.vdiConnector.Login(
		ctx,
		&v.vdiConfig,
	); err != nil {
		s.Fatal("Was not able to login to the VDI application: ", err)
	}

	if err := v.vdiConnector.WaitForMainScreenVisible(ctx); err != nil {
		s.Fatal("Main screen of the VDI application was not visible: ", err)
	}

	chrome.Lock()
	return &FixtureData{vdiConnector: v.vdiConnector, cr: cr, uidetector: detector, inKioskMode: true}
}

func (v *kioskFixtureState) TearDown(ctx context.Context, s *testing.FixtState) {
	// Shorten ctx to reserve time for cleanups.
	kioskCloseCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
	defer cancel()
	tapeCleanupCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if v.useTape {
		v.tapeAccountManager.CleanUp(tapeCleanupCtx)
	}

	if err := v.vdiConnector.Logout(ctx); err != nil {
		s.Error("Couldn't logout from the VDI application: ", err)
	}

	v.keyboard.Close(ctx)
	chrome.Unlock()

	if v.cr == nil {
		s.Error("Chrome not yet started")
	}

	if err := v.kiosk.Close(kioskCloseCtx); err != nil {
		s.Error("Failed to close kiosk: ", err)
	}

	v.cr = nil
}

func (v *kioskFixtureState) Reset(ctx context.Context) error {
	// Check the connection to Chrome.
	if err := v.cr.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}

	// Check the main VDI screen is on.
	if err := v.vdiConnector.WaitForMainScreenVisible(ctx); err != nil {
		return errors.Wrap(err, "VDI main screen was not present")
	}

	return nil
}

func (v *kioskFixtureState) PreTest(ctx context.Context, s *testing.FixtTestState) {}
func (v *kioskFixtureState) PostTest(ctx context.Context, s *testing.FixtTestState) {
	tconn, err := v.cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create TestAPI connection: ", err)
	}

	if err := dumpPolicies(ctx, tconn, fixtures.PolicyFileDump); err != nil {
		s.Fatal("Could not store policies: ", err)
	}

	chrome.Unlock()
	testing.ContextLog(ctx, "VDI kiosk: Restarting VDI app")
	cr, err := v.kiosk.RestartChromeWithOptions(
		ctx,
		chrome.NoLogin(),
		chrome.DMSPolicy(v.fdms.URL),
		chrome.KeepState())
	if err != nil {
		s.Fatal("Failed to connect to new chrome instance: ", err)
	}
	v.cr = cr
	chrome.Lock()

	if err := v.kiosk.WaitLaunchLogs(ctx); err != nil {
		s.Fatal("Kiosk is not started after restarting Chrome: ", err)
	}

	// A new connection is required since Chrome was restarted.
	tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Detector has to be recreated and updated in vdi connector.
	detector := uidetection.NewDefault(tconn)
	v.vdiConnector.ReplaceDetector(detector)

	// After Kiosk restart applications ask for credentials.
	if err := v.vdiConnector.EnterCredentialsAndLogin(ctx, &v.vdiConfig); err != nil {
		s.Fatal("Failed to enter credentials and login: ", err)
	}

	if err := v.vdiConnector.WaitForMainScreenVisible(ctx); err != nil {
		s.Fatal("VDI main screen was not present: ", err)
	}
}
