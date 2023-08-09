// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture defines fixtures for video conferencing tests.
package fixture

import (
	"context"
	"io/ioutil"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/loginstatus"

	"go.chromium.org/tast/core/errors"
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

	baseLoggedInWithFakeVCExtension       = "baseLoggedInWithFakeVCExtension"
	baseLoggedInLacrosWithFakeVCExtension = "baseLoggedInLacrosWithFakeVCExtension"

	noLoggedIn = "noLoggedInForVideoConferencing"
)

var fakeVCExtension = "fake_vc_extension.zip"

const (
	resetTimeout    = 30 * time.Second
	preTestTimeout  = 10 * time.Second
	postTestTimeout = 15 * time.Second
)

// Set VC related flags by default for all fixtures.
var vcOpts = []chrome.Option{
	chrome.EnableFeatures("CrosPrivacyHub"),
	chrome.EnableFeatures("VideoConference"),
	chrome.EnableFeatures("CrOSLateBootAudioFlexibleLoopback"),
	chrome.EnableFeatures("SystemLiveCaption"),
	chrome.EnableFeatures("FeatureManagementVideoConference"),
	chrome.EnableFeatures("ShowLiveCaptionInVideoConferenceTray"),
	chrome.ExtraArgs("--disable-sync"),
}

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

	testing.AddFixture(&testing.Fixture{
		Name: baseLoggedInWithFakeVCExtension,
		Desc: "A fixture with fake user logged in and fake VC extension installed",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@google.com",
		},
		Data:            []string{fakeVCExtension},
		Impl:            baseSetupFixtureWithFakeExtension(browser.TypeAsh, nil),
		Parent:          fixture.StereoAloopLoaded,
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: baseLoggedInLacrosWithFakeVCExtension,
		Desc: "A fixture with fake user logged in Lacros and fake VC extension installed",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@google.com",
		},
		Data:            []string{fakeVCExtension},
		Impl:            baseSetupFixtureWithFakeExtension(browser.TypeLacros, nil),
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

func baseSetupFixtureWithFakeExtension(browserType browser.Type, fOpts chrome.OptionsCallback) testing.FixtureImpl {
	return &baseSetupFixtureImpl{
		browserType: browserType,
		fOpts:       fOpts,
		installExt:  true,
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
	installExt  bool // Whether to install fake VC extension.
}

func (f *baseSetupFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	opts := vcOpts

	if f.installExt {
		s.Log("Copying extension to temp directory")
		extDir, err := ioutil.TempDir("", "videoconferencing")
		if err != nil {
			s.Fatal("Failed to create temp dir: ", err)
		}
		defer os.RemoveAll(extDir)

		if err := testexec.CommandContext(ctx, "unzip", s.DataPath(fakeVCExtension), "-d", extDir).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to unzip fake extension : ", err)
		}

		extPath := extDir + "/vc_extension"
		// Set option to install unpacked extension.
		if f.browserType == browser.TypeAsh {
			opts = append(opts, chrome.UnpackedExtension(extPath))
		} else {
			opts = append(opts, chrome.LacrosUnpackedExtension(extPath))
		}
	}

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
	// Check oauth2 token is still valid. If not, return an error to restart
	// chrome and re-login.
	if f.cr.LoginMode() == "GAIA" {
		if st, err := loginstatus.GetLoginStatus(ctx, f.tconn); err != nil {
			return errors.Wrap(err, "failed to get login status")
		} else if !*st.HasValidOauth2Token {
			return errors.New("invalid oauth2 token")
		}
	}

	if err := f.cr.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}
	if err := f.cr.ResetState(ctx); err != nil {
		return errors.Wrap(err, "failed resetting existing Chrome session")
	}
	if err := f.tconn.WaitForExpr(ctx, `document.readyState === "complete"`); err != nil {
		return errors.Wrap(err, "test API extension becomes unavailable")
	}
	return nil
}

func (f *baseSetupFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome connection: ", err)
	}
	f.cr = nil
}
