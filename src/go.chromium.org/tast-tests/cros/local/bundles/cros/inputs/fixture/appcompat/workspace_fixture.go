// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package appcompat defines fixtures for different application specific fixtures
package appcompat

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googledocs"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast/core/testing"
)

// fixture's name
const (
	GoogleDocsWithVK         = "googleDocsWithVK"
	GoogleSheetsWithVK       = "googleSheetsWithVK"
	GoogleSlidesWithVK       = "googleSlidesWithVK"
	GoogleDocsNonVK          = "googleDocsNoVK"
	GoogleSheetsNonVK        = "googleSheetsNoVK"
	GoogleSlidesNonVK        = "googleSlidesNoVK"
	LacrosGoogleDocsWithVK   = "lacrosGoogleDocsWithVK"
	LacrosGoogleSheetsWithVK = "lacrosGoogleSheetsWithVK"
	LacrosGoogleSlidesWithVK = "lacrosGoogleSlidesWithVK"
	LacrosGoogleDocsNonVK    = "lacrosGoogleDocsNoVK"
	LacrosGoogleSheetsNonVK  = "lacrosGoogleSheetsNoVK"
	LacrosGoogleSlidesNonVK  = "lacrosGoogleSlidesNoVK"
)

// app's name
const (
	googleDocs   = "Google Docs"
	googleSheets = "Google Sheets"
	googleSlides = "Google Slides"
)

const (
	workspaceSetUpTestTimeout = 5 * time.Second
	workspacePreTestTimeout   = time.Minute
	workspacePostTestTimeout  = time.Minute
)

// workSpaceFixtureImpl implements testing.FixtureImpl.
type workSpaceFixtureImpl struct {
	appName string // name of testing app
	tconn   *chrome.TestConn
	conn    *chrome.Conn
	cr      *chrome.Chrome
}

// WorkspaceFixtData is the data returned by SetUp and passed to tests.
type WorkspaceFixtData struct {
	Chrome             *chrome.Chrome
	TestAPIConn        *chrome.TestConn
	UserContext        *useractions.UserContext
	AppRootWebAreaName string
}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: GoogleDocsWithVK,
		Desc: "Open google docs for testing with VK enabled",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            &workSpaceFixtureImpl{appName: googleDocs},
		SetUpTimeout:    workspaceSetUpTestTimeout,
		PreTestTimeout:  workspacePreTestTimeout,
		PostTestTimeout: workspacePostTestTimeout,
		Parent:          fixture.AnyVKInGAIA,
	})
	testing.AddFixture(&testing.Fixture{
		Name: GoogleDocsNonVK,
		Desc: "Open google docs for testing in any mode with VK disabled",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            &workSpaceFixtureImpl{appName: googleDocs},
		SetUpTimeout:    workspaceSetUpTestTimeout,
		PreTestTimeout:  workspacePreTestTimeout,
		PostTestTimeout: workspacePostTestTimeout,
		Parent:          fixture.ClamshellNonVKInGAIA,
	})
	testing.AddFixture(&testing.Fixture{
		Name: GoogleSlidesWithVK,
		Desc: "Open google slides for testing in any mode with VK enabled",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            &workSpaceFixtureImpl{appName: googleSlides},
		SetUpTimeout:    workspaceSetUpTestTimeout,
		PreTestTimeout:  workspacePreTestTimeout,
		PostTestTimeout: workspacePostTestTimeout,
		Parent:          fixture.AnyVKInGAIA,
	})
	testing.AddFixture(&testing.Fixture{
		Name: GoogleSlidesNonVK,
		Desc: "Open google slides for testing in any mode with VK disabled",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            &workSpaceFixtureImpl{appName: googleSlides},
		SetUpTimeout:    workspaceSetUpTestTimeout,
		PreTestTimeout:  workspacePreTestTimeout,
		PostTestTimeout: workspacePostTestTimeout,
		Parent:          fixture.ClamshellNonVKInGAIA,
	})
	testing.AddFixture(&testing.Fixture{
		Name: GoogleSheetsWithVK,
		Desc: "Open google sheet for testing in any mode with VK enabled",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            &workSpaceFixtureImpl{appName: googleSheets},
		SetUpTimeout:    workspaceSetUpTestTimeout,
		PreTestTimeout:  workspacePreTestTimeout,
		PostTestTimeout: workspacePostTestTimeout,
		Parent:          fixture.AnyVKInGAIA,
	})
	testing.AddFixture(&testing.Fixture{
		Name: GoogleSheetsNonVK,
		Desc: "Open google sheet for testing in any mode with VK disabled",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            &workSpaceFixtureImpl{appName: googleSheets},
		SetUpTimeout:    workspaceSetUpTestTimeout,
		PreTestTimeout:  workspacePreTestTimeout,
		PostTestTimeout: workspacePostTestTimeout,
		Parent:          fixture.ClamshellNonVKInGAIA,
	})

	//--------------Lacros Fixtures--------------------------------------------
	testing.AddFixture(&testing.Fixture{
		Name: LacrosGoogleDocsWithVK,
		Desc: "Lacros variant: Open google docs for testing in any mode with VK enabled",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            &workSpaceFixtureImpl{appName: googleDocs},
		SetUpTimeout:    workspaceSetUpTestTimeout,
		PreTestTimeout:  workspacePreTestTimeout,
		PostTestTimeout: workspacePostTestTimeout,
		Parent:          fixture.LacrosAnyVKInGAIA,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosGoogleDocsNonVK,
		Desc: "Lacros variant: Open google docs for testing in Clamshell mode with VK disabled",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            &workSpaceFixtureImpl{appName: googleDocs},
		SetUpTimeout:    workspaceSetUpTestTimeout,
		PreTestTimeout:  workspacePreTestTimeout,
		PostTestTimeout: workspacePostTestTimeout,
		Parent:          fixture.LacrosClamshellNonVKInGAIA,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosGoogleSlidesWithVK,
		Desc: "Lacros variant: Open google slides for testing in any mode with VK enabled",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            &workSpaceFixtureImpl{appName: googleSlides},
		SetUpTimeout:    workspaceSetUpTestTimeout,
		PreTestTimeout:  workspacePreTestTimeout,
		PostTestTimeout: workspacePostTestTimeout,
		Parent:          fixture.LacrosAnyVKInGAIA,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosGoogleSlidesNonVK,
		Desc: "Lacros variant: Open google slides for testing in Clamshell mode with VK disabled",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            &workSpaceFixtureImpl{appName: googleSlides},
		SetUpTimeout:    workspaceSetUpTestTimeout,
		PreTestTimeout:  workspacePreTestTimeout,
		PostTestTimeout: workspacePostTestTimeout,
		Parent:          fixture.LacrosClamshellNonVKInGAIA,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosGoogleSheetsWithVK,
		Desc: "Lacros variant: Open google sheet for testing in any mode with VK enabled",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            &workSpaceFixtureImpl{appName: googleSheets},
		SetUpTimeout:    workspaceSetUpTestTimeout,
		PreTestTimeout:  workspacePreTestTimeout,
		PostTestTimeout: workspacePostTestTimeout,
		Parent:          fixture.LacrosAnyVKInGAIA,
	})
	testing.AddFixture(&testing.Fixture{
		Name: LacrosGoogleSheetsNonVK,
		Desc: "Lacros variant: Open google sheet for testing in Clamshell mode with VK disabled",
		Contacts: []string{
			"essential-inputs-team@google.com",
			"xiuwen@google.com",
		},
		Impl:            &workSpaceFixtureImpl{appName: googleSheets},
		SetUpTimeout:    workspaceSetUpTestTimeout,
		PreTestTimeout:  workspacePreTestTimeout,
		PostTestTimeout: workspacePostTestTimeout,
		Parent:          fixture.LacrosClamshellNonVKInGAIA,
	})

}

