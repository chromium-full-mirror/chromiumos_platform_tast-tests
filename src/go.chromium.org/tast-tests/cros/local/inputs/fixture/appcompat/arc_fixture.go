// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package appcompat

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/inputs/inputactions"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast/core/testing"
)

// Fixture for optin playstore.
const (
	PlayStore       = "playStore"
	PlayStoreWithVK = "playStoreWithVK"
)

// arcFixtureImpl implements testing.FixtureImpl.
type arcFixtureImpl struct {
	tconn      *chrome.TestConn
	conn       *chrome.Conn
	cr         *chrome.Chrome
	kb         *input.KeyboardEventWriter
	uc         *useractions.UserContext
	uidetector *uidetection.Context
	recorder   *uiauto.ScreenRecorder
	vkEnabled  bool // Whether virtual keyboard is force enabled
}

// ArcFixtData is the data returned by SetUp and passed to tests.
type ArcFixtData struct {
	Chrome      *chrome.Chrome
	TestAPIConn *chrome.TestConn
	UserContext *useractions.UserContext
	Keyboard    *input.KeyboardEventWriter
	UIDetector  *uidetection.Context
}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: PlayStore,
		Desc: "Optin playstore",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            &arcFixtureImpl{},
		SetUpTimeout:    2 * time.Minute,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 2 * time.Minute,
		Vars:            []string{"ui.gaiaPoolDefault"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: PlayStoreWithVK,
		Desc: "Optin playstore with vk on",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            &arcFixtureImpl{vkEnabled: true},
		SetUpTimeout:    2 * time.Minute,
		PreTestTimeout:  4 * time.Minute,
		PostTestTimeout: 2 * time.Minute,
		Vars:            []string{"ui.gaiaPoolDefault"},
	})
}

func (f *arcFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var chromeOpts []chrome.Option

	chromeOpts = append(chromeOpts, chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")))
	chromeOpts = append(chromeOpts, chrome.ARCSupported())
	chromeOpts = append(chromeOpts, chrome.ExtraArgs(arc.DisableSyncFlags()...))

	if f.vkEnabled {
		chromeOpts = append(chromeOpts, chrome.VKEnabled())
	}

	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	f.tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection: ", err)
	}

	// Opt in to Play Store.
	if err = optin.PerformAndClose(ctx, cr, f.tconn); err != nil {
		s.Fatal("Failed to optin to Play Store and Close: ", err)
	}

	f.cr = cr

	// Get keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	f.kb = kb

	uc, err := inputactions.NewInputsUserContextWithoutState(ctx, "", s.OutDir(), cr, f.tconn, nil)
	if err != nil {
		s.Fatal("Failed to create new inputs user context: ", err)
	}
	f.uc = uc

	f.uidetector = uidetection.NewDefault(f.tconn).WithScreenshotStrategy(uidetection.ImmediateScreenshot)

	return ArcFixtData{f.cr, f.tconn, f.uc, f.kb, f.uidetector}
}

func (f *arcFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	f.recorder = uiauto.CreateAndStartScreenRecorder(ctx, f.tconn)

	if err := optin.LaunchAndWaitForPlayStore(ctx, f.tconn, f.cr, 2*time.Minute); err != nil {
		s.Fatal("Failed to launch Play Store: ", err)
	}
}

func (f *arcFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if err := optin.ClosePlayStore(ctx, f.tconn); err != nil {
		s.Fatal("Failed to close Play Store: ", err)
	}

	// Do nothing if the recorder is not initialized.
	if f.recorder != nil {
		f.recorder.StopAndSaveOnError(ctx, filepath.Join(s.OutDir(), "record.webm"), s.HasError)
	}
}

func (f *arcFixtureImpl) Reset(ctx context.Context) error {
	return nil
}

func (f *arcFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	f.kb.Close(ctx)
}
