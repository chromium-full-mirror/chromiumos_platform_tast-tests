// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture defines fixtures for video conferencing tests.
package fixture

import (
	"context"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"go.chromium.org/tast/core/testing"
)

// List of fixture names for video conferencing testing.
const (
	loggedIn       = "loggedInForVideoConferencing"
	loggedInLacros = "loggedInLacrosForVideoConferencing"

	gaiaLoggedIn                = "gaiaLoggedInForVideoConferencing"
	gaiaLoggedInClamshell       = "gaiaLoggedInClamshellForVideoConferencing"
	gaiaLoggedInTablet          = "gaiaLoggedInTabletForVideoConferencing"
	gaiaLoggedInLacros          = "gaiaLoggedInLacrosForVideoConferencing"
	gaiaLoggedInLacrosClamshell = "gaiaLoggedInLacrosClamshellForVideoConferencing"
	gaiaLoggedInLacrosTablet    = "gaiaLoggedInLacrosTabletForVideoConferencing"

	noLoggedIn = "noLoggedInForVideoConferencing"
)

const (
	resetTimeout    = 30 * time.Second
	preTestTimeout  = 10 * time.Second
	postTestTimeout = 15 * time.Second
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: loggedIn,
		Desc: "A fixture with fake user logged in",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@google.com",
		},
		Impl:            baseSetupFixture(browser.TypeAsh, nil),
		Parent:          fixture.StereoAloopLoaded,
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: gaiaLoggedIn,
		Desc: "A fixture with GAIA user logged in",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@google.com",
		},
		Vars: []string{"ui.gaiaPoolDefault"},
		Impl: baseSetupFixture(browser.TypeAsh, func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault"))}, nil
		}),
		Parent:          fixture.StereoAloopLoaded,
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: gaiaLoggedInClamshell,
		Desc: "A fixture with GAIA user logged in forcing clamshell mode",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@google.com",
		},
		Parent: fixture.StereoAloopLoaded,
		Vars:   []string{"ui.gaiaPoolDefault"},
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
		Name: gaiaLoggedInTablet,
		Desc: "A fixture with GAIA user logged in forcing tablet mode",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@google.com",
		},
		Vars: []string{"ui.gaiaPoolDefault"},
		Impl: baseSetupFixture(browser.TypeAsh, func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
				chrome.ExtraArgs("--force-tablet-mode=touch_view"),
			}, nil
		}),
		Parent:          fixture.StereoAloopLoaded,
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: loggedInLacros,
		Desc: "A fixture with fake user logged in Lacros",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@google.com",
		},
		Impl:            baseSetupFixture(browser.TypeLacros, nil),
		Parent:          fixture.StereoAloopLoaded,
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: gaiaLoggedInLacros,
		Desc: "A fixture with GAIA user logged in Lacros",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@google.com",
		},
		Vars: []string{"ui.gaiaPoolDefault"},
		Impl: baseSetupFixture(browser.TypeLacros, func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
			}, nil
		}),
		Parent:          fixture.StereoAloopLoaded,
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: gaiaLoggedInLacrosClamshell,
		Desc: "A fixture with GAIA user logged in Lacros in clamshell mode",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@google.com",
		},
		Vars: []string{"ui.gaiaPoolDefault"},
		Impl: baseSetupFixture(browser.TypeLacros, func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
				chrome.ExtraArgs("--force-tablet-mode=clamshell"),
			}, nil
		}),
		Parent:          fixture.StereoAloopLoaded,
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: gaiaLoggedInLacrosTablet,
		Desc: "A fixture with GAIA user logged in Lacros in tablet mode",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@google.com",
		},
		Vars: []string{"ui.gaiaPoolDefault"},
		Impl: baseSetupFixture(browser.TypeLacros, func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
				chrome.ExtraArgs("--force-tablet-mode=touch_view"),
			}, nil
		}),
		Parent:          fixture.StereoAloopLoaded,
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: noLoggedIn,
		Desc: "A fixture with no user logged in",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@google.com",
		},
		Impl: baseSetupFixture(browser.TypeAsh, func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{chrome.NoLogin()}, nil
		}),
		Parent:          fixture.StereoAloopLoaded,
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

// baseSetupFixtData is the data returned by SetUp and passed to tests.
type baseSetupFixtData struct {
	cr *chrome.Chrome
	bt browser.Type
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
	// Set VC related flags by default for all fixtures.
	var opts = []chrome.Option{
		chrome.EnableFeatures("SpeakOnMuteEnabled"),
		chrome.EnableFeatures("VideoConference"),
		chrome.EnableFeatures("CrOSLateBootAudioFlexibleLoopback"),
		chrome.EnableFeatures("SystemLiveCaption"),
		// Disable WindowLayoutMenu to prevent `Keep hovering for more layout options` nudge.
		// The nudge is overlap with app window and causes screen diff flakiness.
		chrome.DisableFeatures("WindowLayoutMenu"),
	}

	var err error
	if f.fOpts != nil {
		fOpts, err := f.fOpts(ctx, s)
		if err != nil {
			s.Fatal("Failed to get Chrome options: ", err)
		}
		opts = append(opts, fOpts...)
	}

	// keep-alive for lacros extension apps. A no-op for ash extensions.
	cr, err := browserfixt.NewChrome(ctx, f.browserType, lacrosfixt.NewConfig(
		lacrosfixt.KeepAlive(true),
	), opts...)
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

	return &baseSetupFixtData{f.cr, f.browserType}
}

func (f *baseSetupFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *baseSetupFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *baseSetupFixtureImpl) Reset(ctx context.Context) error {
	return nil
}

func (f *baseSetupFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome connection: ", err)
	}
	f.cr = nil
}
