// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture defines fixtures for ML service tests.
package fixture

import (
	"context"
	"path/filepath"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/testing"
)

// List of fixture names for ML service testing.
const (
	LoggedIn              = "mlLoggedIn"
	GAIALoggedIn          = "mlGaiaLoggedIn"
	GAIALoggedInClamshell = "mlGAIALoggedInClamshell"
	GAIALoggedInTablet    = "mlGAIALoggedInTablet"
	NoLoggedIn            = "mlNoLoggedIn"
)

const (
	resetTimeout    = 30 * time.Second
	preTestTimeout  = 10 * time.Second
	postTestTimeout = 15 * time.Second
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: LoggedIn,
		Desc: "A fixture with fake user logged in",
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"shengjun@google.com",
		},
		Impl:            baseSetupFixture(browser.TypeAsh, nil),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: GAIALoggedIn,
		Desc: "A fixture with GAIA user logged in",
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"shengjun@google.com",
		},
		Vars: []string{"ui.gaiaPoolDefault"},
		Impl: baseSetupFixture(browser.TypeAsh, func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault"))}, nil
		}),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: GAIALoggedInClamshell,
		Desc: "A fixture with GAIA user logged in",
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"shengjun@google.com",
		},
		Vars: []string{"ui.gaiaPoolDefault"},
		Impl: baseSetupFixture(browser.TypeAsh, func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
				chrome.ExtraArgs("--force-tablet-mode=clamshell"),
			}, nil
		}),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: GAIALoggedInTablet,
		Desc: "A fixture with GAIA user logged in",
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"shengjun@google.com",
		},
		Vars: []string{"ui.gaiaPoolDefault"},
		Impl: baseSetupFixture(browser.TypeAsh, func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
				chrome.ExtraArgs("--force-tablet-mode=touch_view"),
			}, nil
		}),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: NoLoggedIn,
		Desc: "A fixture with no user logged in",
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"shengjun@google.com",
		},
		Impl: baseSetupFixture(browser.TypeAsh, func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{chrome.NoLogin()}, nil
		}),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}

func baseSetupFixture(browserType browser.Type, fOpts chrome.OptionsCallback) testing.FixtureImpl {
	return &baseSetupFixtureImpl{
		browserType: browserType,
		fOpts:       fOpts,
	}
}

// BaseSetupFixtData is the data returned by SetUp and passed to tests.
type BaseSetupFixtData struct {
	cr *chrome.Chrome
	bt browser.Type
}

// Chrome returns Chrome. This adds support for chrome.HasChrome interface.
func (fd BaseSetupFixtData) Chrome() *chrome.Chrome {
	return fd.cr
}

// BrowserType returns the browser type setup in fixture.
func (fd BaseSetupFixtData) BrowserType() browser.Type {
	return fd.bt
}

// baseSetupFixtureImpl implements testing.FixtureImpl.
type baseSetupFixtureImpl struct {
	cr          *chrome.Chrome         // Underlying Chrome instance
	browserType browser.Type           // Whether Ash or Lacros is used for test
	fOpts       chrome.OptionsCallback // Function to return chrome options.
	tconn       *chrome.TestConn
	recorder    *uiauto.ScreenRecorder
}

func (f *baseSetupFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Start Chrome instance.
	var fOpts = []chrome.Option{}
	var err error
	if f.fOpts != nil {
		fOpts, err = f.fOpts(ctx, s)
		if err != nil {
			s.Fatal("Failed to get Chrome options: ", err)
		}
	}
	cr, err := browserfixt.NewChrome(ctx, f.browserType, lacrosfixt.NewConfig(), fOpts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	f.cr = cr

	if cr.LoginMode() != "NoLogin" {
		// cr.TestAPIConn does not work on login page.
		// It can be achieved via cr.SigninProfileTestAPIConn(ctx) but not necessary.
		f.tconn, err = f.cr.TestAPIConn(ctx)
		if err != nil {
			s.Fatal("Failed to get test API connection: ", err)
		}
	}

	return BaseSetupFixtData{f.cr, f.browserType}
}

func (f *baseSetupFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// Do not setup recorder if f.tconn is not created.
	if f.tconn == nil {
		return
	}

	f.recorder = uiauto.CreateAndStartScreenRecorder(ctx, f.tconn)
}

func (f *baseSetupFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// Do nothing if the recorder is not initialized.
	if f.recorder != nil {
		f.recorder.StopAndSaveOnError(ctx, filepath.Join(s.OutDir(), "record.webm"), s.HasError)
	}
}

func (f *baseSetupFixtureImpl) Reset(ctx context.Context) error {
	if err := f.cr.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}
	if err := f.cr.ResetState(ctx); err != nil {
		return errors.Wrap(err, "failed resetting existing Chrome session")
	}
	return nil
}

func (f *baseSetupFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome connection: ", err)
	}
	f.cr = nil
}
