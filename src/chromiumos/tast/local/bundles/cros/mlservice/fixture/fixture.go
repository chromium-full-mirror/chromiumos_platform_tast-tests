// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture defines fixtures for ML service tests.
package fixture

import (
	"context"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/local/dlc"
	"chromiumos/tast/testing"
)

const (
	resetTimeout    = 30 * time.Second
	preTestTimeout  = 10 * time.Second
	postTestTimeout = 15 * time.Second
)

// List of fixture names for ML service testing.
const (
	EffectsPipelineInstalled = "effectsPipelineInstalled"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: EffectsPipelineInstalled,
		Desc: "A fixture with Effects Pipeline DLC installed",
		Contacts: []string{
			"chromeos-platform-ml-accelerators@google.com",
			"shengjun@google.com",
			"shafron@google.com",
		},
		Impl:            mlFixture(browser.TypeAsh, false, []chrome.Option{chrome.NoLogin()}, []string{"ml-core-internal"}),
		SetUpTimeout:    chrome.LoginTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}

func mlFixture(browserType browser.Type, restart bool, chromeOpts []chrome.Option, dlcs []string) testing.FixtureImpl {
	return &mlFixtureImpl{
		restart:    restart,
		chromeOpts: chromeOpts,
		dlcs:       dlcs,
	}
}

// FixtData is the data returned by SetUp and passed to tests.
type FixtData struct {
	Chrome      *chrome.Chrome
	BrowserType browser.Type
}

// mlFixtureImpl implements testing.FixtureImpl.
type mlFixtureImpl struct {
	cr          *chrome.Chrome  // Underlying Chrome instance
	browserType browser.Type    // Whether Ash or Lacros is used for test
	restart     bool            // Whether restart the fixture after each test
	chromeOpts  []chrome.Option // Options that are passed to chrome.New
	dlcs        []string        // A list of dlcs that need to be installed in setup.
}

func (f *mlFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Start Chrome instance.
	cr, err := browserfixt.NewChrome(ctx, f.browserType, lacrosfixt.NewConfig(), f.chromeOpts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	f.cr = cr

	// Install required DLCs.
	for _, dlcID := range f.dlcs {
		if err := dlc.Install(ctx, dlcID, ""); err != nil {
			s.Fatalf("Failed to install DLC %q: %v", dlcID, err)
		}
	}

	return FixtData{f.cr, f.browserType}
}

func (f *mlFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *mlFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *mlFixtureImpl) Reset(ctx context.Context) error {
	return nil
}

func (f *mlFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome connection: ", err)
	}
	f.cr = nil
}
