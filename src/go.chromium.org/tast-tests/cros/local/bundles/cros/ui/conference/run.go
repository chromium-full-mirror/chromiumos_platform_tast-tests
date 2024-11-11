// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package conference

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/bond"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestParams stores data common to the tests run in this package.
type TestParams struct {
	Cr              *chrome.Chrome
	Conf            Conference
	Tier            cuj.Tier
	BrowserType     browser.Type
	RoomType        RoomType
	MeetLink        string
	OutDir          string
	TraceConfigPath string
	TabletMode      bool
	ExtendedDisplay bool
}

// Run runs the specified user scenario in conference room with different CUJ tiers.
func Run(ctx context.Context, params *TestParams) (retErr error) {
	var (
		cr              = params.Cr
		conf            = params.Conf
		tier            = params.Tier
		browserType     = params.BrowserType
		roomType        = params.RoomType
		meetLink        = params.MeetLink
		outDir          = params.OutDir
		traceConfigPath = params.TraceConfigPath
		tabletMode      = params.TabletMode
	)

	// Shorten context a bit to allow for cleanup.
	cleanUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	defer faillog.DumpUITreeWithScreenshotOnError(cleanUpCtx, outDir, func() bool { return retErr != nil }, cr, "ui_dump")

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the test API connection")
	}

	testing.ContextLog(ctx, "Start to get browser start time")
	browserStartTime, err := cuj.GetBrowserStartTime(ctx, tconn, true, tabletMode)
	if err != nil {
		return errors.Wrap(err, "failed to get browser start time")
	}
	br := cr.Browser()
	conf.SetBrowser(br)

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to create Test API connection for %v browser", browserType)
	}
	// Give 10 seconds to set initial settings. It is critical to ensure
	// cleanupSetting can be executed with a valid context so it has its
	// own cleanup context from other cleanup functions. This is to avoid
	// other cleanup functions executed earlier to use up the context time.
	cleanupSettingsCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cleanupSetting, err := cuj.InitializeSetting(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to set initial settings")
	}
	defer cleanupSetting(cleanupSettingsCtx)

	// Shorten the context to cleanup recorder.
	cleanUpRecorderCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	testing.ContextLog(ctx, "Start recording actions")
	options := cujrecorder.NewPerformanceCUJOptions()
	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, options)
	if err != nil {
		return errors.Wrap(err, "failed to create the recorder")
	}
	defer recorder.Close(cleanUpRecorderCtx)
	if err := cuj.AddPerformanceCUJMetrics(tconn, recorder); err != nil {
		return errors.Wrap(err, "failed to add metrics to recorder")
	}
	if err := recorder.AddCollectedMetrics(bTconn, browserType, cujrecorder.WebRTCMetrics()...); err != nil {
		return errors.Wrap(err, "failed to add metrics to recorder")
	}
	isNoRoom := roomType == NoRoom
	meetTimeout := 50 * time.Second
	if isNoRoom {
		meetTimeout = 70 * time.Second
	} else if tier == cuj.Plus {
		meetTimeout = 140 * time.Second
	} else if tier == cuj.Premium {
		meetTimeout = 3 * time.Minute
	}

	pv := perf.NewValues()
	if err := recorder.Run(ctx, func(ctx context.Context) error {
		// Start tracing now.
		if traceConfigPath != "" {
			if err := recorder.StartTracing(ctx, outDir, traceConfigPath); err != nil {
				return errors.Wrap(err, "failed to start tracing")
			}
			defer recorder.StopTracing(ctx)
		}

		// Collect GPU metrics in goroutine while other tests are being executed.
		errc := make(chan error, 1) // Buffered channel to make sure goroutine will not be blocked.
		gpuCtx, cancel := context.WithTimeout(ctx, meetTimeout+5*time.Second)
		defer cancel() // Make sure goroutine ctx will be cancelled before return.
		go func() {
			errc <- graphics.MeasureGPUCounters(gpuCtx, meetTimeout, pv)
		}()

		if !isNoRoom {
			// Only premium tier need to change background to blur at the beginning.
			toBlur := tier == cuj.Premium
			if err := conf.Join(ctx, meetLink, toBlur); err != nil {
				return err
			}
			// Basic steps:
			// 1. Set the layout to max tiled grid. (Google meet: "Tiled", Zoom: "Gallery")
			// 2. Switch to another tab (wikipedia) and back to meeting.
			// 3. Use video and audio control buttons.
			// 4. Open chat window and type.
			// 5. Set the layout to a minimal tiled grid. (Google meet: "Spotlight", Zoom: "Speacker View")
			if err := uiauto.Combine("basic actions",
				conf.SetLayoutMax,
				conf.SwitchTabs,
				conf.VideoAudioControl,
				conf.TypingInChat,
				conf.SetLayoutMin,
			)(ctx); err != nil {
				return err
			}
		}

		// Plus and premium tier.
		if tier == cuj.Plus || tier == cuj.Premium {
			application := googleSlides
			if tier == cuj.Premium {
				application = googleDocs
			}
			if err := conf.Presenting(ctx, application); err != nil {
				return err
			}
		}

		// Premium tier.
		if !isNoRoom && tier == cuj.Premium {
			if err := conf.BackgroundChange(ctx); err != nil {
				return err
			}
		}

		// Wait for meetTimeout expires in goroutine and get GPU result.
		if err := <-errc; err != nil {
			return errors.Wrap(err, "failed to collect GPU counters")
		}
		return nil
	}); err != nil {
		return errors.Wrap(err, "failed to conduct the recorder task")
	}

	// Use a short timeout value so it can return fast in case of failure.
	recordCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := recorder.Record(recordCtx, pv); err != nil {
		return errors.Wrap(err, "failed to record the data")
	}
	if err := recorder.SaveTraceFiles(recordCtx); err != nil {
		testing.ContextLog(recordCtx, "Failed to save trace files: ", err)
	}

	pv.Set(perf.Metric{
		Name:      "Browser.StartTime",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, float64(browserStartTime.Milliseconds()))

	pv.Set(perf.Metric{
		Name:      "TPS.Meet.NetworkLost",
		Unit:      "count",
		Direction: perf.SmallerIsBetter,
	}, float64(conf.LostNetworkCount()))

	pv.Set(perf.Metric{
		Name:      "TPS.Meet.DisplayAllParticipantsTime",
		Unit:      "s",
		Direction: perf.SmallerIsBetter,
	}, float64(conf.DisplayAllParticipantsTime().Seconds()))

	if err := pv.Save(outDir); err != nil {
		return errors.Wrap(err, "failed to save perf data")
	}

	if err := recorder.SaveHistograms(outDir); err != nil {
		return errors.Wrap(err, "failed to save histogram raw data")
	}

	return nil
}

