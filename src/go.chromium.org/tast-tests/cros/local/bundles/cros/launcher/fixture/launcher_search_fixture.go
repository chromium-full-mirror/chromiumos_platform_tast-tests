// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture defines fixtures for launcher search tests.
package fixture

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
)

const (
	launcherSearchSetUpTestTimeout = 40 * time.Second
	launcherSearchPreTestTimeout   = 10 * time.Second
	launcherSearchPostTestTimeout  = 10 * time.Second
)

// fixture's name
const (
	LauncherImageSearchIcaAndOcr = "launcherImageSearchIcaAndOcr"
	LauncherImageSearchOcr       = "launcherImageSearchOcr"
	LauncherImageSearchIca       = "launcherImageSearchIca"
	LauncherImageSearch          = "launcherImageSearch"
	NormalLauncherSearch         = "normalLauncherSearch"
)

// launcherSearchFixtureImpl implements testing.FixtureImpl.
type launcherSearchFixtureImpl struct {
	featureFlags []string // Feature flags for testing.
	tconn        *chrome.TestConn
	cr           *chrome.Chrome
	kb           *input.KeyboardEventWriter
	recorder     *uiauto.ScreenRecorder
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
		Impl:            &launcherSearchFixtureImpl{featureFlags: []string{}},
		SetUpTimeout:    launcherSearchSetUpTestTimeout,
		PreTestTimeout:  launcherSearchPreTestTimeout,
		PostTestTimeout: launcherSearchPostTestTimeout,
	})

}

func (f *launcherSearchFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var opts []chrome.Option
	opts = append(opts, chrome.EnableFeatures(f.featureFlags...))

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}

	f.cr = cr

	f.tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	f.kb = kb

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
