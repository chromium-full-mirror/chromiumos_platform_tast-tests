// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package appcompat

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/inputs/data"
	"go.chromium.org/tast-tests/cros/local/inputs/inputactions"
	"go.chromium.org/tast-tests/cros/local/inputs/util"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast/core/testing"
)

// Fixture for open the fake e14s app.
const (
	InputsApp       = "inputsApp"
	InputsAppWithVK = "inputsAppWithVK"
	AppName         = "HelloGoogle3 Main"
	ApkName         = "e14stestapp.apk"
	ArcPackageName  = "google3.chrome.inputs.testing.e14stestapp"
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
		Name: InputsApp,
		Desc: "Open fake arc app",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		BugComponent:    "b:95887",
		Impl:            &arcFixtureImpl{},
		SetUpTimeout:    3 * time.Minute,
		PreTestTimeout:  1 * time.Minute,
		PostTestTimeout: 2 * time.Minute,
		Data: []string{
			ApkName,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: InputsAppWithVK,
		Desc: "Open fake arc app with vk on",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		BugComponent:    "b:95887",
		Impl:            &arcFixtureImpl{vkEnabled: true},
		SetUpTimeout:    3 * time.Minute,
		PreTestTimeout:  1 * time.Minute,
		PostTestTimeout: 2 * time.Minute,
		Data: []string{
			ApkName,
		},
	})
}

func (f *arcFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	var chromeOpts []chrome.Option
	chromeOpts = append(chromeOpts, chrome.ARCSupported())
	chromeOpts = append(chromeOpts, chrome.ExtraArgs(arc.DisableSyncFlags()...))
	chromeOpts = append(chromeOpts, chrome.ARCEnabled())

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

	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(ctx)

	// Install fake app and launch it.
	a.Install(ctx, s.DataPath(ApkName))

	ui := uiauto.New(f.tconn)
	homeButtonFinder := nodewith.Name("Launcher").Role(role.Button).Ancestor(nodewith.HasClass("ShelfContainer"))

	if err := uiauto.Combine("Search app and launch it",
		ui.DoDefault(homeButtonFinder),
		f.uidetector.LeftClick(uidetection.Word("HelloGoo")),
		ui.WaitUntilExists(nodewith.ClassName("Widget").Name(AppName)),
	)(ctx); err != nil {
		s.Fatal("Failed to start app: ", err)
	}

	return ArcFixtData{f.cr, f.tconn, f.uc, f.kb, f.uidetector}
}

func (f *arcFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	f.recorder = uiauto.CreateAndStartScreenRecorder(ctx, f.tconn)

	// clear the single line input field
	if err := f.uidetector.Tap(uidetection.Word("Single"))(ctx); err != nil {
		s.Fatal("Failed to trigger vk in playstore: ", err)
	}

	util.ClearTextFieldViaClickingBackspace(f.kb, data.LongestInputLength)(ctx)

}

func (f *arcFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
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
