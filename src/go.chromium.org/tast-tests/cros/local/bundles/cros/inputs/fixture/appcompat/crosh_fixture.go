// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package appcompat

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/inputactions"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast/core/testing"
)

// Crosh is a fixture for open the crosh terminal.
const Crosh = "crosh"

// croshFixtureImpl implements testing.FixtureImpl.
type croshFixtureImpl struct {
	tconn      *chrome.TestConn
	conn       *chrome.Conn
	cr         *chrome.Chrome
	kb         *input.KeyboardEventWriter
	uc         *useractions.UserContext
	uidetector *uidetection.Context
	recorder   *uiauto.ScreenRecorder
	ui         *uiauto.Context
}

// CroshFixtData is the data returned by SetUp and passed to tests.
type CroshFixtData struct {
	Chrome      *chrome.Chrome
	TestAPIConn *chrome.TestConn
	UserContext *useractions.UserContext
	Keyboard    *input.KeyboardEventWriter
	UIDetector  *uidetection.Context
}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: Crosh,
		Desc: "Open crosh",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            &croshFixtureImpl{},
		SetUpTimeout:    time.Minute,
		PreTestTimeout:  time.Minute,
		PostTestTimeout: time.Minute,
	})
}

func (f *croshFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cr, err := chrome.New(ctx)

	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	f.tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection: ", err)
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
	f.ui = uiauto.New(f.tconn)

	return CroshFixtData{f.cr, f.tconn, f.uc, f.kb, f.uidetector}
}

func (f *croshFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	f.recorder = uiauto.CreateAndStartScreenRecorder(ctx, f.tconn)

	if err := uiauto.Combine("open crosh via typing hot key",
		f.kb.AccelAction("Ctrl+Alt+t"),
		f.ui.WaitUntilExists(nodewith.Role(role.StaticText).Name("crosh").ClassName("Label")),
	)(ctx); err != nil {
		s.Fatal("Failed to start crosh: ", err)
	}
}

func (f *croshFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	closeCurrentTab := nodewith.Role(role.Button).Name("Close").ClassName("TabCloseButton")
	leaveAppButton := nodewith.Role(role.Button).Name("Leave").ClassName("MdTextButton")

	if err := uiauto.Combine("close crosh app",
		f.ui.DoDefault(closeCurrentTab),
		f.ui.WaitUntilExists(leaveAppButton),
		f.ui.DoDefault(leaveAppButton),
	)(ctx); err != nil {
		s.Log("Failed to close crosh: ", err)
	}

	// Do nothing if the recorder is not initialized.
	if f.recorder != nil {
		f.recorder.StopAndSaveOnError(ctx, filepath.Join(s.OutDir(), "record.webm"), s.HasError)
	}
}

func (f *croshFixtureImpl) Reset(ctx context.Context) error {
	return nil
}

func (f *croshFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	f.kb.Close(ctx)
}
