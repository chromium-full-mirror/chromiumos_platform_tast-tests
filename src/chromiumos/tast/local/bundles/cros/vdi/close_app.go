// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vdi

import (
	"context"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/coords"
	"chromiumos/tast/local/uidetection"
	"chromiumos/tast/local/vdi/fixtures"
	"chromiumos/tast/testing"
)

type closeAppParams struct {
	AppName     string
	AppOpenText []string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CloseApp,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Test closes an application by two methods in VDI sessions in user session, Kiosk and MGS",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"kamilszarek@google.com", // VDI testing infrastructure owner
			"laurila@google.com",     // Test author
		},
		BugComponent: "b:1198148", // ChromeOS > Software > Commercial (Enterprise) > App Platforms > Virtualization
		Attr:         []string{},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Close apps launched from VDI app picker: COM_VDI_CUJ4_TASK6_WF1.
			Value: "screenplay-c4216bd8-b700-4ae0-bb5b-a319546c16d2",
		}},
		Requirements: []string{"screenplay-c4216bd8-b700-4ae0-bb5b-a319546c16d2"},
		Params: []testing.Param{
			{
				Name:      "citrix",
				Fixture:   fixture.CitrixLaunched,
				ExtraAttr: []string{"group:vdi_limited"},
				Val: closeAppParams{
					AppName:     "Notepad",
					AppOpenText: []string{"Notepad", "Untitled"},
				},
			},
			{
				Name:      "kiosk_citrix",
				Fixture:   fixture.KioskCitrixLaunched,
				ExtraAttr: []string{"group:vdi_limited"},
				Val: closeAppParams{
					AppName:     "Notepad",
					AppOpenText: []string{"Notepad", "Untitled"},
				},
			},
			{
				Name:      "mgs_citrix",
				Fixture:   fixture.MgsCitrixLaunched,
				ExtraAttr: []string{"group:vdi_limited"},
				Val: closeAppParams{
					AppName:     "Notepad",
					AppOpenText: []string{"Notepad", "Untitled"},
				},
			},
			// TODO(b/263380690): VMWare doesn't have as simple apps as notepad.
		},
		Data: []string{"notepad_window_controls.png"},
	})
}

func CloseApp(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	vdi := s.FixtValue().(fixtures.HasVDIConnector).VDIConnector()
	uidetector := s.FixtValue().(fixtures.HasUIDetector).UIDetector()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	ui := uiauto.New(tconn)

	appName := s.Param().(closeAppParams).AppName

	appTitleBar := uidetection.TextBlock(s.Param().(closeAppParams).AppOpenText).First()
	isOpened := func(ctx context.Context) error {
		if err := uidetector.WithTimeout(60 * time.Second).WaitUntilExists(appTitleBar)(ctx); err != nil {
			s.Fatal("fail: ", err)
		}
		return nil
	}

	// Open the app.
	if err := vdi.SearchAndOpenApplication(ctx, appName, isOpened)(ctx); err != nil {
		s.Fatalf("Failed to open %v app: %v", appName, err)
	}

	// Close app using right click + close.
	closeMenuItem := uidetection.Word("Close").First()
	if err := ui.RetryUntil(
		uiauto.Combine("close notepad",
			uidetector.RightClick(appTitleBar),
			uidetector.WithTimeout(10*time.Second).WaitUntilExists(closeMenuItem),
			uidetector.LeftClick(closeMenuItem),
		),
		uidetector.WithTimeout(20*time.Second).Gone(appTitleBar),
	)(ctx); err != nil {
		s.Fatal("Failed to close the app from right click menu: ", err)
	}

	// Reset the search to be able to open the app again.
	if err := vdi.ResetSearch(ctx); err != nil {
		s.Fatal("Was not able to reset search results: ", err)
	}

	// Re-open the app.
	if err := vdi.SearchAndOpenApplication(ctx, appName, isOpened)(ctx); err != nil {
		s.Fatalf("Failed to open %v app: %v", appName, err)
	}

	// Close app using the X in the corner.
	closeAppWithX := func(ctx context.Context) error {
		wco := uidetection.CustomIcon(s.DataPath("notepad_window_controls.png"), uidetection.MinConfidence(0.65))
		windowControlsLoc, err := uidetection.NewDefault(tconn).Location(ctx, wco)
		if err != nil {
			return errors.Wrap(err, "failed to find the location of the window controls")
		}
		if err := ui.MouseClickAtLocation( /*left click*/ 0, coords.Point{X: windowControlsLoc.RightCenter().X, Y: windowControlsLoc.RightCenter().Y})(ctx); err != nil {
			return errors.Wrap(err, "failed to close the app with the close button")
		}
		return nil
	}

	if err := ui.RetryUntil(
		closeAppWithX,
		uidetector.WithTimeout(20*time.Second).Gone(appTitleBar),
	)(ctx); err != nil {
		s.Fatal("Failed to close the app with the close button: ", err)
	}
}
