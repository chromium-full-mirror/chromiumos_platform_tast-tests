// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture defines fixtures for inputs tests.
package fixture

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/inputs/inputactions"
	"go.chromium.org/tast-tests/cros/local/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosproc"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/kioskmode"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// List of fixture names for inputs using kiosk mode.
const (
	// KioskNonVK is the fixture for physical keyboard in Kiosk mode for ash.
	KioskNonVK = "kioskNonVK"
	// KioskNonVK is the fixture for physical keyboard in Kiosk mode for lacros.
	LacrosKioskNonVK = "lacrosKioskNonVK"
	// KioskNonVK is the fixture for virtual keyboard in Kiosk mode for ash.
	KioskVK = "kioskVK"
	// KioskNonVK is the fixture for virtual keyboard in Kiosk mode for lacros.
	LacrosKioskVK = "lacrosKioskVK"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: KioskNonVK,
		Desc: "Fixture should be used to test physical keyboard typing in kiosk mode (ash chrome) with e14s-test page loaded",
		Contacts: []string{
			"jhtin@chromium.org",
			"alt-modalities-stability@google.com",
		},
		Impl:            &inputsKioskFixture{},
		SetUpTimeout:    kioskmode.SetupDuration + kioskmode.LaunchDuration,
		TearDownTimeout: 5*time.Second + kioskmode.CleanupDuration, // Chrome unlock time + kiosk clean up time.
		ResetTimeout:    chrome.ResetTimeout,
		Parent:          fixture.FakeDMSEnrolled,
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosKioskNonVK,
		Desc: "Fixture should be used to test physical keyboard typing in kiosk mode (lacros chrome) with e14s-test page loaded",
		Contacts: []string{
			"jhtin@chromium.org",
			"alt-modalities-stability@google.com",
		},
		Impl: &inputsKioskFixture{
			extraPublicAccountPolicies: []policy.Policy{
				&policy.LacrosAvailability{Val: "lacros_only"},
			},
			lacros: true,
		},
		SetUpTimeout:    kioskmode.SetupDuration + kioskmode.LaunchDuration,
		TearDownTimeout: 5*time.Second + kioskmode.CleanupDuration, // Chrome unlock time + kiosk clean up time.
		ResetTimeout:    chrome.ResetTimeout,
		Parent:          fixture.FakeDMSEnrolled,
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: KioskVK,
		Desc: "Fixture should be used to test virtual keyboard typing in kiosk mode (ash chrome) with e14s-test page loaded",
		Contacts: []string{
			"jhtin@chromium.org",
			"alt-modalities-stability@google.com",
		},
		Impl: &inputsKioskFixture{
			extraOpts: []chrome.Option{chrome.VKEnabled()},
		},
		SetUpTimeout:    kioskmode.SetupDuration + kioskmode.LaunchDuration,
		TearDownTimeout: 5*time.Second + kioskmode.CleanupDuration, // Chrome unlock time + kiosk clean up time.
		ResetTimeout:    chrome.ResetTimeout,
		Parent:          fixture.FakeDMSEnrolled,
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosKioskVK,
		Desc: "Fixture should be used to test virtual keyboard typing in kiosk mode (lacros chrome) with e14s-test page loaded",
		Contacts: []string{
			"jhtin@chromium.org",
			"alt-modalities-stability@google.com",
		},
		Impl: &inputsKioskFixture{
			extraOpts: []chrome.Option{chrome.VKEnabled(), chrome.ExtraArgs("--force-tablet-mode=touch_view")},
			extraPublicAccountPolicies: []policy.Policy{
				&policy.LacrosAvailability{Val: "lacros_only"},
			},
			lacros: true,
		},
		SetUpTimeout:    kioskmode.SetupDuration + kioskmode.LaunchDuration,
		TearDownTimeout: 5*time.Second + kioskmode.CleanupDuration, // Chrome unlock time + kiosk clean up time.
		ResetTimeout:    chrome.ResetTimeout,
		Parent:          fixture.FakeDMSEnrolled,
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey"},
	})
}

type inputsKioskFixture struct {
	cr         *chrome.Chrome
	testserver *httptest.Server
	kiosk      *kioskmode.Kiosk
	extraOpts  []chrome.Option
	// extraPublicAccountPolicies holds a policies that will be applied.
	extraPublicAccountPolicies []policy.Policy
	tconn                      *chrome.TestConn
	uc                         *useractions.UserContext
	// lacros is a flag indicating whether fixture implementation suppose to run Lacros.
	lacros                         bool
	signinTestExtensionManifestKey string
}

