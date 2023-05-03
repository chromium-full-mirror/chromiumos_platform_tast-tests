// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vdi

import (
	"context"
	"strings"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/uidetection"
	"chromiumos/tast/local/vdi/fixtures"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type endVdiSessionData struct {
	DesktopName   string
	EndSessionCmd []string
	BackInVdiApp  []string
}

var citrixData = endVdiSessionData{
	DesktopName:   "WindowsServer2019",
	EndSessionCmd: []string{"Disconnect"},
	BackInVdiApp:  []string{"See", "more", "results"},
}

// TODO b/270322387: add the VmWare implementation details
// var vmwareData = endVdiSessionData{
// 	DesktopName:   "TD-RDS-DESKTOPS",
// 	DisconnectCmd: []string{"Disconnect"},
// 	BackInVdiApp:  []string{},
// }

func init() {
	testing.AddTest(&testing.Test{
		Func:         EndVdiSession,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test verifies the behaviour of ending a VDI session",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"giovax@google.com", // Test author
		},
		BugComponent: "b:1198148", // ChromeOS > Software > Commercial (Enterprise) > App Platforms > Virtualization
		Attr:         []string{},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-137bd441-64ae-4eaf-9eb0-a6e0e1fdb8d0", // COM_VDI_CUJ7_TASK10_WF1
		}},
		Requirements: []string{"screenplay-137bd441-64ae-4eaf-9eb0-a6e0e1fdb8d0"},
		Params: []testing.Param{
			{
				Name:      "citrix",
				Fixture:   fixture.CitrixLaunched,
				Val:       citrixData,
				ExtraAttr: []string{"group:vdi_limited"},
			},
			{
				Name:              "lacros_citrix",
				Fixture:           fixture.LacrosCitrixLaunched,
				Val:               citrixData,
				ExtraAttr:         []string{"group:vdi_limited"},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:      "kiosk_citrix",
				Fixture:   fixture.KioskCitrixLaunched,
				Val:       citrixData,
				ExtraAttr: []string{"group:vdi_limited"},
			},
			{
				Name:              "kiosk_lacros_citrix",
				Fixture:           fixture.KioskLacrosCitrixLaunched,
				Val:               citrixData,
				ExtraAttr:         []string{"group:vdi_limited"},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name:      "mgs_citrix",
				Fixture:   fixture.MgsCitrixLaunched,
				Val:       citrixData,
				ExtraAttr: []string{"group:vdi_limited"},
			},
			{
				Name:              "mgs_lacros_citrix",
				Fixture:           fixture.MgsLacrosCitrixLaunched,
				Val:               citrixData,
				ExtraAttr:         []string{"group:vdi_limited"},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			// TODO(b/270322387) add VMware implementations
		},
		Data: []string{"toolbar_buttons_icon.png", "Start_btn.png", "Power_btn.png"},
	})
}

func EndVdiSession(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	vdi := s.FixtValue().(fixtures.HasVDIConnector).VDIConnector()
	kioskMode := s.FixtValue().(fixtures.IsInKioskMode).InKioskMode()
	uidetector := s.FixtValue().(fixtures.HasUIDetector).UIDetector()
	data := s.Param().(endVdiSessionData)

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree")

	appToOpen := data.DesktopName

	checkRemoteDesktopStarted := func(ctx context.Context) error {
		if !kioskMode {
			// Wait for actual application window to open.
			if err := ash.WaitForCondition(ctx, tconn,
				func(w *ash.Window) bool {
					return strings.Contains(w.Title, appToOpen)
				},
				&testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
				s.Fatalf("Failed to find %s window: %v", appToOpen, err)
			}
		}

		// Wait for the Recycle Bin.
		recycleBin := uidetection.TextBlock([]string{"Recycle", "Bin"})
		if err := uidetector.WithTimeout(60 * time.Second).WaitUntilExists(recycleBin)(ctx); err != nil {
			return errors.Wrap(err, "failed waiting for the recycle bin to appear")
		}

		return nil
	}

	if err := vdi.SearchAndOpenApplication(ctx, appToOpen, checkRemoteDesktopStarted)(ctx); err != nil {
		s.Fatalf("Failed to open %v app: %v", appToOpen, err)
	}

	startBtn := uidetection.CustomIcon(s.DataPath("Start_btn.png"))
	powerBtn := uidetection.CustomIcon(s.DataPath("Power_btn.png"))
	disconnectCmd := uidetection.TextBlock(data.EndSessionCmd)

	if err := uiauto.Combine("Disconnect the VDI session",
		uidetector.WithTimeout(30*time.Second).WaitUntilExists(startBtn),
		uidetector.LeftClick(startBtn),
		uidetector.WithTimeout(30*time.Second).WaitUntilExists(powerBtn),
		uidetector.LeftClick(powerBtn),
		uidetector.WithTimeout(30*time.Second).WaitUntilExists(disconnectCmd),
		uidetector.LeftClick(disconnectCmd),
		uidetector.WithTimeout(30*time.Second).WaitUntilExists(uidetection.TextBlock(data.BackInVdiApp)),
	)(ctx); err != nil {
		s.Fatal("Failed to log out: ", err)
	}
}
