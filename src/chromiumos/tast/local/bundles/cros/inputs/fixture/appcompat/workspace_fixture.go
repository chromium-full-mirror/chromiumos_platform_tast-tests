// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package appcompat defines fixtures for different application specific fixtures
package appcompat

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/local/bundles/cros/inputs/fixture"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/googleapps"
	"chromiumos/tast/local/chrome/useractions"
	"chromiumos/tast/local/chrome/webutil"
	"chromiumos/tast/testing"
)

// app's name
const (
	GoogleDocs   = "googleDocs"
	GoogleSheets = "googleSheets"
	GoogleSlides = "googleSlides"
)

const (
	workspaceSetUpTestTimeout = 5 * time.Second
	workspacePreTestTimeout   = 20 * time.Second
	workspacePostTestTimeout  = 20 * time.Second
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
		Name: GoogleDocs,
		Desc: "Open google docs for testing",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            &workSpaceFixtureImpl{appName: GoogleDocs},
		SetUpTimeout:    workspaceSetUpTestTimeout,
		PreTestTimeout:  workspacePreTestTimeout,
		PostTestTimeout: workspacePostTestTimeout,
		Parent:          fixture.AnyVKInGAIA,
	})
	testing.AddFixture(&testing.Fixture{
		Name: GoogleSlides,
		Desc: "Open google slides for testing",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            &workSpaceFixtureImpl{appName: GoogleSlides},
		SetUpTimeout:    workspaceSetUpTestTimeout,
		PreTestTimeout:  workspacePreTestTimeout,
		PostTestTimeout: workspacePostTestTimeout,
		Parent:          fixture.AnyVKInGAIA,
	})
	testing.AddFixture(&testing.Fixture{
		Name: GoogleSheets,
		Desc: "Open google sheet for testing",
		Contacts: []string{
			"xiuwen@google.com",
			"essential-inputs-team@google.com",
		},
		Impl:            &workSpaceFixtureImpl{appName: GoogleSheets},
		SetUpTimeout:    workspaceSetUpTestTimeout,
		PreTestTimeout:  workspacePreTestTimeout,
		PostTestTimeout: workspacePostTestTimeout,
		Parent:          fixture.AnyVKInGAIA,
	})
}

func (f *workSpaceFixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cr := s.ParentValue().(fixture.FixtData).Chrome
	tconn := s.ParentValue().(fixture.FixtData).TestAPIConn
	uc := s.ParentValue().(fixture.FixtData).UserContext

	f.cr = cr
	f.tconn = tconn

	var appRootWebAreaName string
	switch f.appName {
	case GoogleDocs:
		appRootWebAreaName = "Google Docs"
	case GoogleSheets:
		appRootWebAreaName = "Google Sheets"
	case GoogleSlides:
		appRootWebAreaName = "Google Slides"
	}

	return WorkspaceFixtData{f.cr, f.tconn, uc, appRootWebAreaName}
}

func (f *workSpaceFixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	var conn *chrome.Conn
	var err error

	switch f.appName {
	case GoogleDocs:
		conn, err = f.cr.NewConn(ctx, cuj.NewGoogleDocsURL)
	case GoogleSheets:
		conn, err = f.cr.NewConn(ctx, cuj.NewGoogleSheetsURL)
	case GoogleSlides:
		conn, err = f.cr.NewConn(ctx, cuj.NewGoogleSlidesURL)
	}

	if err != nil {
		s.Fatal(fmt.Sprintf("Failed to open %s: ", f.appName), err)
	}
	f.conn = conn

	if err := webutil.WaitForQuiescence(ctx, conn, time.Minute); err != nil {
		s.Fatal("Failed to wait for page to finish loading: ", err)
	}
	if err := cuj.MaximizeBrowserWindow(ctx, f.tconn, true, f.appName); err != nil {
		s.Fatal(fmt.Sprintf("Failed to maximize the %s page: ", f.appName), err)
	}

	// google slide has to make title field or text field editable first
	// otherwise, user cannot typing anything
	if f.appName == GoogleSlides {
		if err := googleapps.ActivateTitleField(f.tconn)(ctx); err != nil {
			s.Fatal("Failed to activate slides title field")
		}
	}
}

func (f *workSpaceFixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	var err error

	switch f.appName {
	case GoogleDocs:
		err = googleapps.DeleteDoc(f.tconn)(ctx)
	case GoogleSheets:
		err = googleapps.DeleteSheets(f.tconn)(ctx)
	case GoogleSlides:
		err = googleapps.DeleteSlide(f.tconn)(ctx)
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
