// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package spera

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/bond"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/spera/conference"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const googleMeetTimeout = 50 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         GoogleMeet,
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
			"spera.cuj_mode",               // Optional. Expecting "tablet" or "clamshell".
			"spera.collectTrace",           // Optional. Expecting "enable" or "disable", default is "disable".
			"spera.collectWebRTCInternals", // Optional. Expecting "true" or "false", default is "false".
			// Credentials for BOND API.
			"spera.meet_bond_key",
			// Optional. The total timeout and inteval when bond api fails.
			"spera.meet_retry_timeout",
			"spera.meet_retry_interval",
			// Optional. Expecting "google", "external", default is "google".
			"spera.GoogleMeet.web_source",
		},
		Params: []testing.Param{
			{
				Name:    "grid_essential",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: googleMeetTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Essential,
					RoomType:    conference.GridRoomSize,
					BrowserType: browser.TypeAsh,
				},
			},
			{
				Name:    "grid_advanced",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: googleMeetTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Advanced,
					RoomType:    conference.GridRoomSize,
					BrowserType: browser.TypeAsh,
				},
			},
			{
				Name:    "class_essential",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: googleMeetTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Essential,
					RoomType:    conference.ClassRoomSize,
					BrowserType: browser.TypeAsh,
				},
			},
			{
				Name:    "class_advanced",
				Fixture: "loggedInAndKeepStateWithLowResFakeCamera",
				Timeout: googleMeetTimeout,
				Val: conference.TestParameters{
					Tier:        cuj.Advanced,
					RoomType:    conference.ClassRoomSize,
					BrowserType: browser.TypeAsh,
				},
			},
			{
				Name:              "grid_essential_lacros",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           googleMeetTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Essential,
					RoomType:    conference.GridRoomSize,
					BrowserType: browser.TypeLacros,
				},
			},
			{
				Name:              "grid_advanced_lacros",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           googleMeetTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Advanced,
					RoomType:    conference.GridRoomSize,
					BrowserType: browser.TypeLacros,
				},
			},
			{
				Name:              "class_essential_lacros",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           googleMeetTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Essential,
					RoomType:    conference.ClassRoomSize,
					BrowserType: browser.TypeLacros,
				},
			},
			{
				Name:              "class_advanced_lacros",
				Fixture:           "loggedInAndKeepStateLacrosWithLowResFakeCamera",
				Timeout:           googleMeetTimeout,
				ExtraSoftwareDeps: []string{"lacros"},
				Val: conference.TestParameters{
					Tier:        cuj.Advanced,
					RoomType:    conference.ClassRoomSize,
					BrowserType: browser.TypeLacros,
				},
			},
		},
	})
}

func GoogleMeet(ctx context.Context, s *testing.State) {
	p := s.Param().(conference.TestParameters)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tabletMode, resetTabletMode, err := cuj.EnableTabletMode(ctx, tconn, s.Var, "spera.cuj_mode")
	if err != nil {
		s.Fatal("Failed to enable tablet mode: ", err)
	}
	defer resetTabletMode(cleanupCtx)

	var traceConfigPath string
	if collect, ok := s.Var("spera.collectTrace"); ok && collect == "enable" {
		traceConfigPath = s.DataPath(cujrecorder.SystemTraceConfigFile)
	}

	// The default web source is the google websites.
	webSource := cuj.GoogleWebSource
	v, ok := s.Var("spera.GoogleMeet.web_source")
	if ok && strings.ToLower(v) == string(cuj.ExternalWebSource) {
		webSource = cuj.ExternalWebSource
	}

	meetConfig, err := conference.GetGoogleMeetConfig(ctx, s)
	if err != nil {
		s.Fatal("Failed to get meet config: ", err)
	}

	testParams := &conference.TestParams{
		Cr:              cr,
		Tier:            p.Tier,
		WebSource:       webSource,
		BrowserType:     p.BrowserType,
		RoomType:        p.RoomType,
		OutDir:          s.OutDir(),
		TraceConfigPath: traceConfigPath,
		TabletMode:      tabletMode,
	}
	if err := runWithGoogleConfig(ctx, tconn, meetConfig, testParams); err != nil {
		s.Fatal("Failed to run google meet: ", err)
	}
}

