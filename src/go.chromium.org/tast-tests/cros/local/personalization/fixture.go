// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package personalization

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/wallpaper"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"

	"go.chromium.org/tast/core/testing"
)

const (
	preTestTimeout  = 10 * time.Second
	postTestTimeout = 10 * time.Second
)

// Set of public fixture names for personalization tests.
const (
	BaseFixture                  = "personalizationBase"
	ClamshellFixture             = "personalizationClamshell"
	GaiaFixture                  = "personalizationGaiaLogin"
	GooglePhotosFixture          = "personalizationGooglePhotos"
	GooglePhotosClamshellFixture = "personalizationGooglePhotosClamshell"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: BaseFixture,
		Desc: "Base fixture for Personalization tests",
		Contacts: []string{
			"pzliu@google.com",
			"cowmoo@google.com",
			"chromeos-sw-engprod@google.com",
			"assistive-eng@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent:    "b:1006527",
		Impl:            baseFixture(asOptionsCallback()),
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: ClamshellFixture,
		Desc: "Clamshell mode for Personalization tests",
		Contacts: []string{
			"thuongphan@google.com",
			"chromeos-sw-engprod@google.com",
			"assistive-eng@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent:    "b:1006527",
		Impl:            baseFixture(asOptionsCallback(chrome.ExtraArgs("--force-tablet-mode=clamshell"))),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: GaiaFixture,
		Desc: "Login using Gaia account for Personalization tests",
		Contacts: []string{
			"thuongphan@google.com",
			"chromeos-sw-engprod@google.com",
			"assistive-eng@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Impl: baseFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.GAIALogin(chrome.Creds{
					User: s.RequiredVar("ambient.username"),
					Pass: s.RequiredVar("ambient.password"),
				}),
			}, nil
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars: []string{
			"ambient.username",
			"ambient.password",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: GooglePhotosFixture,
		Desc: "Login with Gaia account with Google Photos Wallpaper enabled and with existing photos",
		Contacts: []string{
			"thuongphan@google.com",
			"chromeos-sw-engprod@google.com",
			"assistive-eng@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		// Setting Google Photos wallpapers requires that Chrome be logged in with
		// a user from an account pool which has been preconditioned to have a
		// Google Photos library with specific photos/albums present. Note that sync
		// is disabled to prevent flakiness caused by wallpaper cross device sync.
		Impl: baseFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.GAIALoginPool(dma.CredsFromPool(wallpaper.GooglePhotosAccountPoolVarName)),
				chrome.ExtraArgs("--disable-sync"),
			}, nil
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: GooglePhotosClamshellFixture,
		Desc: "Login with Gaia account that has google photos albums for screen saver tests in Clamshell mode",
		Contacts: []string{
			"assistive-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"cowmoo@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Impl: baseFixture(
			func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
				return []chrome.Option{
					chrome.GAIALoginPool(dma.CredsFromPool(wallpaper.GooglePhotosAccountPoolVarName)),
					chrome.ExtraArgs("--disable-sync"),
					chrome.ExtraArgs("--force-tablet-mode=clamshell"),
				}, nil
			}),
		SetUpTimeout:    chrome.GAIALoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}

// BaseFixtureData is passed as fixture data to each test.
type BaseFixtureData struct {
	cr *chrome.Chrome
}

// Chrome implements HasChrome interface so test cases can access chrome.
func (f BaseFixtureData) Chrome() *chrome.Chrome { return f.cr }

type personalizationBaseFixtureImpl struct {
	cr       *chrome.Chrome
	fOpts    chrome.OptionsCallback
	recorder *uiauto.ScreenRecorder
	tconn    *chrome.TestConn
}

func (f *personalizationBaseFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	opts, err := f.fOpts(ctx, s)
	if err != nil {
		s.Fatal("Failed to get Chrome options: ", err)
	}

	cr, err := browserfixt.NewChrome(ctx, browser.TypeAsh, nil, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer func() {
		if s.HasError() {
			cr.Close(ctx)
		}
	}()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection: ", err)
	}

	f.cr = cr
	f.tconn = tconn

	return BaseFixtureData{f.cr}
}

func (f *personalizationBaseFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
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

func (f *personalizationBaseFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// Do nothing if the recorder is not initialized.
	if f.recorder != nil {
		f.recorder.StopAndSaveOnError(ctx, filepath.Join(s.OutDir(), "record.webm"), s.HasError)

	}
}

func (f *personalizationBaseFixtureImpl) Reset(ctx context.Context) error {
	return nil
}

func (f *personalizationBaseFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome: ", err)
	}

	f.cr = nil
	f.tconn = nil
}

func asOptionsCallback(options ...chrome.Option) chrome.OptionsCallback {
	return func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
		return options, nil
	}
}

func baseFixture(optionsCallback chrome.OptionsCallback) testing.FixtureImpl {
	return &personalizationBaseFixtureImpl{fOpts: optionsCallback}
}
