// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package videoeditingapp contains the test code for VideoEditingApp.
package videoeditingapp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/spera/videoeditingapp/wevideo"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googledocs"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	weVideoTitle    = "WeVideo"
	googleDocsTitle = "Google Docs"
	clip1           = "Springtime Birds Migration Northern Parula Warbler 4K"
	clip2           = "Hammock and Beach"
	clipTime1       = "00:23:00"
	clipTime2       = "00:22:00"
	videoTrack      = "Video 1"
	textTrack       = "Text 1"
	demoText        = "Springtime Birds"
	docParagraph    = "The Little Prince's story follows a young prince who visits various planets in space, including Earth, and addresses themes of loneliness, friendship, love, and loss."
)

const clipsTotalDuration = 45 * time.Second

// Run runs the VideoEditingApp test.
func Run(ctx context.Context, outDir, traceConfigPath string, cr *chrome.Chrome, tabletMode bool, bt browser.Type) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create test API connection")
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open the keyboard")
	}
	defer kb.Close(ctx)

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

	uiHdl, err := uiHandler(ctx, tconn, tabletMode)
	if err != nil {
		return err
	}

	testing.ContextLog(ctx, "Start to get browser start time")
	l, browserStartTime, err := cuj.GetBrowserStartTime(ctx, tconn, true, tabletMode, bt)
	if err != nil {
		return errors.Wrap(err, "failed to get browser start time")
	}
	br := cr.Browser()
	if l != nil {
		br = l.Browser()
	}
	defer cuj.CloseAllWindows(ctx, tconn)
	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to create Test API connection for %v browser", bt)
	}

	options := cujrecorder.NewPerformanceCUJOptions()
	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, options)
	if err != nil {
		return errors.Wrap(err, "failed to create a recorder")
	}
	defer recorder.Close(cleanupCtx)
	if err := cuj.AddPerformanceCUJMetrics(bt, tconn, bTconn, recorder); err != nil {
		return errors.Wrap(err, "failed to add metrics to recorder")
	}
	if err := recorder.Run(ctx, func(ctx context.Context) error {
		// Start tracing now.
		if traceConfigPath != "" {
			if err := recorder.StartTracing(ctx, outDir, traceConfigPath); err != nil {
				return errors.Wrap(err, "failed to start tracing")
			}
			defer recorder.StopTracing(ctx)
		}

		return videoEditingScenario(ctx, tconn, cr, kb, uiHdl, tabletMode, outDir, br)
	}); err != nil {
		return errors.Wrap(err, "failed to run the video editing on the WeVideo web")
	}

	pv := perf.NewValues()
	pv.Set(perf.Metric{
		Name:      "Browser.StartTime",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, float64(browserStartTime.Milliseconds()))

	// Use a short timeout value so it can return fast in case of failure.
	recordCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := recorder.Record(recordCtx, pv); err != nil {
		return errors.Wrap(err, "failed to record")
	}
	if err := recorder.SaveTraceFiles(recordCtx); err != nil {
		testing.ContextLog(recordCtx, "Failed to save trace files: ", err)
	}
	if err = pv.Save(outDir); err != nil {
		return errors.Wrap(err, "failed to store values")
	}
	if err := recorder.SaveHistograms(outDir); err != nil {
		return errors.Wrap(err, "failed to save histogram raw data")
	}
	return nil
}

func videoEditingScenario(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, kb *input.KeyboardEventWriter,
	uiHdl cuj.UIActionHandler, tabletMode bool, outDir string, br *browser.Browser) error {
	hasError := true
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()
	account := cr.Creds().User
	w := wevideo.NewWeVideo(tconn, kb, uiHdl, tabletMode, br)
	if err := w.Open()(ctx); err != nil {
		return errors.Wrap(err, "failed to open the WeVideo page")
	}
	defer cleanup(cleanupCtx, tconn, cr, w, outDir, func() bool { return hasError })

	if err := uiauto.Combine("run the video editing scenario",
		w.Login(account),
		w.Create(),
		w.AddStockVideo(clip1, "", clipTime1, videoTrack),
		w.AddStockVideo(clip2, clip1, clipTime2, videoTrack),
		w.AddText(clip1, textTrack, demoText),
	)(ctx); err != nil {
		return err
	}

	if err := googledocs.NewGoogleDocs(ctx, tconn, br, uiHdl, true); err != nil {
		return err
	}
	defer docCleanup(cleanupCtx, tconn, cr, uiHdl, outDir, func() bool { return hasError })

	if err := uiauto.Combine("run the video editing scenario",
		googledocs.EditDoc(tconn, kb, docParagraph),
		uiHdl.SwitchToAppWindowByName("Chrome", weVideoTitle),
		w.AddTransition(clip2),
		w.PlayVideo(clip1, clipsTotalDuration),
	)(ctx); err != nil {
		return err
	}
	if err := cuj.GenerateADF(ctx, tconn, tabletMode); err != nil {
		return errors.Wrap(err, "failed to generate ADF")
	}
	hasError = false
	return nil
}

func uiHandler(ctx context.Context, tconn *chrome.TestConn, tabletMode bool) (cuj.UIActionHandler, error) {
	var uiHdl cuj.UIActionHandler
	var err error
	if tabletMode {
		if uiHdl, err = cuj.NewTabletActionHandler(ctx, tconn); err != nil {
			return uiHdl, errors.Wrap(err, "failed to create tablet action handler")
		}
	} else {
		if uiHdl, err = cuj.NewClamshellActionHandler(ctx, tconn); err != nil {
			return uiHdl, errors.Wrap(err, "failed to create clamshell action handler")
		}
	}
	return uiHdl, nil
}

func cleanup(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, w *wevideo.WeVideo, outDir string, hasError func() bool) {
	// The screenshot and ui tree dump must been taken before the connection is closed.
	faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, hasError, cr, "ui_dump")
	w.Close(ctx)
}

func docCleanup(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, uiHdl cuj.UIActionHandler, outDir string, hasError func() bool) {
	// docCleanup switches to the document page and deletes it.
	docCleanup := uiauto.Combine("switch to the document page and delete it",
		uiHdl.SwitchToAppWindowByName("Chrome", googleDocsTitle),
		googledocs.DeleteDoc(tconn),
	)
	// If case fails, dump the last screen before deleting the document.
	faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, hasError, cr, "ui_dump_last")
	if err := docCleanup(ctx); err != nil {
		// Only log the error.
		testing.ContextLog(ctx, "Failed to clean up the document: ", err)
	}
}
