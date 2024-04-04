// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture defines fixtures for launcher search tests.
package fixture

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/oobe"
	"go.chromium.org/tast/core/testing"
)

const (
	launcherSearchSetUpTestTimeout = 4 * time.Minute
	launcherSearchPreTestTimeout   = 10 * time.Second
	launcherSearchPostTestTimeout  = 10 * time.Second
	arcOptinTimeout                = 3 * time.Minute
	oobeTimeout                    = 2 * time.Minute
)

// fixture's name
const (
	LauncherImageSearchIcaAndOcr = "launcherImageSearchIcaAndOcr"
	LauncherImageSearchOcr       = "launcherImageSearchOcr"
	LauncherImageSearchOcrNonCBX = "launcherImageSearchOcrNonCBX"
	LauncherImageSearchIca       = "launcherImageSearchIca"
	LauncherImageSearch          = "launcherImageSearch"
	NormalLauncherSearch         = "normalLauncherSearch"
	NormalLauncherSearchWithArc  = "normalLauncherSearchWithArc"
	NormalLauncherSearchWithOOBE = "normalLauncherSearchWithOOBE"
)

// launcherSearchFixtureImpl implements testing.FixtureImpl.
type launcherSearchFixtureImpl struct {
	featureFlags []string // Feature flags for testing.
	tconn        *chrome.TestConn
	cr           *chrome.Chrome
	kb           *input.KeyboardEventWriter
	recorder     *uiauto.ScreenRecorder
	ARCSupported bool
	CompleteOOBE bool
}

// LauncherSearchFixtData is the data returned by SetUp and passed to tests.
type LauncherSearchFixtData struct {
	Chrome      *chrome.Chrome
	TestAPIConn *chrome.TestConn
	Keyboard    *input.KeyboardEventWriter
}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: LauncherImageSearchIcaAndOcr,
		Desc: "Turn on LauncherImageSearchIcaAndOcr",
		Contacts: []string{
			"xiuwen@google.com",
			"ml-service-team@google.com",
		},
		Impl:            &launcherSearchFixtureImpl{featureFlags: []string{"ProductivityLauncherImageSearch", "LauncherImageSearch", "LauncherImageSearchOcr", "LauncherImageSearchIca"}},
		SetUpTimeout:    launcherSearchSetUpTestTimeout,
		PreTestTimeout:  launcherSearchPreTestTimeout,
		PostTestTimeout: launcherSearchPostTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LauncherImageSearchOcr,
		Desc: "Turn on LauncherImageSearchOcr",
		Contacts: []string{
			"xiuwen@google.com",
			"ml-service-team@google.com",
		},
		Impl:            &launcherSearchFixtureImpl{featureFlags: []string{"ProductivityLauncherImageSearch", "LauncherImageSearch", "LauncherImageSearchOcr"}},
		SetUpTimeout:    launcherSearchSetUpTestTimeout,
		PreTestTimeout:  launcherSearchPreTestTimeout,
		PostTestTimeout: launcherSearchPostTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LauncherImageSearchOcrNonCBX,
		Desc: "Turn on LauncherImageSearchOcr on non cbx device",
		Contacts: []string{
			"xiuwen@google.com",
			"ml-service-team@google.com",
		},
		Impl:            &launcherSearchFixtureImpl{featureFlags: []string{"ProductivityLauncherImageSearch", "LauncherImageSearch", "LauncherImageSearchOcr", "ICASupportedByHardware", "FeatureManagementLocalImageSearch"}},
		SetUpTimeout:    launcherSearchSetUpTestTimeout,
		PreTestTimeout:  launcherSearchPreTestTimeout,
		PostTestTimeout: launcherSearchPostTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LauncherImageSearchIca,
		Desc: "Turn on LauncherImageSearchIca",
		Contacts: []string{
			"xiuwen@google.com",
			"ml-service-team@google.com",
		},
		Impl:            &launcherSearchFixtureImpl{featureFlags: []string{"ProductivityLauncherImageSearch", "LauncherImageSearch", "LauncherImageSearchIca"}},
		SetUpTimeout:    launcherSearchSetUpTestTimeout,
		PreTestTimeout:  launcherSearchPreTestTimeout,
		PostTestTimeout: launcherSearchPostTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LauncherImageSearch,
		Desc: "Turn on ProductivityLauncherImageSearch and LauncherImageSearch",
		Contacts: []string{
			"xiuwen@google.com",
			"ml-service-team@google.com",
		},
		Impl:            &launcherSearchFixtureImpl{featureFlags: []string{"ProductivityLauncherImageSearch", "LauncherImageSearch"}},
		SetUpTimeout:    launcherSearchSetUpTestTimeout,
		PreTestTimeout:  launcherSearchPreTestTimeout,
		PostTestTimeout: launcherSearchPostTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: NormalLauncherSearch,
		Desc: "Normal launcher search",
		Contacts: []string{
			"xiuwen@google.com",
			"ml-service-team@google.com",
		},
		Impl:            &launcherSearchFixtureImpl{featureFlags: []string{"LauncherSystemInfoAnswerCards"}}, //remove flag once answer card released.
		SetUpTimeout:    launcherSearchSetUpTestTimeout + arcOptinTimeout,
		PreTestTimeout:  launcherSearchPreTestTimeout,
		PostTestTimeout: launcherSearchPostTestTimeout,
		Vars:            []string{"ui.gaiaPoolDefault"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: NormalLauncherSearchWithArc,
		Desc: "Normal launcher search with Acr++ turn on",
		Contacts: []string{
			"xiuwen@google.com",
			"ml-service-team@google.com",
		},
		Impl:            &launcherSearchFixtureImpl{featureFlags: []string{}, ARCSupported: true},
		SetUpTimeout:    launcherSearchSetUpTestTimeout + arcOptinTimeout,
		PreTestTimeout:  launcherSearchPreTestTimeout,
		PostTestTimeout: launcherSearchPostTestTimeout,
		Vars:            []string{"ui.gaiaPoolDefault"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: NormalLauncherSearchWithOOBE,
		Desc: "Normal launcher search with complete OOBE onboarding",
		Contacts: []string{
			"xiuwen@google.com",
			"ml-service-team@google.com",
		},
		Impl:            &launcherSearchFixtureImpl{featureFlags: []string{}, CompleteOOBE: true},
		SetUpTimeout:    launcherSearchSetUpTestTimeout + oobeTimeout,
		PreTestTimeout:  launcherSearchPreTestTimeout,
		PostTestTimeout: launcherSearchPostTestTimeout,
		Vars:            []string{"ui.gaiaPoolDefault"},
	})

}