func runWithGoogleConfig(ctx context.Context, tconn *chrome.TestConn, meetConfig conference.GoogleMeetConfig, p *conference.TestParams) error {
	var uiHandler cuj.UIActionHandler
	var err error

	if p.TabletMode {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancel()

		cleanup, err := display.RotateToLandscape(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to rotate display to landscape")
		}
		defer cleanup(cleanupCtx)
		if uiHandler, err = cuj.NewTabletActionHandler(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to create tablet action handler")
		}
	} else {
		if uiHandler, err = cuj.NewClamshellActionHandler(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to create clamshell action handler")
		}
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to initialize keyboard input")
	}
	defer kb.Close(ctx)

	run := func(ctx context.Context, meetLink string) error {
		cleanupGoogleMeetCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
		defer cancel()

		// Creates a Google Meet conference instance which implements conference.Conference methods
		// which provides conference operations.
		gmcli := conference.NewGoogleMeetConference(p.Cr, tconn, kb, uiHandler, p.BrowserType, p.RoomType, p.OutDir, p.TabletMode)
		defer gmcli.End(cleanupGoogleMeetCtx)

		p.Conf = gmcli
		p.MeetLink = meetLink
		p.CollectWebRTCInternals = meetConfig.CollectWebRTCInternals

		if err := conference.Run(ctx, p); err != nil {
			return errors.Wrap(err, "failed to run Google Meet conference")
		}

		return nil
	}

	runWithMeetLinkViaBond := func(ctx context.Context) error {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancel()

		meetLink, cleanupBond, err := generateMeetLinkViaBond(ctx, meetConfig, p.RoomType)
		if err != nil {
			return &conference.BondError{Err: errors.Wrap(err, "failed to create meet link via BOND API")}
		}
		defer cleanupBond(cleanupCtx)

		return run(ctx, meetLink)
	}

	// If meet.RetryTimeout equal to 0, don't do any retry.
	if meetConfig.RetryTimeout == 0 {
		testing.ContextLog(ctx, "Start running meet scenario")
		if err := runWithMeetLinkViaBond(ctx); err != nil {
			return errors.Wrap(err, "failed to run google meet")
		}
	}

	var lastError error
	startTime := time.Now()
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := runWithMeetLinkViaBond(ctx); err != nil {
			elapsedTime := time.Now().Sub(startTime)
			if elapsedTime < meetConfig.RetryTimeout {
				// Record the complete run result if the failure is not because of timeout.
				lastError = err
			}
			if conference.IsParticipantError(err) || conference.IsBondError(err) {
				testing.ContextLogf(ctx, "Wait %v and try to run meet scenario again; caused by error: %v", meetConfig.RetryInterval, err)
				return err
			}
			return testing.PollBreak(err) // Break if error is not participant number related.
		}

		return nil
	}, &testing.PollOptions{Timeout: meetConfig.RetryTimeout, Interval: meetConfig.RetryInterval}); err != nil {
		// Return test failure reason of last complete run.
		if lastError != nil {
			err = lastError
		}
		return errors.Wrap(err, "failed to run google meet")
	}

	return nil
}

func generateMeetLinkViaBond(ctx context.Context, meet conference.GoogleMeetConfig, roomType conference.RoomType) (meetLink string, cleanup func(ctx context.Context), err error) {
	var (
		bondConn        *bond.Client
		bondMeetingCode string
		numFailures     int
	)
	cleanupfunc := func(ctx context.Context) {
		if bondConn != nil {
			if bondMeetingCode != "" {
				bondConn.RemoveAllBotsFromConference(ctx, bondMeetingCode)
			}
			bondConn.Close()
		}
	}
	// Connect.
	bondConn, err = bond.NewClient(ctx, bond.WithCredsJSON(meet.BondCreds), bond.WithExternalEndpoint())
	if err != nil {
		return "", cleanupfunc, errors.Wrap(err, "BOND API2: Failed to connect")
	}
	defer func(ctx context.Context) {
		if err != nil {
			bondConn.Close()
		}
	}(ctx)

	// Create room with bots.
	botsDuration := 60 * time.Minute // one hour long by default.
	deadline, ok := ctx.Deadline()
	if ok {
		botsDuration = deadline.Add(90 * time.Second).Sub(time.Now())
	}
	numBots := conference.GoogleMeetRoomParticipants[roomType] - 1 // one of participants is the test itself
	bondMeetingCode, numFailures, err = bondConn.CreateConferenceWithBots(ctx, numBots, botsDuration)
	defer func(ctx context.Context) {
		if err != nil {
			bondConn.RemoveAllBotsFromConference(ctx, bondMeetingCode)
		}
	}(ctx)

	if err != nil || numFailures > 0 {
		return "", cleanupfunc, errors.Wrapf(err, "BOND API2: %d bots failed to connect", numFailures)
	}
	testing.ContextLogf(ctx, "BOND API2: Created conference: %+v and added %d bots for the duration of %v", bondMeetingCode, numBots, botsDuration)

	// Make the room created by BOND the first one to try.
	meetLink = "https://meet.google.com/" + bondMeetingCode

	return meetLink, cleanupfunc, nil
}
