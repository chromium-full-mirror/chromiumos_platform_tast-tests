// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package spera

import (
	"context"
	"time"

	"chromiumos/tast/local/bundles/cros/spera/conference"
	"chromiumos/tast/local/bundles/cros/spera/conference/zoomserver"
	"chromiumos/tast/local/bundles/cros/spera/enterprisecuj"
	cx "chromiumos/tast/local/bundles/cros/spera/enterprisecuj/citrix"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/display"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TelemedicineCUJ,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measure the performance of simulated telemedicine operations on the Citrix Workspace client app",
		Contacts:     []string{"chromeos-perf-reliability-eng@google.com", "cienet-development@googlegroups.com", "chicheny@google.com"},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > SPERA > Automation
		Vars: []string{
			// Optional. Expecting "tablet" or "clamshell". Other values will be be taken as "clamshell".
			"spera.cuj_mode",
			// Required. Credentials used to login Citrix.
			"spera.citrix_url",
			"spera.citrix_username",
			"spera.citrix_password",
			"spera.citrix_desktopname",
			// Required. Used for UI detection API.
			"uidetection.key_type",
			"uidetection.key",
			"uidetection.server",
			// Required. Zoom meet bot server address.
			"spera.zoom_bot_server",
			"spera.zoom_bot_token",
			// Required. Credentials used by Citrix to log in to zoom.
			"spera.citrix_zoom_account",
			"spera.citrix_zoom_password",
		},
		Params: []testing.Param{
			{
				Name:    "basic",
				Fixture: "enrolledLoggedInToCUJUser",
				Timeout: 15 * time.Minute,
				Val:     cx.NormalMode,
			},
			{
				// basic_record is a subcase for recording telemedicine CUJ.
				// When executed, it will record the coordinates and waiting time of all pictures and text
				// detected by uidetection, and will read these data in replay mode.
				Name:    "basic_record",
				Fixture: "enrolledLoggedInToCUJUser",
				Timeout: 15 * time.Minute,
				Val:     cx.RecordMode,
			},
			{
				// basic_replay is a subcase for replaying telemedicine CUJ.
				// When executed, the coordinates and waiting time of the picture/text recorded in the record
				// mode will be loaded. Use this coordinate data to perform ui click, and reduce this waiting
				// time data to wait for ui. This can greatly reduce the execution time of the case
				Name:    "basic_replay",
				Fixture: "enrolledLoggedInToCUJUser",
				Timeout: 15 * time.Minute,
				Val:     cx.ReplayMode,
			},
		},
		Data: enterprisecuj.TelemedicineData,
	})
}

func TelemedicineCUJ(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	var tabletMode bool
	if mode, ok := s.Var("spera.cuj_mode"); ok {
		tabletMode = mode == "tablet"
		cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, tabletMode)
		if err != nil {
			s.Fatalf("Failed to enable tablet mode to %v: %v", tabletMode, err)
		}
		defer cleanup(cleanupCtx)
	} else {
		// Use default screen mode of the DUT.
		tabletMode, err = ash.TabletModeEnabled(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to get DUT default screen mode: ", err)
		}
	}
	s.Log("Running test with tablet mode: ", tabletMode)
	var uiHandler cuj.UIActionHandler
	if tabletMode {
		cleanup, err := display.RotateToLandscape(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to rotate display to landscape: ", err)
		}
		defer cleanup(cleanupCtx)
		if uiHandler, err = cuj.NewTabletActionHandler(ctx, tconn); err != nil {
			s.Fatal("Failed to create tablet action handler: ", err)
		}
	} else {
		if uiHandler, err = cuj.NewClamshellActionHandler(ctx, tconn); err != nil {
			s.Fatal("Failed to create clamshell action handler: ", err)
		}
	}

	host := s.RequiredVar("spera.zoom_bot_server")
	sessionToken := s.RequiredVar("spera.zoom_bot_token")
	citrixZoomAccount := s.RequiredVar("spera.citrix_zoom_account")
	citrixZoomPassword := s.RequiredVar("spera.citrix_zoom_password")

	cleanUpRoomCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	roomType := conference.TwoRoomSize
	roomSize := conference.ZoomRoomParticipants[roomType] - 1
	room, cleanup, err := zoomserver.CreateConference(ctx, roomSize, sessionToken, host)
	if err != nil {
		s.Fatal("Failed to create conference room: ", err)
	}
	defer cleanup(cleanUpRoomCtx)

	scenario := enterprisecuj.NewTelemedicineScenario(room, citrixZoomAccount, citrixZoomPassword)
	params := &enterprisecuj.TestParams{
		OutDir:          s.OutDir(),
		CitrixServerURL: s.RequiredVar("spera.citrix_url"),
		CitrixUserName:  s.RequiredVar("spera.citrix_username"),
		CitrixPassword:  s.RequiredVar("spera.citrix_password"),
		DesktopName:     s.RequiredVar("spera.citrix_desktopname"),
		TabletMode:      tabletMode,
		TestMode:        s.Param().(cx.TestMode),
		DataPath:        s.DataPath,
		UIHandler:       uiHandler,
	}
	if err := enterprisecuj.Run(ctx, cr, scenario, params); err != nil {
		s.Fatal("Failed to run the telemedicine cuj: ", err)
	}
}