// RunWithGoogleConfig runs google meet testing with google meet config.
func RunWithGoogleConfig(ctx context.Context, tconn *chrome.TestConn, meetConfig GoogleMeetConfig, p *TestParams) error {
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
		gmcli := NewGoogleMeetConference(p.Cr, tconn, kb, uiHandler, p.BrowserType, p.RoomType, p.OutDir, p.TabletMode, p.ExtendedDisplay)
		defer gmcli.End(cleanupGoogleMeetCtx)

		p.Conf = gmcli
		p.MeetLink = meetLink

		if err := Run(ctx, p); err != nil {
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
			return &BondError{Err: errors.Wrap(err, "failed to create meet link via BOND API")}
		}
		defer cleanupBond(cleanupCtx)

		return run(ctx, meetLink)
	}

	if p.RoomType == NoRoom {
		// Without Google Meet, there is no need to assign a meet url.
		if err := run(ctx, ""); err != nil {
			return errors.Wrap(err, "failed to run no room")
		}
		return nil
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
			elapsedTime := time.Since(startTime)
			if elapsedTime < meetConfig.RetryTimeout {
				// Record the complete run result if the failure is not because of timeout.
				lastError = err
			}
			if IsParticipantError(err) || IsBondError(err) {
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

func generateMeetLinkViaBond(ctx context.Context, meet GoogleMeetConfig, roomType RoomType) (meetLink string, cleanup func(ctx context.Context), err error) {
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
		botsDuration = time.Until(deadline.Add(90 * time.Second))
	}
	numBots := GoogleMeetRoomParticipants[roomType] - 1 // one of participants is the test itself
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
