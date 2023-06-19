// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture defines fixtures for launcher search tests.
package fixture

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
)

const (
	imageSearchSetUpTestTimeout = 40 * time.Second
	imageSearchPreTestTimeout   = 10 * time.Second
	imageSearchPostTestTimeout  = 10 * time.Second
)

// fixture's name
const (
	LauncherImageSearchIcaAndOcr = "launcherImageSearchIcaAndOcr"
	LauncherImageSearchOcr       = "launcherImageSearchOcr"
	LauncherImageSearchIca       = "launcherImageSearchIca"
	LauncherImageSearch          = "launcherImageSearch"
)

// imageSearchFixtureImpl implements testing.FixtureImpl.
type imageSearchFixtureImpl struct {
	featureFlags []string // Feature flags for testing.
	tconn        *chrome.TestConn
	cr           *chrome.Chrome
	kb           *input.KeyboardEventWriter
}

// ImageSearchFixtData is the data returned by SetUp and passed to tests.
type ImageSearchFixtData struct {
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
		Impl:            &imageSearchFixtureImpl{featureFlags: []string{"ProductivityLauncherImageSearch", "LauncherImageSearch", "LauncherImageSearchOcr", "LauncherImageSearchIca"}},
		SetUpTimeout:    imageSearchSetUpTestTimeout,
		PreTestTimeout:  imageSearchPreTestTimeout,
		PostTestTimeout: imageSearchPostTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LauncherImageSearchOcr,
		Desc: "Turn on LauncherImageSearchOcr",
		Contacts: []string{
			"xiuwen@google.com",
			"ml-service-team@google.com",
		},
		Impl:            &imageSearchFixtureImpl{featureFlags: []string{"ProductivityLauncherImageSearch", "LauncherImageSearch", "LauncherImageSearchOcr"}},
		SetUpTimeout:    imageSearchSetUpTestTimeout,
		PreTestTimeout:  imageSearchPreTestTimeout,
		PostTestTimeout: imageSearchPostTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LauncherImageSearchIca,
		Desc: "Turn on LauncherImageSearchIca",
		Contacts: []string{
			"xiuwen@google.com",
			"ml-service-team@google.com",
		},
		Impl:            &imageSearchFixtureImpl{featureFlags: []string{"ProductivityLauncherImageSearch", "LauncherImageSearch", "LauncherImageSearchIca"}},
		SetUpTimeout:    imageSearchSetUpTestTimeout,
		PreTestTimeout:  imageSearchPreTestTimeout,
		PostTestTimeout: imageSearchPostTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LauncherImageSearch,
		Desc: "Turn on ProductivityLauncherImageSearch and LauncherImageSearch",
		Contacts: []string{
			"xiuwen@google.com",
			"ml-service-team@google.com",
		},
		Impl:            &imageSearchFixtureImpl{featureFlags: []string{"ProductivityLauncherImageSearch", "LauncherImageSearch"}},
		SetUpTimeout:    imageSearchSetUpTestTimeout,
		PreTestTimeout:  imageSearchPreTestTimeout,
		PostTestTimeout: imageSearchPostTestTimeout,
	})

}

func (f *imageSearchFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cr, err := chrome.New(ctx, chrome.EnableFeatures(f.featureFlags...))
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

	return ImageSearchFixtData{TestAPIConn: f.tconn, Keyboard: kb}
}

func (f *imageSearchFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {

}

func (f *imageSearchFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {

}

func (f *imageSearchFixtureImpl) Reset(ctx context.Context) error {
	return nil
}

func (f *imageSearchFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome connection: ", err)
	}

	f.kb.Close(ctx)
}
