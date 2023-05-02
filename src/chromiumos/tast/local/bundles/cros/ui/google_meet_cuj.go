// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"chromiumos/tast/common/cros/ui/setup"
	"chromiumos/tast/local/bundles/cros/ui/conference"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const googleMeetTimeout = 50 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         GoogleMeetCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Host a Google Meet video conference and do presentation to participants",
		Contacts: []string{
			"chromeos-perf-reliability-eng@google.com",
			"cienet-development@googlegroups.com",
			"chicheny@google.com",
		},
		BugComponent: "b:1025042", // ChromeOS > EngProd > Platform > SPERA > Automation
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"ui.cujAccountPool", // CrOS login credentials.
			"ui.cuj_mode",       // Optional. Expecting "tablet" or "clamshell".
			"ui.collectTrace",   // Optional. Expecting "enable" or "disable", default is "disable".
			// Credentials for BOND API.
			"ui.meet_bond_key",
			// Optional. The total timeout and inteval when bond api fails.
			"ui.meet_retry_timeout",
			"ui.meet_retry_interval",
		},
		Data: []string{cujrecorder.SystemTraceConfigFile},
		Params: []testing.Param{
			{
				Name:    "basic_two",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: googleMeetTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.TwoRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "basic_lacros_two",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           googleMeetTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.TwoRoomSize,
					BrowserType: browser.TypeLacros,
				},
			}, {
				Name:              "basic_two_crosbolt",
				Fixture:           "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout:           googleMeetTimeout,
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
				ExtraHardwareDeps: hwdep.D(setup.PerfCUJDevices()),
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.TwoRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:    "basic_small",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: googleMeetTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.SmallRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "basic_lacros_small",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           googleMeetTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.SmallRoomSize,
					BrowserType: browser.TypeLacros,
				},
			}, {
				Name:              "basic_small_crosbolt",
				Fixture:           "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout:           googleMeetTimeout,
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
				ExtraHardwareDeps: hwdep.D(setup.PerfCUJDevices()),
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.SmallRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:    "basic_large",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: googleMeetTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.LargeRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "basic_lacros_large",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           googleMeetTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.LargeRoomSize,
					BrowserType: browser.TypeLacros,
				},
			}, {
				Name:              "basic_large_crosbolt",
				Fixture:           "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout:           googleMeetTimeout,
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
				ExtraHardwareDeps: hwdep.D(setup.PerfCUJDevices()),
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.LargeRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:    "basic_class",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: googleMeetTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.ClassRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "basic_lacros_class",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           googleMeetTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.ClassRoomSize,
					BrowserType: browser.TypeLacros,
				},
			}, {
				Name:              "basic_class_crosbolt",
				Fixture:           "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout:           googleMeetTimeout,
				ExtraAttr:         []string{"group:crosbolt", "crosbolt_perbuild"},
				ExtraHardwareDeps: hwdep.D(setup.PerfCUJDevices()),
				Val: conference.TestParameters{
					Tier:        cuj.Basic,
					RoomType:    conference.ClassRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:    "plus_large",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: googleMeetTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Plus,
					RoomType:    conference.LargeRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "plus_lacros_large",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           googleMeetTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Plus,
					RoomType:    conference.LargeRoomSize,
					BrowserType: browser.TypeLacros,
				},
			}, {
				Name:    "plus_class",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: googleMeetTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Plus,
					RoomType:    conference.ClassRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "plus_lacros_class",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           googleMeetTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Plus,
					RoomType:    conference.ClassRoomSize,
					BrowserType: browser.TypeLacros,
				},
			}, {
				Name:    "premium_large",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: googleMeetTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Premium,
					RoomType:    conference.LargeRoomSize,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "premium_lacros_large",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           googleMeetTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Premium,
					RoomType:    conference.LargeRoomSize,
					BrowserType: browser.TypeLacros,
				},
			}, {
				Name:    "plus_no_meet",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: 10 * time.Minute,
				Val: conference.TestParameters{
					Tier:        cuj.Plus,
					RoomType:    conference.NoRoom,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "plus_lacros_no_meet",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           10 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Plus,
					RoomType:    conference.NoRoom,
					BrowserType: browser.TypeLacros,
				},
			}, {
				Name:    "premium_no_meet",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: 10 * time.Minute,
				Val: conference.TestParameters{
					Tier:        cuj.Premium,
					RoomType:    conference.NoRoom,
					BrowserType: browser.TypeAsh,
				},
			}, {
				Name:              "premium_lacros_no_meet",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           10 * time.Minute,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Premium,
					RoomType:    conference.NoRoom,
					BrowserType: browser.TypeLacros,
				},
			},
		},
	})
}

func GoogleMeetCUJ(ctx context.Context, s *testing.State) {
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

	var traceConfigPath string
	if collect, ok := s.Var("ui.collectTrace"); ok && collect == "enable" {
		traceConfigPath = s.DataPath(cujrecorder.SystemTraceConfigFile)
	}

	meetConfig, err := conference.GetGoogleMeetConfig(ctx, s, p.RoomType)
	if err != nil {
		s.Fatal("Failed to get meet config: ", err)
	}

	testParams := &conference.TestParams{
		Cr:              cr,
		Tier:            p.Tier,
		BrowserType:     p.BrowserType,
		RoomType:        p.RoomType,
		OutDir:          s.OutDir(),
		TraceConfigPath: traceConfigPath,
		TabletMode:      tabletMode,
		ExtendedDisplay: false,
	}

	if err := conference.RunWithGoogleConfig(ctx, tconn, meetConfig, testParams); err != nil {
		s.Fatal("Failed to run google meet cuj: ", err)
	}
}
