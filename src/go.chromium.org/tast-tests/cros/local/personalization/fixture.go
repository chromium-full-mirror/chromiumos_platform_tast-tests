// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package personalization

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"

	"go.chromium.org/tast/core/testing"
)

func getTimeOfDayOption() chrome.Option {
	return chrome.EnableFeatures(
		"Jelly",
		"TimeOfDayScreenSaver",
		"TimeOfDayWallpaper")
}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "personalizationWithClamshell",
		Desc: "Login with Personalization Hub in clamshell mode",
		Contacts: []string{
			"thuongphan@google.com",
			"chromeos-sw-engprod@google.com",
			"assistive-eng@google.com",
		},
		Impl:            &clamshellFixture{},
		Parent:          "chromeLoggedIn",
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "personalizationWithGaiaLogin",
		Desc: "Login using Gaia account with Personalization Hub enabled",
		Contacts: []string{
			"thuongphan@google.com",
			"chromeos-sw-engprod@google.com",
			"assistive-eng@google.com",
		},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.GAIALogin(chrome.Creds{
					User: s.RequiredVar("ambient.username"),
					Pass: s.RequiredVar("ambient.password"),
				}),
			}, nil
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars: []string{
			"ambient.username",
			"ambient.password",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "personalizationWithGaiaLoginClamshell",
		Desc: "Login using Gaia account with Personalization Hub enabled",
		Contacts: []string{
			"thuongphan@google.com",
			"chromeos-sw-engprod@google.com",
			"assistive-eng@google.com",
		},
		Impl:            &clamshellFixture{},
		Parent:          "personalizationWithGaiaLogin",
		SetUpTimeout:    chrome.GAIALoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars: []string{
			"ambient.username",
			"ambient.password",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "personalizationWithRgbKeyboard",
		Desc: "Login with Personalization Hub and RGB Keyboard enabled",
		Contacts: []string{
			"thuongphan@google.com",
			"chromeos-sw-engprod@google.com",
			"assistive-eng@google.com",
		},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{chrome.EnableFeatures("RgbKeyboard")}, nil
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "personalizationWithMultizoneRgbKeyboard",
		Desc: "Login with Personalization Hub with Multi-zone RGB Keyboard enabled",
		Contacts: []string{
			"thuongphan@google.com",
			"chromeos-sw-engprod@google.com",
			"assistive-eng@google.com",
		},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{chrome.EnableFeatures("RgbKeyboard", "MultiZoneRgbKeyboard")}, nil
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "personalizationWithGooglePhotosWallpaper",
		Desc: "Login with Gaia account with Google Photos Wallpaper enabled",
		Contacts: []string{
			"thuongphan@google.com",
			"chromeos-sw-engprod@google.com",
			"assistive-eng@google.com",
		},
		// Setting Google Photos wallpapers requires that Chrome be logged in with
		// a user from an account pool which has been preconditioned to have a
		// Google Photos library with specific photos/albums present. Note that sync
		// is disabled to prevent flakiness caused by wallpaper cross device sync.
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.GAIALoginPool(s.RequiredVar("wallpaper.googlePhotosAccountPool")),
				chrome.EnableFeatures("WallpaperGooglePhotosIntegration"),
				chrome.EnableFeatures("WallpaperGooglePhotosSharedAlbums"),
				chrome.ExtraArgs("--disable-sync"),
			}, nil
		}),
		Vars: []string{
			"wallpaper.googlePhotosAccountPool",
		},
		SetUpTimeout:    chrome.GAIALoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "personalizationWithAvatarsCloudMigration",
		Desc: "Login with Personalization Hub and Avatars Cloud migration enabled",
		Contacts: []string{
			"updowndota@google.com",
			"chromeos-sw-engprod@google.com",
			"assistive-eng@google.com",
		},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{chrome.EnableFeatures("AvatarsCloudMigration")}, nil
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "personalizationScreenSaver",
		Desc: "Login with Gaia account that has google photos albums for screen saver tests",
		Contacts: []string{
			"assistive-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"cowmoo@google.com",
		},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.GAIALoginPool(s.RequiredVar("wallpaper.googlePhotosAccountPool")),
				getTimeOfDayOption(),
			}, nil
		}),
		SetUpTimeout:    chrome.GAIALoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars: []string{
			"wallpaper.googlePhotosAccountPool",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "personalizationScreenSaverClamshell",
		Desc: "Login with Gaia account that has google photos albums for screen saver tests in Clamshell mode",
		Contacts: []string{
			"assistive-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"cowmoo@google.com",
		},
		Impl:            &clamshellFixture{},
		Parent:          "personalizationScreenSaver",
		SetUpTimeout:    chrome.GAIALoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars: []string{
			"ui.gaiaPoolDefault",
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: "personalizationWithTimeOfDayFeature",
		Desc: "Login with Personalization Hub with Time of Day feature enabled",
		Contacts: []string{
			"assistive-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"jasontt@google.com",
		},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{getTimeOfDayOption()}, nil
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: "personalizationWithTimeOfDayFeatureClamshell",
		Desc: "Login with Personalization Hub with Time of Day feature enabled in clamshell mode",
		Contacts: []string{
			"assistive-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"jasontt@google.com",
		},
		Impl:            &clamshellFixture{},
		Parent:          "personalizationWithTimeOfDayFeature",
		SetUpTimeout:    chrome.GAIALoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}

type clamshellFixture struct {
	cleanup func(ctx context.Context) error
	cr      *chrome.Chrome
}

func (f *clamshellFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cr := s.ParentValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure DUT is not in tablet mode: ", err)
	}
	f.cleanup = cleanup

	// If a DUT switches from Tablet mode to Clamshell mode, it can take a while
	// until launcher gets settled down.
	if err := ash.WaitForLauncherState(ctx, tconn, ash.Closed); err != nil {
		s.Fatal("Failed to wait the launcher state Closed: ", err)
	}

	f.cr = cr
	return cr
}

func (f *clamshellFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.cleanup != nil {
		f.cleanup(ctx)
	}
	f.cr = nil
}

func (f *clamshellFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *clamshellFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *clamshellFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
