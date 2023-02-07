// Copyright 2022 The ChromiumOS Authors
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
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/uidetection"
	"chromiumos/tast/local/vdi/fixtures"
	"chromiumos/tast/testing"
)

type desktopData struct {
	DesktopName   string
	RunDialogKeys string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         OpenDesktop,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Test opens Desktop in VDI sessions in user session, Kiosk and MGS",
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
					DesktopName:   "WindowsServer2019",
					RunDialogKeys: "Search+R",
				},
				ExtraAttr: []string{"group:vdi_limited"},
			},
			{
				Name:    "vmware",
				Fixture: fixture.VmwareLaunched,
				Val: desktopData{
					DesktopName:   "TD-RDS-DESKTOPS",
					RunDialogKeys: "Ctrl+Search+R",
				},
			},
			{
				Name:    "mgs_citrix",
				Fixture: fixture.MgsCitrixLaunched,
				Val: desktopData{
					DesktopName:   "WindowsServer2019",
					RunDialogKeys: "Search+R",
				},
				ExtraAttr: []string{"group:vdi_limited"},
			},
			{
				Name:    "mgs_vmware",
				Fixture: fixture.MgsVmwareLaunched,
				Val: desktopData{
					DesktopName:   "TD-RDS-DESKTOPS",
					RunDialogKeys: "Ctrl+Search+R",
				},
			},
		},
		Data: []string{"toolbar_buttons_icon.png"},
	})
}

func OpenDesktop(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	vdi := s.FixtValue().(fixtures.HasVDIConnector).VDIConnector()
	uidetector := s.FixtValue().(fixtures.HasUIDetector).UIDetector()

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	ui := uiauto.New(tconn)

	isOpened := func(ctx context.Context) error {
		recycleBin := uidetection.TextBlock([]string{"Recycle", "Bin"})
		// Find the Recycle Bin first.
		if err := uidetector.WithTimeout(60 * time.Second).WaitUntilExists(recycleBin)(ctx); err != nil {
			return errors.Wrap(err, "failed waiting for the recycle bin to appear")
		}
		if err := uidetector.WithTimeout(10 * time.Second).WaitUntilExists(uidetection.CustomIcon(s.DataPath("toolbar_buttons_icon.png")))(ctx); err != nil {
			return errors.Wrap(err, "failed waiting for the toolbar buttons icon to appear")
		}

		return nil
	}

	desktopToOpen := s.Param().(desktopData).DesktopName
	keysToOpenRunDialog := s.Param().(desktopData).RunDialogKeys

	if err := vdi.SearchAndOpenApplication(ctx, desktopToOpen, isOpened)(ctx); err != nil {
		s.Fatalf("Failed to open %v app: %v", desktopToOpen, err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get a keyboard")
	}
	defer kb.Close()

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
}
