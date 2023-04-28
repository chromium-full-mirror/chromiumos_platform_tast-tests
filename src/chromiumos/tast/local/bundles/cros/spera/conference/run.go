// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package conference

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"chromiumos/tast/common/perf"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/graphics"
	"chromiumos/tast/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestParams stores data common to the tests run in this package.
type TestParams struct {
	Cr                     *chrome.Chrome
	Conf                   Conference
	Tier                   cuj.Tier
	WebSource              cuj.WebSourceType
	BrowserType            browser.Type
	RoomType               RoomType
	MeetLink               string
	OutDir                 string
	TraceConfigPath        string
	TabletMode             bool
	CollectWebRTCInternals bool
}

// Run runs the specified user scenario in conference room with different CUJ tiers.
func Run(ctx context.Context, params *TestParams) (retErr error) {
	var (
		cr                     = params.Cr
		conf                   = params.Conf
		tier                   = params.Tier
		browserType            = params.BrowserType
		roomType               = params.RoomType
		meetLink               = params.MeetLink
		outDir                 = params.OutDir
		traceConfigPath        = params.TraceConfigPath
		tabletMode             = params.TabletMode
		collectWebRTCInternals = params.CollectWebRTCInternals
	)
	url := cuj.WikipediaURL
	if params.WebSource == cuj.GoogleWebSource {
		url = cuj.GoogleHelpChromeURL
	}

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

	isAdvanced := tier == cuj.Advanced
	isGridRoom := roomType == GridRoomSize
	meetTimeout := 50 * time.Second
	if isAdvanced {
		meetTimeout = 140 * time.Second
	} else if isAdvanced && isGridRoom {
		meetTimeout = 3 * time.Minute
	}

	if collectWebRTCInternals {
		webRTCInternalsConn, err := cuj.OpenWebRTCInternals(ctx, tconn, br)
		if err != nil {
			return err
		}
		defer webRTCInternalsConn.Close()
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

		if err := conf.Join(ctx, meetLink); err != nil {
			return err
		}
		// Essential steps:
		// 1. Set the layout to max tiled grid. (Google meet: "Tiled", Zoom: "Gallery")
		// 2. Switch to another tab (wikipedia) and back to meeting.
		// 3. Use video and audio control buttons.
		// 4. Open chat window and type.
		// 5. Set the layout to a minimal tiled grid. (Google meet: "Spotlight", Zoom: "Speacker View")
		if err := uiauto.Combine("essential actions",
			conf.SetLayoutMax,
			conf.SwitchTabs(url),
			conf.VideoAudioControl,
			conf.TypingInChat,
			conf.SetLayoutMin,
		)(ctx); err != nil {
			return err
		}

		// Advanced tier presents Google apps.
		// Class room presents Google Slides
		// Grid room presents Google Docs.
		if isAdvanced {
			application := googleSlides
			if isGridRoom {
				application = googleDocs
			}
			if err := conf.Presenting(ctx, application); err != nil {
				return err
			}
		}

		// If it's an Advanced tier and a grid room, change the background.
		if isAdvanced && isGridRoom {
			if err := conf.BackgroundChange(ctx); err != nil {
				return err
			}
		}
		if err := cuj.GenerateADF(ctx, tconn, tabletMode); err != nil {
			return errors.Wrap(err, "failed to generate ADF")
		}
		if collectWebRTCInternals {
			participants, err := conf.GetParticipants(ctx)
			if err != nil {
				return err
			}
			numBots := participants - 1
			if err := reportWebRTCInternals(ctx, pv, tconn, bTconn, cr.NormalizedUser(), outDir, numBots, tier != cuj.Essential); err != nil {
				return errors.Wrap(err, "failed to report WebRTC internals")
			}
		}

		closeFunc := func(ctx context.Context) error {
			if browserType == browser.TypeLacros {
				tabs, err := browser.AllTabs(ctx, bTconn)
				if err != nil {
					return err
				}
				// Leaving one tab is critical to keep the lacros-chrome process running.
				if len(tabs) == 1 {
					return browser.ReplaceAllTabsWithSingleNewTab(ctx, bTconn)
				}
			}
			return conf.CloseConference(ctx)
		}
		if err := cuj.RunAndWaitLCPHistograms(ctx, bTconn, closeFunc); err != nil {
			testing.ContextLog(ctx, "Failed to run and wait for LCP histograms to update: ", err)
		}

		// Wait for meetTimeout expires in goroutine and get GPU result.
		if err := <-errc; err != nil {
			return errors.Wrap(err, "failed to collect GPU counters")
		}
		return nil
	}); err != nil {
		err = CheckCommonError(ctx, tconn, err)
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

// reportWebRTCInternals reports information from WebRTC internals and dumps to performance metrics.
func reportWebRTCInternals(ctx context.Context, pv *perf.Values, tconn, bTconn *chrome.TestConn, username, outDir string, numBots int, present bool) error {
	testing.ContextLog(ctx, "Start reporting performance metrics from WebRTC internals dump file")
	ui := uiauto.New(tconn)
	webRTCUI := ui.WithTimeout(10 * time.Minute)
	path, err := cuj.DumpWebRTCInternals(ctx, tconn, webRTCUI, username)
	if err != nil {
		testing.ContextLog(ctx, "Failed to download dump from chrome://webrtc-internals: ", err)
	} else {
		dump, readErr := os.ReadFile(path)
		if readErr != nil {
			return errors.Wrap(readErr, "failed to read WebRTC internals dump from Downloads folder")
		}
		defer os.Remove(path)

		if err := os.WriteFile(filepath.Join(outDir, "webrtc-internals.json"), dump, 0644); err != nil {
			return errors.Wrap(err, "failed to write WebRTC internals dump to test results folder")
		}
		if err := cuj.ReportWebRTCInternals(pv, dump, numBots, present); err != nil {
			testing.ContextLog(ctx, "Failed to report info from WebRTC internals dump to performance metrics: ", err)
		}
	}

	return nil
}
