// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package videoconfproxy contains the test code for VideoConfProxy.
package videoconfproxy

import (
	"context"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/common/perf"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/chrome/webutil"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/ui/cujrecorder"
	"chromiumos/tast/testing"
)

// TestParams stores data common to the tests run in this package.
type TestParams struct {
	BrowserType     browser.Type
	VideoCallURL    string
	OutDir          string
	TraceConfigPath string
	TabletMode      bool
}

// Run runs the VideoConfProxy test.
func Run(ctx context.Context, cr *chrome.Chrome, p TestParams) (retErr error) {
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
	defer kb.Close()

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

	testing.ContextLog(ctx, "Start to get browser start time")
	l, browserStartTime, err := cuj.GetBrowserStartTime(ctx, tconn, true, p.TabletMode, p.BrowserType)
	if err != nil {
		return errors.Wrap(err, "failed to get browser start time")
	}
	br := cr.Browser()
	if l != nil {
		defer l.Close(ctx)
		br = l.Browser()
	}
	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to create Test API connection for %v browser", p.BrowserType)
	}

	// uiHandler will be assigned with different instances for clamshell and tablet mode.
	var uiHandler cuj.UIActionHandler
	if p.TabletMode {
		if uiHandler, err = cuj.NewTabletActionHandler(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to create tablet action handler")
		}
	} else {
		if uiHandler, err = cuj.NewClamshellActionHandler(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to create clamshell action handler")
		}
	}
	defer uiHandler.Close()

	// Shorten the context to cleanup recorder.
	cleanUpRecorderCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	options := cujrecorder.NewPerformanceCUJOptions()
	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, options)
	if err != nil {
		return errors.Wrap(err, "failed to create a recorder")
	}
	defer recorder.Close(cleanUpRecorderCtx)
	if err := cuj.AddPerformanceCUJMetrics(p.BrowserType, tconn, bTconn, recorder); err != nil {
		return errors.Wrap(err, "failed to add metrics to recorder")
	}
	if err := recorder.AddCollectedMetrics(bTconn, p.BrowserType, cujrecorder.WebRTCMetrics()...); err != nil {
		return errors.Wrap(err, "failed to add metrics to recorder")
	}

	pv := perf.NewValues()
	if err := recorder.Run(ctx, func(ctx context.Context) (recorderErr error) {
		// Start tracing now.
		if p.TraceConfigPath != "" {
			if err := recorder.StartTracing(ctx, p.OutDir, p.TraceConfigPath); err != nil {
				return errors.Wrap(err, "failed to start tracing")
			}
			defer recorder.StopTracing(ctx)
		}

		docsConn, err := br.NewConn(ctx, cuj.VideoCallDocsURL, browser.WithNewWindow())
		if err != nil {
			return errors.Wrap(err, "failed to open docs window")
		}
		defer func(ctx context.Context) {
			faillog.DumpUITreeWithScreenshotOnError(ctx, p.OutDir, func() bool { return retErr != nil }, cr, "ui_dump_docs")
			docsConn.CloseTarget(ctx)
			docsConn.Close()
		}(cleanupCtx)
		videoConn, err := uiHandler.NewChromeTab(ctx, br, p.VideoCallURL, true)
		if err != nil {
			return errors.Wrapf(err, "failed to open URL: %s", p.VideoCallURL)
		}
		defer func(ctx context.Context) {
			faillog.DumpUITreeWithScreenshotOnError(ctx, p.OutDir, func() bool { return recorderErr != nil }, cr, "ui_dump")
			videoConn.CloseTarget(ctx)
			videoConn.Close()
		}(cleanupCtx)
		if err := webutil.WaitForQuiescence(ctx, videoConn, time.Minute); err != nil {
			return errors.Wrap(err, "failed to wait for tab to achieve quiescence")
		}

		ui := uiauto.New(tconn)
		bubbleView := nodewith.ClassName("PermissionPromptBubbleView").First()
		allowButton := nodewith.Name("Allow").Role(role.Button).Ancestor(bubbleView)
		clearPromptBubble := uiauto.IfSuccessThen(ui.WithTimeout(3*time.Second).WaitUntilExists(allowButton),
			ui.DoDefaultUntil(allowButton, ui.WithTimeout(3*time.Second).WaitUntilGone(allowButton)))
		editHereField := nodewith.Name("Edit here").Role(role.TextField)

		if err := uiauto.Combine("initial test",
			clearPromptBubble,
			putWindowSideBySide(tconn),
			uiauto.NamedAction("select text input field", ui.LeftClick(editHereField)),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to initial test")
		}

		if err := videoConfProxyScenario(ctx, tconn, videoConn, kb, pv); err != nil {
			return err
		}
		// To collect the ADF/PDF histogram, clicking on the first video to make video
		// page as the active window. Through the switch window, the focus will be on
		// the url field on lacros, so click on the content of the web page to make the
		// window active.
		firstVideo := nodewith.Role(role.Video).First()
		if err := ui.LeftClick(firstVideo)(ctx); err != nil {
			return err
		}
		if err := cuj.GeneratePDF(ctx, bTconn, kb); err != nil {
			testing.ContextLog(ctx, "Failed to generate PDF histogram: ", err)
		}
		if err := cuj.GenerateADF(ctx, tconn, p.TabletMode); err != nil {
			return errors.Wrap(err, "failed to generate ADF")
		}

		// Stop WebRTC to generate WebRTC video metrics.
		// window.stopWebRTC() is a function exposed by JavaScript in video_conf_proxy.html.
		if err := videoConn.Eval(ctx, "window.stopWebRTC()", nil); err != nil {
			return errors.Wrap(err, "failed to stop WebRTC")
		}
		return nil
	}); err != nil {
		return errors.Wrap(err, "failed to run the video conf proxy scenario")
	}

	pv.Set(perf.Metric{
		Name:      "Browser.StartTime",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
	}, float64(browserStartTime.Milliseconds()))

	// Use a short timeout value so it can return fast in case of failure.
	recordCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	if err := recorder.Record(recordCtx, pv); err != nil {
		return errors.Wrap(err, "failed to report")
	}
	if err := recorder.SaveTraceFiles(recordCtx); err != nil {
		testing.ContextLog(recordCtx, "Failed to save trace files: ", err)
	}
	if err = pv.Save(p.OutDir); err != nil {
		return errors.Wrap(err, "failed to store values")
	}
	if err := recorder.SaveHistograms(p.OutDir); err != nil {
		return errors.Wrap(err, "failed to save histogram raw data")
	}

	return nil
}