func (f *workSpaceFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cr := s.ParentValue().(fixture.FixtData).Chrome
	tconn := s.ParentValue().(fixture.FixtData).TestAPIConn
	uc := s.ParentValue().(fixture.FixtData).UserContext

	f.cr = cr
	f.tconn = tconn
	appRootWebAreaName := f.appName

	return WorkspaceFixtData{f.cr, f.tconn, uc, appRootWebAreaName}
}

func (f *workSpaceFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	var conn *chrome.Conn
	var err error

	switch f.appName {
	case googleDocs:
		conn, err = f.cr.NewConn(ctx, cuj.NewGoogleDocsURL)
	case googleSheets:
		conn, err = f.cr.NewConn(ctx, cuj.NewGoogleSheetsURL)
	case googleSlides:
		conn, err = f.cr.NewConn(ctx, cuj.NewGoogleSlidesURL)
	}

	if err != nil {
		s.Fatal(fmt.Sprintf("Failed to open %s: ", f.appName), err)
	}
	f.conn = conn

	if err := webutil.WaitForQuiescence(ctx, conn, workspacePreTestTimeout); err != nil {
		s.Fatal("Failed to wait for page to finish loading: ", err)
	}
	if err := cuj.MaximizeBrowserWindow(ctx, f.tconn, true, f.appName); err != nil {
		s.Fatal(fmt.Sprintf("Failed to maximize the %s page: ", f.appName), err)
	}

	// google slide has to make title field or text field editable first
	// otherwise, user cannot typing anything.
	if f.appName == googleSlides {
		if err := googledocs.ActivateTitleField(f.tconn)(ctx); err != nil {
			s.Fatal("Failed to activate slides title field: ", err)
		}
	}
}

func (f *workSpaceFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	var err error

	switch f.appName {
	case googleDocs:
		err = googledocs.DeleteDoc(f.tconn)(ctx)
	case googleSheets:
		err = googledocs.DeleteSheets(f.tconn)(ctx)
	case googleSlides:
		err = googledocs.DeleteSlide(f.tconn)(ctx)
	}

	if err != nil {
		s.Errorf("Fail to close %s with error: %s", f.appName, err)
	}

	f.conn.Close()
}

func (f *workSpaceFixtureImpl) Reset(ctx context.Context) error {
	return nil
}

func (f *workSpaceFixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {}
