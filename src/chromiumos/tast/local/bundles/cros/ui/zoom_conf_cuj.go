// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/bundles/cros/ui/conference"
	"chromiumos/tast/local/bundles/cros/ui/conference/zoomserver"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/display"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/ui/cujrecorder"
	"chromiumos/tast/testing"
)

const (
	zoomSmallRoomTimeout = 15 * time.Minute
	zoomClassRoomTimeout = 20 * time.Minute
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ZoomConfCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Host a Zoom video conference and do presentation to participants",
		Contacts: []string{
			"chromeos-perf-reliability-eng@google.com",
			"cienet-development@googlegroups.com",
			"chicheny@google.com",
		},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > SPERA > Automation
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"ui.cuj_mode",     // Optional. Expecting "tablet" or "clamshell".
			"ui.collectTrace", // Optional. Expecting "enable" or "disable", default is "disable".
			// Zoom meet bot server address.
			"ui.zoom_bot_server",
			"ui.zoom_bot_token",
		},
		Params: []testing.Param{
			{
				Name:    "basic_two",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: zoomSmallRoomTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.TwoRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "basic_lacros_two",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           zoomSmallRoomTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.TwoRoomSize,
					BrowserType: browser.TypeLacros,
				},
			}, {
				Name:    "basic_small",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: zoomSmallRoomTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.SmallRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "basic_lacros_small",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           zoomSmallRoomTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.SmallRoomSize,
					BrowserType: browser.TypeLacros,
				},
			}, {
				Name:    "basic_large",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: zoomClassRoomTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.LargeRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "basic_lacros_large",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           zoomClassRoomTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.LargeRoomSize,
					BrowserType: browser.TypeLacros,
				},
			}, {
				Name:    "basic_class",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: zoomClassRoomTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.ClassRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "basic_lacros_class",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           zoomClassRoomTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.ClassRoomSize,
					BrowserType: browser.TypeLacros,
				},
			}, {
				Name:    "plus_large",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: zoomClassRoomTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Plus,
					RoomType:    conference.LargeRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "plus_lacros_large",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           zoomClassRoomTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Plus,
					RoomType:    conference.LargeRoomSize,
					BrowserType: browser.TypeLacros,
				},
			}, {
				Name:    "plus_class",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: zoomClassRoomTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Plus,
					RoomType:    conference.ClassRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "plus_lacros_class",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           zoomClassRoomTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Plus,
					RoomType:    conference.ClassRoomSize,
					BrowserType: browser.TypeLacros,
				},
			}, {
				Name:    "premium_large",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: zoomClassRoomTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Premium,
					RoomType:    conference.LargeRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "premium_lacros_large",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           zoomClassRoomTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Premium,
					RoomType:    conference.LargeRoomSize,
					BrowserType: browser.TypeLacros,
				},
			},
		},
	})
}

func ZoomConfCUJ(ctx context.Context, s *testing.State) {
	p := s.Param().(conference.TestParameters)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tabletMode, resetTabletMode, err := cuj.EnableTabletMode(ctx, tconn, s.Var, "ui.cuj_mode")
	if err != nil {
		s.Fatal("Failed to enable tablet mode: ", err)
	}
	defer resetTabletMode(cleanupCtx)

	var uiHandler cuj.UIActionHandler

	if tabletMode {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancel()

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

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to initialize keyboard input: ", err)
	}
	defer kb.Close(ctx)

	var traceConfigPath string
	if collect, ok := s.Var("ui.collectTrace"); ok && collect == "enable" {
		traceConfigPath = s.DataPath(cujrecorder.SystemTraceConfigFile)
	}
	host, ok := s.Var("ui.zoom_bot_server")
	if !ok {
		s.Fatal("Failed to get variable ui.zoom_bot_server")
	}

	sessionToken, ok := s.Var("ui.zoom_bot_token")
	if !ok {
		s.Fatal("Ffailed to get variable ui.zoom_bot_token")
	}

	roomType := p.RoomType
	account := cr.Creds().User
	outDir := s.OutDir()
	zmcli := conference.NewZoomConference(cr, tconn, kb, uiHandler, tabletMode, roomType, account, outDir)
	defer zmcli.End(cleanupCtx)

	cleanupZoomCtx := ctx
	ctx, cancelZoom := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancelZoom()
	roomSize := conference.ZoomRoomParticipants[roomType] - 1
	meetLink, cleanupZoom, err := zoomserver.CreateConference(ctx, roomSize, sessionToken, host)
	if err != nil {
		s.Fatal("Failed to run create zoom conference: ", err)
	}
	defer cleanupZoom(cleanupZoomCtx)

	// Shorten context a bit to allow for cleanup if Run fails.
	ctx, cancel = ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	testParams := &conference.TestParams{
		Cr:              cr,
		Conf:            zmcli,
		Tier:            p.Tier,
		BrowserType:     p.BrowserType,
		RoomType:        roomType,
		MeetLink:        meetLink,
		OutDir:          outDir,
		TraceConfigPath: traceConfigPath,
		TabletMode:      tabletMode,
	}

	if err := conference.Run(ctx, testParams); err != nil {
		s.Fatal("Failed to run zoom conference cuj: ", err)
	}
}