// InputsKioskFixtData is the data returned by kiosk fixture SetUp and passed to tests.
type InputsKioskFixtData struct {
	chrome      *chrome.Chrome
	TestAPIConn *chrome.TestConn
	UserContext *useractions.UserContext
}

// Chrome implements the HasChrome interface.
func (f InputsKioskFixtData) Chrome() *chrome.Chrome {
	if f.chrome == nil {
		panic("Chrome is called with nil chrome instance")
	}
	return f.chrome
}

func (k *inputsKioskFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	fdms, ok := s.ParentValue().(*fakedms.FakeDMS)
	if !ok {
		s.Fatal("Parent is not a fakeDMSEnrolled fixture")
	}
	// Launches the server for e14s-test page.
	k.testserver = testserver.LaunchServer(ctx)

	k.signinTestExtensionManifestKey = s.RequiredVar("ui.signinProfileTestExtensionManifestKey")

	// Creating a kiosk mode configuration that will launch an app that points to the e14s-test page.
	webKioskAccountID := "arbitrary_id_web_kiosk_1@managedchrome.com"
	webKioskAccountType := policy.AccountTypeWebKioskApp
	webKioskTitle := "TastKioskModeSetByPolicyE14sPage"
	webKioskURL := k.testserver.URL + "/e14s-test"
	webKioskPolicy := policy.DeviceLocalAccountInfo{
		AccountID:   &webKioskAccountID,
		AccountType: &webKioskAccountType,
		WebKioskAppInfo: &policy.WebKioskAppInfo{
			Url:     &webKioskURL,
			Title:   &webKioskTitle,
			IconUrl: &webKioskURL,
		}}

	localAccountsConfiguration := &policy.DeviceLocalAccounts{
		Val: []policy.DeviceLocalAccountInfo{
			webKioskPolicy,
		},
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
	defer cancel()

	kiosk, cr, err := kioskmode.New(
		ctx,
		fdms,
		k.signinTestExtensionManifestKey,
		kioskmode.AutoLaunch(webKioskAccountID),
		kioskmode.CustomLocalAccounts(localAccountsConfiguration),
		kioskmode.ExtraChromeOptions(k.extraOpts...),
		kioskmode.PublicAccountPolicies(webKioskAccountID, k.extraPublicAccountPolicies),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
	}
	defer func(ctx context.Context) {
		if s.HasError() {
			if err := kiosk.Close(ctx); err != nil {
				s.Error("Failed to close Kiosk: ", err)
			}
		}
	}(cleanupCtx)

	if err := kiosk.WaitLaunchLogs(ctx); err != nil {
		s.Fatal("Failed to launch Kiosk: ", err)
	}

	k.cr = cr
	k.kiosk = kiosk

	k.tconn, err = k.cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get test API connection")
	}

	if k.lacros {
		if _, err = lacrosproc.Root(ctx, k.tconn); err != nil {
			s.Fatal("Failed to get lacros proc: ", err)
		}
	}

	uc, err := inputactions.NewInputsUserContextWithoutState(ctx, "", s.OutDir(), k.cr, k.tconn, nil)
	if err != nil {
		return errors.Wrap(err, "failed to create new inputs user context")
	}
	k.uc = uc

	chrome.Lock()
	return InputsKioskFixtData{k.cr, k.tconn, k.uc}
}

func (k *inputsKioskFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	chrome.Unlock()
	if k.cr == nil {
		s.Log("Chrome not yet started")
	}

	if err := k.kiosk.Close(ctx); err != nil {
		s.Error("Failed to close Kiosk: ", err)
	}

	k.testserver.Close()
	k.cr = nil
	k.kiosk = nil
	k.tconn = nil
}

func (k *inputsKioskFixture) Reset(ctx context.Context) error {
	if err := k.tconn.Eval(ctx, "chrome.tabs.reload()", nil); err != nil {
		return errors.Wrap(err, "failed to run chrome.tabs.reload()")
	}

	if err := k.tconn.WaitForExpr(ctx, "document.readyState == 'complete'"); err != nil {
		return errors.Wrap(err, "document did not load after reload")
	}
	return nil
}

func (k *inputsKioskFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// filepath.Base(s.OutDir()) returns the test name.
	// TODO(b/235164130) use s.TestName once it is available.
	k.uc.SetTestName(filepath.Base(s.OutDir()))
}

func (k *inputsKioskFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
