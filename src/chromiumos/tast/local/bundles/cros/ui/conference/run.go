// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package conference

import (
	"context"
	"path/filepath"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/graphics"
	"chromiumos/tast/local/ui/cujrecorder"
	"chromiumos/tast/testing"
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

	// Dump the UI tree to the service/faillog subdirectory.
	// Don't dump directly into outDir
	// because it might be overridden by the test faillog after pulled back to remote server.
	defer faillog.DumpUITreeWithScreenshotOnError(cleanUpCtx, filepath.Join(outDir, "service"), func() bool { return retErr != nil }, cr, "ui_dump")

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the test API connection")
	}

	testing.ContextLog(ctx, "Start to get browser start time")
	l, browserStartTime, err := cuj.GetBrowserStartTime(ctx, tconn, true, tabletMode, browserType)
	if err != nil {
		return errors.Wrap(err, "failed to get browser start time")
	}
	br := cr.Browser()
	if l != nil {
		br = l.Browser()
	}
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
	if err := cuj.AddPerformanceCUJMetrics(browserType, tconn, bTconn, recorder); err != nil {
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
