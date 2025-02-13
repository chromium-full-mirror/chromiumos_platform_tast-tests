// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vdi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast-tests/cros/local/vdi/fixtures"
	"go.chromium.org/tast/core/testing"
)

type desktopData struct {
	DesktopName   string
	RunDialogKeys string
}

func init() {
	testing.AddTest(&testing.Test{
		Func: OpenDesktop,
		Desc: "Test opens Desktop in VDI sessions in user session, Kiosk and MGS",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"kamilszarek@google.com", // Test author
		},
		BugComponent: "b:1170223", // ChromeOS > Software > Commercial (Enterprise) > EngProd
		// TODO(crbug.com/1293793): Add cleanup for kiosk and add its params.
		Attr:         []string{},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Launch Citrix or VMWare virtual desktop: COM_VDI_CUJ7_TASK1_WF1.
			Value: "screenplay-e4fd3e2c-eb81-4feb-8cca-4a30962136fb",
		}},
		Requirements: []string{"screenplay-e4fd3e2c-eb81-4feb-8cca-4a30962136fb"},
		Params: []testing.Param{
			{
				Name:    "citrix",
				Fixture: fixture.CitrixLaunched,
				Val: desktopData{
					RunDialogKeys: "Search+R",
				},
				ExtraAttr: []string{"group:vdi_limited"},
			},
			{
				Name:    "mgs_citrix",
				Fixture: fixture.MgsCitrixLaunched,
				Val: desktopData{
					RunDialogKeys: "Search+R",
				},
				ExtraAttr: []string{"group:vdi_limited"},
			},
			{
				Name:    "kiosk_citrix",
				Fixture: fixture.KioskCitrixLaunched,
				Val: desktopData{
					DesktopName:   "COMMERCIAL VDI",
					RunDialogKeys: "Search+R",
				},
				ExtraAttr: []string{"group:vdi_limited"},
			},
			// TODO(b/396331887): Add VMware fixture to VDI tests when the infra is ready.

		},
		Data: []string{"toolbar_buttons_icon.png"},
	})
}

func OpenDesktop(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	vdi := s.FixtValue().(fixtures.HasVDIConnector).VDIConnector()
	uidetector := s.FixtValue().(fixtures.HasUIDetector).UIDetector()
	kioskMode := s.FixtValue().(fixtures.IsInKioskMode).InKioskMode()

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	ui := uiauto.New(tconn)

	keysToOpenRunDialog := s.Param().(desktopData).RunDialogKeys

	if err := vdi.OpenDesktop(ctx); err != nil {
		s.Fatal("Failed to open the desktop: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get a keyboard")
	}
	defer kb.Close(ctx)

	// Move focus on Windows desktop.
	if err := kb.Accel(ctx, "Tab"); err != nil {
		s.Fatal("Failed to execute Tab command: ", err)
	}

	// Invoke opening the run dialog box.
	if err := ui.RetryUntil(
		kb.AccelAction(keysToOpenRunDialog),
		uidetector.WithTimeout(30*time.Second).WaitUntilExists(uidetection.Word("Run").First()),
	)(ctx); err != nil {
		s.Error("Failed to invoke opening the run dialog box: ", err)
	}

	// Cleanup after test by closing desktop.
	if err := vdi.CleanUpDesktop(ctx, kioskMode); err != nil {
		s.Fatal("Failed to close the desktop for the cleanup: ", err)
	}
}
