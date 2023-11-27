// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package appcompat defines fixtures for different application specific fixtures
package appcompat

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/inputs/inputactions"
	"go.chromium.org/tast-tests/cros/local/inputs/util"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ime"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast-tests/cros/local/vdi/apps"
	"go.chromium.org/tast-tests/cros/local/vdi/fixtures"
	"go.chromium.org/tast/core/testing"
)

const (
	citrixNotepadSetUpTestTimeout = 2 * time.Minute
	citrixNotepadPreTestTimeout   = 2 * time.Minute
	citrixNotepadPostTestTimeout  = time.Minute
)

const (
	appName                   = "Notepad"
	textToLookForWhenLaunched = "Untitled - Notepad"
	closeButtonText           = "Close"
	doNotSaveText             = "Don't Save"
)

// citrixNotepadFixtureImpl implements testing.FixtureImpl.
type citrixNotepadFixtureImpl struct {
	tconn      *chrome.TestConn
	uc         *useractions.UserContext
	kb         *input.KeyboardEventWriter
	uidetector *uidetection.Context
	vdi        apps.VDIInt
	imeID      string
	recorder   *uiauto.ScreenRecorder
}

// CitrixNotepadFixtData is the data returned by SetUp and passed to tests.
type CitrixNotepadFixtData struct {
	UserContext *useractions.UserContext
	UIDetector  *uidetection.Context
	Keyboard    *input.KeyboardEventWriter
}

// CitrixNotepad is a fixture for open the remote notepad on citrix workspace.
const CitrixNotepad = "citrixNotepad"

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: CitrixNotepad,
		Desc: "Open notepad app in cirtirx",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            &citrixNotepadFixtureImpl{},
		SetUpTimeout:    citrixNotepadSetUpTestTimeout,
		PreTestTimeout:  citrixNotepadPreTestTimeout,
		PostTestTimeout: citrixNotepadPostTestTimeout,
		Parent:          fixture.CitrixLaunched,
	})
}

func (f *citrixNotepadFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cr := s.ParentValue().(chrome.HasChrome).Chrome()
	vdi := s.ParentValue().(fixtures.HasVDIConnector).VDIConnector()
	uidetector := s.ParentValue().(fixtures.HasUIDetector).UIDetector()

	f.uidetector = uidetector.WithScreenshotResizing()
	f.vdi = vdi

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	f.tconn = tconn

	// Get keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	f.kb = kb

	imeID, err := ime.CurrentInputMethod(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the current ime: ", err)
	}
	f.imeID = imeID

	uc, err := inputactions.NewInputsUserContextWithoutState(ctx, "", s.OutDir(), cr, f.tconn, nil)
	if err != nil {
		s.Fatal("Failed to create new inputs user context: ", err)
	}
	f.uc = uc
	return CitrixNotepadFixtData{f.uc, f.uidetector, f.kb}
}

func (f *citrixNotepadFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	f.recorder = uiauto.CreateAndStartScreenRecorder(ctx, f.tconn)
	util.OpenRemoteApplicationInCitirx(ctx, s, f.tconn, f.vdi, f.uidetector, appName, textToLookForWhenLaunched)
}

func (f *citrixNotepadFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	closeMenuItem := uidetection.Word(closeButtonText).First()
	appTitleBar := uidetection.TextBlock(strings.Split(textToLookForWhenLaunched, " ")).First()
	doNotSaveButton := uidetection.TextBlock(strings.Split(doNotSaveText, " ")).First()

	ui := uiauto.New(f.tconn)
	if err := ui.RetryUntil(
		uiauto.Combine("Close remote notepad without saving",
			f.uidetector.RightClick(appTitleBar),
			f.uidetector.WithTimeout(10*time.Second).WaitUntilExists(closeMenuItem),
			f.uidetector.LeftClick(closeMenuItem),
			f.uidetector.WithTimeout(10*time.Second).WaitUntilExists(doNotSaveButton),
			f.uidetector.LeftClick(doNotSaveButton),
		),
		f.uidetector.WaitUntilGone(appTitleBar),
	)(ctx); err != nil {
		s.Fatal("Failed to close the notepad without saving: ", err)
	}

	//Reset to default ime, otherwise the search application text may not right.
	if err := ime.SetCurrentInputMethod(ctx, f.tconn, f.imeID); err != nil {
		s.Error("Failed to set the default input method: ", err)
	}

	// Do nothing if the recorder is not initialized.
	if f.recorder != nil {
		f.recorder.StopAndSaveOnError(ctx, filepath.Join(s.OutDir(), "record.webm"), s.HasError)
	}
}

func (f *citrixNotepadFixtureImpl) Reset(ctx context.Context) error {
	return nil
}

func (f *citrixNotepadFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	f.kb.Close(ctx)
}
