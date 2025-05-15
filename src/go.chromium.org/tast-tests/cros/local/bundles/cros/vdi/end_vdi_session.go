// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vdi

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast-tests/cros/local/vdi/fixtures"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type endVdiSessionData struct {
	DesktopName   string
	EndSessionCmd []string
	BackInVdiApp  []string
}

var citrixData = endVdiSessionData{
	DesktopName:   "COMMERCIAL VDI",
	EndSessionCmd: []string{"Disconnect"},
	BackInVdiApp:  []string{"Citrix", "Workspace"},
}

// TODO b/270322387: add the VmWare implementation details
// var vmwareData = endVdiSessionData{
// 	DesktopName:   "TD-RDS-DESKTOPS",
// 	DisconnectCmd: []string{"Disconnect"},
// 	BackInVdiApp:  []string{},
// }

func init() {
	testing.AddTest(&testing.Test{
		Func: EndVdiSession,
		Desc: "Test verifies the behaviour of ending a VDI session",
		Contacts: []string{
			"iwa-team@google.com",
			"cros-engprod-muc@google.com",
			"giovax@google.com", // Test author
		},
		BugComponent: "b:1198148", // ChromeOS > Software > Commercial (Enterprise) > App Platforms > Virtualization
		Attr:         []string{},
		SoftwareDeps: []string{"chrome"},
		Timeout:      10 * time.Minute,
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
				Name:      "kiosk_citrix",
				Fixture:   fixture.KioskCitrixLaunched,
				Val:       citrixData,
				ExtraAttr: []string{"group:vdi_limited"},
			},
			{
				Name:      "mgs_citrix",
				Fixture:   fixture.MgsCitrixLaunched,
				Val:       citrixData,
				ExtraAttr: []string{"group:vdi_limited"},
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

	if err := vdi.OpenApplication(ctx, appToOpen, checkRemoteDesktopStarted)(ctx); err != nil {
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

	// Cleanup after test by closing desktop.
	if err := vdi.CleanUpDesktop(ctx, kioskMode); err != nil {
		s.Fatal("Failed to close the desktop for the cleanup: ", err)
	}
}
