// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture defines fixtures for Essential apps tests.
package fixture

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/dma"
	uiCommon "go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	resetTimeout    = 30 * time.Second
	preTestTimeout  = 10 * time.Second
	postTestTimeout = 15 * time.Second
)

// List of fixture names for Essential Apps.
const (
	LoggedIn                               = "loggedIn"
	LoggedInAppParentalControls            = "loggedInAppParentalControls"
	LoggedInFieldTrialConfigDisable        = "loggedInFieldTrialConfigDisable"
	LoggedInFieldTrialConfigEnable         = "loggedInFieldTrialConfigEnable"
	LoggedInDisableInstall                 = "loggedInDisableAutoInstall"
	LoggedInJP                             = "loggedInJP"
	LoggedInGuest                          = "loggedInGuest"
	ArcBootedWithGalleryPhotosImageFeature = "arcBootedWithGalleryPhotosImageFeature"
	LacrosLoggedIn                         = "lacrosLoggedIn"
	LacrosLoggedInFieldTrialConfigDisable  = "lacrosLoggedInFieldTrialConfigDisable"
	LacrosLoggedInFieldTrialConfigEnable   = "lacrosLoggedInFieldTrialConfigEnable"
	LacrosLoggedInDisableInstall           = "lacrosLoggedInDisableAutoInstall"
	LacrosLoggedInJP                       = "lacrosLoggedInJP"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            LoggedIn,
		Desc:            "Logged into a user session for essential apps",
		Contacts:        []string{"cros-ca-eng@google.com"},
		BugComponent:    "b:385700", // ChromeOS > Software > Consumer > Apps Suite
		Impl:            eaFixture(browser.TypeAsh, true),
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name: LoggedInAppParentalControls,
		Desc: "Logged into a user session with on-device parental controls enabled",
		Contacts: []string{"cros-families-eng+test@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		BugComponent:    "b:1090157", // ChromeOS > Software > Family > Parental guidance
		Impl:            eaFixture(browser.TypeAsh, true, chrome.ARCSupported(), chrome.EnableFeatures("OnDeviceAppControls", "ForceOnDeviceAppControlsForAllRegions")),
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            LoggedInFieldTrialConfigDisable,
		Desc:            "Logged into a user session for essential apps. And field trial test config disabled",
		Contacts:        []string{"cros-ca-eng@google.com"},
		BugComponent:    "b:385700", // ChromeOS > Software > Consumer > Apps Suite
		Impl:            eaFixture(browser.TypeAsh, true, chrome.FieldTrialConfig(chrome.FieldTrialConfigDisable)),
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            LoggedInFieldTrialConfigEnable,
		Desc:            "Logged into a user session for essential apps. And field trial test config enabled",
		Contacts:        []string{"cros-ca-eng@google.com"},
		BugComponent:    "b:385700", // ChromeOS > Software > Consumer > Apps Suite
		Impl:            eaFixture(browser.TypeAsh, true, chrome.FieldTrialConfig(chrome.FieldTrialConfigEnable)),
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            LoggedInDisableInstall,
		Desc:            "Logged into a user session without installing web apps",
		Contacts:        []string{"cros-ca-eng@google.com"},
		BugComponent:    "b:385700", // ChromeOS > Software > Consumer > Apps Suite
		Impl:            eaFixture(browser.TypeAsh, false),
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            LoggedInJP,
		Desc:            "Logged into a user session for essential apps in Japanese language",
		Contacts:        []string{"cros-ca-eng@google.com"},
		BugComponent:    "b:385700", // ChromeOS > Software > Consumer > Apps Suite
		Impl:            eaFixture(browser.TypeAsh, true, chrome.Region("jp")),
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            LoggedInGuest,
		Desc:            "Logged into a guest user session for essential apps",
		Contacts:        []string{"cros-ca-eng@google.com"},
		BugComponent:    "b:385700", // ChromeOS > Software > Consumer > Apps Suite
		Impl:            eaFixture(browser.TypeAsh, true, chrome.GuestLogin()),
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	fixtureConfig := arc.DefaultBootedFixtureConfig()
	fixtureConfig.PlayStoreOptin = true
	fixtureConfig.FOpts = func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return []chrome.Option{
			chrome.EnableFeatures("MediaAppPhotosIntegrationImage:minPhotosVersionForImage/1.0"),
			chrome.ExtraArgs(arc.DisableSyncFlags()...),
			chrome.GAIALoginPool(dma.CredsFromPool(uiCommon.GaiaPoolDefaultVarName))}, nil
	}
	testing.AddFixture(&testing.Fixture{
		Name:            ArcBootedWithGalleryPhotosImageFeature,
		Desc:            "ARC is booted with the MediaAppPhotosIntegrationImage feature flag enabled",
		Contacts:        []string{"backlight-swe@google.com", "cros-ca-eng@google.com", "bugsnash@chromium.org"},
		BugComponent:    "b:562866", // ChromeOS > Software > Consumer > Apps Suite > Backlight
		Vars:            []string{uiCommon.GaiaPoolDefaultVarName},
		Impl:            arc.NewArcBootedFixture(fixtureConfig),
		SetUpTimeout:    chrome.LoginTimeout + arc.BootTimeout + ui.StartTimeout,
		ResetTimeout:    arc.ResetTimeout,
		PreTestTimeout:  arc.PreTestTimeout,
		PostTestTimeout: arc.PostTestTimeout,
		TearDownTimeout: arc.ResetTimeout,
	})

	// LacrosLoggedIn is a fixture to bring up Lacros as a primary browser
	// from the rootfs partition by default.
	// It pre-installs essential apps.
	testing.AddFixture(&testing.Fixture{
		Name:            LacrosLoggedIn,
		Desc:            "Logged into a user session with Lacros for essential apps",
		Contacts:        []string{"cros-ca-eng@google.com"},
		BugComponent:    "b:385700", // ChromeOS > Software > Consumer > Apps Suite
		Impl:            eaFixture(browser.TypeLacros, true),
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		SetUpTimeout:    chrome.LoginTimeout + time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            LacrosLoggedInFieldTrialConfigDisable,
		Desc:            "Logged into a user session with Lacros for essential apps. And field trial test config disabled",
		Contacts:        []string{"cros-ca-eng@google.com"},
		BugComponent:    "b:385700", // ChromeOS > Software > Consumer > Apps Suite
		Impl:            eaFixture(browser.TypeLacros, true, chrome.FieldTrialConfig(chrome.FieldTrialConfigDisable)),
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		SetUpTimeout:    chrome.LoginTimeout + time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            LacrosLoggedInFieldTrialConfigEnable,
		Desc:            "Logged into a user session with Lacros for essential apps. And field trial test config enabled",
		Contacts:        []string{"cros-ca-eng@google.com"},
		BugComponent:    "b:385700", // ChromeOS > Software > Consumer > Apps Suite
		Impl:            eaFixture(browser.TypeLacros, true, chrome.FieldTrialConfig(chrome.FieldTrialConfigEnable)),
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		SetUpTimeout:    chrome.LoginTimeout + time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:            LacrosLoggedInDisableInstall,
		Desc:            "Logged into a user session without installing web apps",
		Contacts:        []string{"cros-ca-eng@google.com"},
		BugComponent:    "b:385700", // ChromeOS > Software > Consumer > Apps Suite
		Impl:            eaFixture(browser.TypeLacros, false),
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	// LacrosLoggedInJP is a fixture to bring up Lacros as a primary browser
	// from the rootfs partition by default and it sets the device language
	// to Japanese.
	// It pre-installs essential apps.
	testing.AddFixture(&testing.Fixture{
		Name:            LacrosLoggedInJP,
		Desc:            "Logged into a user session with Lacros for essential apps in Japanese language",
		Contacts:        []string{"cros-ca-eng@google.com"},
		BugComponent:    "b:385700", // ChromeOS > Software > Consumer > Apps Suite
		Impl:            eaFixture(browser.TypeLacros, true, chrome.Region("jp")),
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		SetUpTimeout:    chrome.LoginTimeout + time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}

// FixtData is the data returned by SetUp and passed to tests.
type FixtData struct {
	Chrome      *chrome.Chrome
	TestAPIConn *chrome.TestConn
	BrowserType browser.Type
}

// fixtureImpl implements testing.FixtureImpl.
type fixtureImpl struct {
	cr            *chrome.Chrome  // Underlying Chrome instance
	browserType   browser.Type    // Whether Ash or Lacros is used for test
	webAppInstall bool            // Whether auto install web apps such as Canvas and Cursive.
	fOpts         []chrome.Option // Options that are passed to chrome.New
	tconn         *chrome.TestConn
	recorder      *uiauto.ScreenRecorder
}

func (f *fixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var opts []chrome.Option
	// If there's a parent fixture and the fixture supplies extra options, use them.
	if extraOpts, ok := s.ParentValue().([]chrome.Option); ok {
		opts = append(opts, extraOpts...)
	}
	opts = append(opts, f.fOpts...)
	// Default web app installation flag is false in ChromeOS options.
	if f.webAppInstall {
		opts = append(opts, chrome.EnableWebAppInstall())
	}

	// According to b/245224264, default Web app installation requires Lacros to be alive.
	lacrosOpts := []lacrosfixt.Option{lacrosfixt.KeepAlive(true)}
	if f.webAppInstall {
		lacrosOpts = append(lacrosOpts, lacrosfixt.EnableWebAppInstall())
	}
	cr, err := browserfixt.NewChrome(ctx, f.browserType, lacrosfixt.NewConfig(lacrosOpts...), opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer func() {
		if s.HasError() {
			cr.Close(ctx)
		}
	}()
	f.cr = cr

	f.tconn, err = f.cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection: ", err)
	}

	chrome.Lock()
	return FixtData{f.cr, f.tconn, f.browserType}
}

func (f *fixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	recorder, err := uiauto.NewScreenRecorder(ctx, f.tconn)
	if err != nil {
		s.Log("Failed to create screen recorder: ", err)
		return
	}
	if err := recorder.Start(ctx, f.tconn); err != nil {
		s.Log("Failed to start screen recorder: ", err)
		return
	}
	f.recorder = recorder
}

func (f *fixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// Do nothing if the recorder is not initialized.
	if f.recorder != nil {
		f.recorder.StopAndSaveOnError(ctx, filepath.Join(s.OutDir(), "record.webm"), s.HasError)
	}
}

func (f *fixtureImpl) Reset(ctx context.Context) error {
	if err := f.cr.Responded(ctx); err != nil {
		return errors.Wrap(err, "existing Chrome connection is unusable")
	}
	if err := f.cr.ResetState(ctx); err != nil {
		return errors.Wrap(err, "failed resetting existing Chrome session")
	}
	return nil
}

func (f *fixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	chrome.Unlock()
	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome connection: ", err)
	}
	f.cr = nil
	f.tconn = nil
}

func eaFixture(browserType browser.Type, webAppInstall bool, opts ...chrome.Option) testing.FixtureImpl {
	return &fixtureImpl{
		browserType:   browserType,
		webAppInstall: webAppInstall,
		fOpts:         opts,
	}
}