func (f *launcherSearchFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var opts []chrome.Option
	opts = append(opts, chrome.EnableFeatures(f.featureFlags...))

	if f.CompleteOOBE {
		opts = append(opts, chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")))
		opts = append(opts, chrome.DontSkipOOBEAfterLogin())
		opts = append(opts, chrome.EnableFeatures("HelpAppLauncherSearch"))
	}

	if f.ARCSupported {
		opts = append(opts, chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")))
		opts = append(opts, chrome.ARCSupported())
		opts = append(opts, chrome.ExtraArgs(arc.DisableSyncFlags()...))
	}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}

	f.cr = cr

	f.tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection: ", err)
	}

	if f.CompleteOOBE {
		ui := uiauto.New(f.tconn)
		if err := oobe.CompleteOnboardingFlow(ctx, ui); err != nil {
			s.Fatal("Failed to go through the oobe flow: ", err)
		}
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	f.kb = kb

	if f.ARCSupported {
		if err = optin.PerformAndClose(ctx, cr, f.tconn); err != nil {
			s.Fatal("Failed to optin to Play Store and Close: ", err)
		}
	}

	return LauncherSearchFixtData{Chrome: f.cr, TestAPIConn: f.tconn, Keyboard: kb}
}

func (f *launcherSearchFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
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

func (f *launcherSearchFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// Do nothing if the recorder is not initialized.
	if f.recorder != nil {
		f.recorder.StopAndSaveOnError(ctx, filepath.Join(s.OutDir(), "record.webm"), s.HasError)
	}
}

func (f *launcherSearchFixtureImpl) Reset(ctx context.Context) error {
	return nil
}

func (f *launcherSearchFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome connection: ", err)
	}

	f.kb.Close(ctx)
}
