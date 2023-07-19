// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package appcompat

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/inputactions"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast/core/testing"
)

// PlayStore is a fixture for open the playstore.
const PlayStore = "playstore"

// arcFixtureImpl implements testing.FixtureImpl.
type arcFixtureImpl struct {
	tconn      *chrome.TestConn
	conn       *chrome.Conn
	cr         *chrome.Chrome
	kb         *input.KeyboardEventWriter
	uc         *useractions.UserContext
	uidetector *uidetection.Context
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
		Name: "playstore",
		Desc: "Optin playstore",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            &arcFixtureImpl{},
		SetUpTimeout:    2 * time.Minute,
		PreTestTimeout:  2 * time.Minute,
		PostTestTimeout: 2 * time.Minute,
		Vars:            []string{"ui.gaiaPoolDefault"},
	})
}

func (f *arcFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cr, err := chrome.New(ctx,
		chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
		chrome.ARCSupported(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...),
	)

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
	if err := optin.LaunchAndWaitForPlayStore(ctx, f.tconn, f.cr, time.Minute); err != nil {
		s.Fatal("Failed to launch Play Store: ", err)
	}
}

func (f *arcFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if err := optin.ClosePlayStore(ctx, f.tconn); err != nil {
		s.Fatal("Failed to close Play Store: ", err)
	}

}

func (f *arcFixtureImpl) Reset(ctx context.Context) error {
	return nil
}

func (f *arcFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	f.kb.Close(ctx)
}