func putWindowSideBySide(tconn *chrome.TestConn) action.Action {
	return func(ctx context.Context) error {
		windows, err := ash.GetAllWindows(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get all windows")
		}
		if len(windows) != 2 {
			return errors.Errorf("expect 2 windows but got %d", len(windows))
		}
		if err := ash.SetWindowStateAndWait(ctx, tconn, windows[0].ID, ash.WindowStateRightSnapped); err != nil {
			return errors.Wrap(err, "failed to snap first window to the right")
		}
		if err := ash.SetWindowStateAndWait(ctx, tconn, windows[1].ID, ash.WindowStateLeftSnapped); err != nil {
			return errors.Wrap(err, "failed to snap second window to the left")
		}

		return nil
	}
}

func videoConfProxyScenario(ctx context.Context, tconn *chrome.TestConn, videoConn *chrome.Conn, kb *input.KeyboardEventWriter, pv *perf.Values) error {
	const (
		notes         = "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. "
		repeatTimeout = 2 * time.Minute
	)
	tracker := NewVideoConfProxyInfoTracker(ctx, tconn, videoConn)
	if err := tracker.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start VideoConfProxyInfoTracker")
	}

	repeatTask := uiauto.Combine("sleep and type",
		uiauto.Sleep(5*time.Second),
		kb.TypeAction(notes))
	// Repeat the task for 2 minutes.
	now := time.Now()
	after := now.Add(repeatTimeout)
	for {
		if err := repeatTask(ctx); err != nil {
			return err
		}
		now = time.Now()
		if now.After(after) {
			break
		}
	}
	if err := tracker.Stop(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to stop VideoConfProxyInfoTracker: ", err)
	}
	if err := tracker.Record(ctx, pv); err != nil {
		testing.ContextLog(ctx, "Failed to record VideoConfProxyInfoTracker: ", err)
	}
	return nil
}
