// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package webrtc provides functions to measure performance metrics in a video
// conference using WebRTC API.
package webrtc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/media/webrtc"
	"go.chromium.org/tast-tests/cros/local/power"
	pm "go.chromium.org/tast-tests/cros/local/power/metrics"
	"go.chromium.org/tast-tests/cros/local/tracing"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// VCTestParams is a structure about test parameters.
type VCTestParams struct {
	// If Step is true, then performance values in each step (e.g. idle, only open camera) are collected.
	// If it is false, then performance values only in a video conference with |numPeople| persons.
	Step bool
	// If Mouse is true, then opens a mouse tab in a new window and there clicks,
	// moves the cursor, click and scroll. If Present and Mouse are both true,
	// then only the mouse tab opens and the tab is presented.
	Mouse bool
	// Numpeope is the number of persons in a video conference.
	// This can be set only if Step is false and it must be more than two.
	NumPeople int
	// If Present is true, then opens a presentation tab in a new window and
	// capture the tab. One more encoder for the tab capture will run, but no
	// decoder runs for it.
	Present bool
	// If Text is true, then opens a text area tab in a new window and texts are
	// input in the text area. The text input latency is measured. If Present
	// and Text both are true, then only the text area tab opens and the tab is
	// presented.
	Text bool
	// If Trace is true, then perfetto tracing is executed and the tracing
	// result is saved in the result directory.
	Trace bool
	// If NoiseCancellation is true, enable input noise cancellation on the platform.
	NoiseCancellation bool
	// If StyleTransfer is true, enable input style transfer on the platform.
	StyleTransfer bool
	// BrowserType represents chrome browser type that the test runs with.
	BrowserType browser.Type
}

const (
	// vcHTML is the HTML holding a video conference using WebRTC API.
	vcHTML = "webrtc/video_conference.html"
	// vcTitle is the html title of vcHTML.
	vcTitle = "WebRTC VideoConference"
	// presentHTML is the HTML changing the content in 30fps for presentation.
	presentHTML = "webrtc/presentation.html"
	// presentTitle is the html title of presentHTML.
	presentTitle = "presentation test"
	// textHTML is the HTML that has a text area.
	textHTML = "webrtc/text.html"
	// textTitle is the html title of presentHTML.
	textTitle = "text test"
	// mouseHTML is the HTML that has letters.
	mouseHTML = "webrtc/mouse.html"
	// mouseTitle is the html title of mouseHTML.
	mouseTitle = "mouse test"

	// Peretto configuration file.
	traceConfigFile = "webrtc/perfetto_trace.txtpb"

	powerInterval          = 5 * time.Second // Power library metrics collection interval.
	typingInterval         = 5 * time.Second // Interval of typing in text area
	mouseOperatingInterval = 3 * time.Second // Interval of operating a mouse.
)

// TestFiles returns the files required running the test.
func TestFiles() []string {
	return []string{
		vcHTML,
		presentHTML,
		textHTML,
		mouseHTML,
		"webrtc/canvas_animation.js",
		"webrtc/video_conference.js",
		traceConfigFile,
	}
}

var graphicsHistograms = []string{
	// This event needs UI animation change. We collect this metric only mouse test cases.
	"Graphics.Smoothness.PercentDroppedFrames3.AllSequences",
}

var mouseHistograms = []string{
	"EventLatency.MouseDragged.TotalLatency",
	// MouseMoved.TotalLatency is not recorded probably due to blink issue.
	// Collect the event once the root cause is fixed.
	// "EventLatency.MouseMoved.TotalLatency",
	"EventLatency.MousePressed.TotalLatency",
	"EventLatency.GestureScrollUpdate.TotalLatency",
}

var keyInputHistograms = []string{
	"EventLatency.KeyPressed.TotalLatency",
}

// runStep performs the following steps in order.
// 1. Open camera.
// 2. Turn on mic.
// 3. Show camera preview.
// 4. Hold 1:1 (2p) call.
// 5. Hold 9p call.
func runStep(ctx context.Context, conn *chrome.Conn, pr *power.Recorder) error {
	const profileInterval = 50 * time.Second // Sleep interval to measure the performance metrics.

	if err := pr.Start(ctx); err != nil {
		return errors.Wrap(err, "cannot start collecting power metrics")
	}
	checkPoint := pr.StartCheckpoint("idle")
	// GoBigSleepLint: Sleep for profiling idle state.
	if err := testing.Sleep(ctx, profileInterval); err != nil {
		return errors.Wrapf(err, "failed to sleep for %v", profileInterval)
	}
	pr.EndCheckpoint(checkPoint)

	type stepConfig struct {
		name    string
		evalStr string
		errMsg  string
	}
	for _, step := range []stepConfig{
		{"camera", "VC.startCamera()", "failed opening camera"},
		{"audio", "VC.micOn()", "failed turning on mic"},
		{"camera_preview", "VC.showCameraPreview()", "failed showing camera preview"},
		{"video_2p", "VC.holdCall(2)", "failed holding 1:1 (2p) call (VP9 L1T3)"},
		{"video_9p", "VC.holdCall(9)", "failed holding 9p call (VP9 L3T3_KEY)"},
	} {
		testing.ContextLog(ctx, "Starting ", step.name)
		if err := conn.Eval(ctx, step.evalStr, nil); err != nil {
			return errors.Wrap(err, step.errMsg)
		}
		checkPoint := pr.StartCheckpoint(step.name)
		testing.ContextLog(ctx, "Measuring performance metrics ", step.name)
		// GoBigSleepLint: Sleep to measure the performance metrics
		if err := testing.Sleep(ctx, profileInterval); err != nil {
			return errors.Wrapf(err, "failed to sleep for %v", profileInterval)
		}
		pr.EndCheckpoint(checkPoint)
	}
	if err := pr.Finish(ctx); err != nil {
		return errors.Wrap(err, "cannot finish collecting power metrics")
	}
	return nil
}

// runNonStep holds a conference video call in which |numPeople| persons attends
// and thus |numPeople-1| decoders and 1 encoder run.
func runNonStep(ctx context.Context, tconn, bTconn *chrome.TestConn, s *testing.State, conn *chrome.Conn, pr *power.Recorder, params VCTestParams) error {
	const profileInterval = 100 * time.Second // Sleep interval to measure the performance metrics.
	if params.NumPeople <= 1 {
		return errors.Errorf("the number of people must be more than 1: NumPeople=%d", params.NumPeople)
	}
	if err := conn.Eval(ctx, "VC.startCamera()", nil); err != nil {
		return errors.Wrap(err, "failed starting camera")
	}
	if err := conn.Eval(ctx, "VC.micOn()", nil); err != nil {
		return errors.Wrap(err, "failed starting mic")
	}
	if err := conn.Eval(ctx, "VC.showCameraPreview()", nil); err != nil {
		return errors.Wrap(err, "failed showing camera preview")
	}
	if err := conn.Eval(ctx, fmt.Sprintf("VC.holdCall(%d, %t)", params.NumPeople, params.Present), nil); err != nil {
		return errors.Wrapf(err, "failed holding %dp call", params.NumPeople)
	}

	var histNames []string
	if params.Mouse {
		histNames = append(graphicsHistograms, mouseHistograms...)
	} else if params.Text {
		histNames = keyInputHistograms
	}

	if params.Mouse {
		// Active the text input window so that keyboard inputs the text area.
		if err := browser.ActivateTabByTitle(ctx, bTconn, mouseTitle); err != nil {
			return errors.Wrap(err, "failed activating video conference window")
		}
		mouseCtx := ctx
		mouseOpCtx, stopMouseOperating := ctxutil.Shorten(mouseCtx, 1*time.Second)
		err := startMouseOperating(mouseCtx, mouseOpCtx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed starting operating mouse")
		}
		defer stopMouseOperating()
	} else if params.Text {
		// Active the text input window so that keyboard inputs the text area.
		if err := browser.ActivateTabByTitle(ctx, bTconn, textTitle); err != nil {
			return errors.Wrap(err, "failed activating video conference window")
		}
		// typeCtx is shorter than kbdCtx because a keyboard needs to be closed
		// after typeCtx is expired.
		kbdCtx := ctx
		typeCtx, stopTyping := ctxutil.Shorten(kbdCtx, 1*time.Second)
		err := startTyping(kbdCtx, typeCtx)
		if err != nil {
			return errors.Wrap(err, "failed starting text input")
		}
		defer stopTyping()
	} else if params.Present {
		// Capturing a tab activates the captured tab and window. Back to the video conference window.
		if err := browser.ActivateTabByTitle(ctx, bTconn, vcTitle); err != nil {
			return errors.Wrap(err, "failed activating video conference window")
		}
	}

	ownPerfs := perf.NewValues()
	if err := measureWebRTCStats(ctx, conn, ownPerfs, params); err != nil {
		return errors.Wrap(err, "failed collecting webrtc stats")
	}

	const coolDownDuration = 10 * time.Second
	testing.ContextLogf(ctx, "Sleep to eliminate the power effect on the start up (%v)", coolDownDuration)
	// GoBigSleepLint: Sleep to eliminate the power effect on the stat up of the
	// video conference workload and WebRTC stats measurement. The duration, 10
	// seconds, is arbitrary selected and will be changed if necessary.
	if err := testing.Sleep(ctx, coolDownDuration); err != nil {
		return errors.Wrapf(err, "failed to sleep for %v", coolDownDuration)
	}

	var err error
	var startHists []*histogram.Histogram
	if len(histNames) > 0 {
		startHists, err = metrics.GetHistograms(ctx, bTconn, histNames)
		if err != nil {
			return errors.Wrap(err, "failed to get histograms")
		}
	}
	testing.ContextLog(ctx, "Start collecting power metrics")
	if err := pr.Start(ctx); err != nil {
		return errors.Wrap(err, "cannot start collecting power metrics")
	}

	// GoBigSleepLint: Sleep to measure the performance metrics
	if err := testing.Sleep(ctx, profileInterval); err != nil {
		return errors.Wrapf(err, "failed to sleep for %v", profileInterval)
	}

	if _, err := pr.Stop(ctx); err != nil {
		return errors.Wrap(err, "cannot stop collecting power metrics")
	}

	if len(histNames) > 0 {
		endHists, err := metrics.GetHistograms(ctx, bTconn, histNames)
		if err != nil {
			return errors.Wrap(err, "failed to get histograms")
		}
		diffHists, err := histogram.DiffHistograms(startHists, endHists)
		if err != nil {
			return errors.Wrap(err, "get histograms")
		}
		histPerfs, err := chromeHistogramMetrics(diffHists)
		if err != nil {
			return errors.Wrap(err, "compute histogram metrics")
		}
		ownPerfs.Merge(histPerfs)
	}

	if err := pr.Finish(ctx, ownPerfs); err != nil {
		return errors.Wrap(err, "cannot finish collecting power metrics")
	}

	if params.Trace {
		if err := recordTracing(ctx, s.OutDir(), s.DataPath(traceConfigFile)); err != nil {
			return errors.Wrap(err, "failed in tracing")
		}
	}
	return nil
}

func chromeHistogramMetrics(hists []*histogram.Histogram) (*perf.Values, error) {
	// Check histograms is not empty.
	for _, hist := range hists {
		if hist.TotalCount() == 0 {
			return nil, errors.Errorf("empty histogram: %s", hist.Name)
		}
	}
	p := perf.NewValues()
	for _, hist := range hists {
		mean, err := hist.Mean()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to compute mean: %s", hist.Name)
		}
		p.Set(perf.Metric{
			Name:      hist.Name + "_mean",
			Unit:      "us",
			Direction: perf.SmallerIsBetter,
		}, mean)
		percentile99, err := hist.Percentile(99)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to compute mean: %s", hist.Name)
		}
		p.Set(perf.Metric{
			Name:      hist.Name + "_percentile_99",
			Unit:      "us",
			Direction: perf.SmallerIsBetter,
		}, percentile99)
	}
	return p, nil
}

// setupDisplayEnv sets up the display environment for the same state.
// The display is landscape-primary and the shelf is auto-hidden.
// The returned func resets the shelf settings and is to be called at last.
func setupDisplayEnv(ctx context.Context, tconn *chrome.TestConn) (func(context.Context), error) {
	// Rotate the display to landscape-primary.
	if _, err := display.GetInternalInfo(ctx, tconn); err == nil {
		if err = graphics.RotateDisplayToLandscapePrimary(ctx, tconn); err != nil {
			return nil, errors.Wrap(err, "failed to set display to landscape-primary orientation")
		}
	}
	// Set shelf to auto-hide.
	dispInfo, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get primary display info")
	}
	origShelfBehavior, err := ash.GetShelfBehavior(ctx, tconn, dispInfo.ID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get shelf behavior")
	}
	if err := ash.SetShelfBehavior(ctx, tconn, dispInfo.ID, ash.ShelfBehaviorAlwaysAutoHide); err != nil {
		return nil, errors.Wrap(err, "failed to set shelf behavior to Auto Hide")
	}
	return func(cleanupCtx context.Context) {
		ash.SetShelfBehavior(cleanupCtx, tconn, dispInfo.ID, origShelfBehavior)
	}, nil
}

// prepareWindowView maximizes the video conference window if only the window exists, or
// snap windows of the video conference window and the presentation/text window.
func prepareWindowView(ctx context.Context, tconn *chrome.TestConn, newWinTitle string) error {

	vcWin, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, vcTitle)
	if err != nil {
		return errors.Wrap(err, "failed to get the video conference window")
	}

	// Maximize the video conference window if we don't open a new window
	if newWinTitle == "" {
		if err := ash.SetWindowStateAndWait(ctx, tconn, vcWin.ID, ash.WindowStateMaximized); err != nil {
			return errors.Wrap(err, "failed to maximize the video conference window")
		}
		return nil
	}

	// Snap a video conference window and a presentation/text window.
	if err := ash.SetWindowStateAndWait(ctx, tconn, vcWin.ID, ash.WindowStatePrimarySnapped); err != nil {
		return errors.Wrap(err, "failed to set up the video conference window")
	}

	newWin, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, newWinTitle)
	if err != nil {
		return errors.Wrap(err, "failed to get the text/presentation window")
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, newWin.ID, ash.WindowStateSecondarySnapped); err != nil {
		return errors.Wrap(err, "failed to set up the text/presentation window")
	}

	return nil
}

func runVCPerf(ctx context.Context, cs ash.ConnSource, tconn, bTconn *chrome.TestConn, s *testing.State, vcURL, newWinURL string, params VCTestParams) error {
	closeCtx := ctx

	// Reserve time for closing tab and cleaning up a power library.
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer cancel()

	if err := setUpAudio(ctx, params); err != nil {
		return errors.Wrap(err, "setUpAudio")
	}

	resetDisplay, err := setupDisplayEnv(ctx, tconn)
	if err != nil {
		return err
	}
	defer resetDisplay(closeCtx)

	r := power.NewRecorder(ctx, powerInterval, s.OutDir(), s.TestName())
	defer r.Close(closeCtx)

	if err := r.UseMetrics(ctx,
		pm.CPUIdleStateClass,
		pm.MemoryClass,
		pm.PackageCStatesClass,
		pm.ProcfsCPUClass,
		pm.RAPLPowerClass,
		pm.SysfsBatteryClass); err != nil {
		return err
	}

	conn, err := cs.NewConn(ctx, vcURL)
	if err != nil {
		return errors.Wrapf(err, "failed to open %s", vcURL)
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	if err := conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		return errors.Wrap(err, "timed out waiting for page loading")
	}

	var newWinTitle string
	var newWinStartUp func() error
	if params.Present || params.Text || params.Mouse {
		// Opens a new window for presentation or text input.
		newWinConn, err := cs.NewConn(ctx, newWinURL, browser.WithNewWindow())
		if err != nil {
			return errors.Wrapf(err, "failed to open %s", newWinURL)
		}
		defer newWinConn.Close()
		defer newWinConn.CloseTarget(ctx)

		newWinTitle = presentTitle
		if params.Text {
			newWinTitle = textTitle
		} else if params.Mouse {
			newWinTitle = mouseTitle
		}
		if err := newWinConn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
			return errors.Wrap(err, "timed out waiting for page loading")
		}

		if newWinTitle == presentTitle {
			newWinStartUp = func() error {
				return newWinConn.Eval(ctx, "drawCanvasAlternatingColours(1280, 720, 30)", nil)
			}
		}
	}

	if err := prepareWindowView(ctx, tconn, newWinTitle); err != nil {
		return err
	}

	if err := cpu.Cooldown(ctx); err != nil {
		return errors.Wrap(err, "failed waiting for CPU to cool down")
	}
	if newWinStartUp != nil {
		if err := newWinStartUp(); err != nil {
			return errors.Wrap(err, "failed to start the presentation window")
		}
	}
	if params.Step {
		return runStep(ctx, conn, r)
	}
	return runNonStep(ctx, tconn, bTconn, s, conn, r, params)
}

// setUpAudio configures the audio server according to p.
func setUpAudio(ctx context.Context, p VCTestParams) error {
	if p.NoiseCancellation {
		if err := dlc.Install(ctx, "nc-ap-dlc", ""); err != nil {
			return errors.Wrap(err, "cannot install nc-ap-dlc")
		}
	}
	cras, err := audio.RestartCras(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot restart CRAS")
	}

	if err := audio.SelectIODevices(ctx, cras, "INTERNAL_MIC", "INTERNAL_SPEAKER"); err != nil {
		return errors.Wrap(err, "audio.SelectIODevices")
	}

	if err := cras.SetNoiseCancellationEnabled(ctx, p.NoiseCancellation); err != nil {
		return errors.Wrap(err, "cras.SetNoiseCancellationEnabled")
	}

	if err := cras.SetStyleTransferEnabled(ctx, p.StyleTransfer); err != nil {
		return errors.Wrap(err, "cras.SetStyleTransferEnabled")
	}
	return nil
}

// RunVideoConference runs a video conference using WebRTC API and measures the
// performance metrics while enabling features in order.
func RunVideoConference(ctx context.Context, cs ash.ConnSource, tconn, bTConn *chrome.TestConn, s *testing.State, params VCTestParams) error {
	const cleanupTime = 5 * time.Second

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()
	vcURL := server.URL + "/" + vcHTML
	newWinURL := server.URL + "/" + presentHTML
	if params.Text {
		newWinURL = server.URL + "/" + textHTML
	} else if params.Mouse {
		newWinURL = server.URL + "/" + mouseHTML
	}

	ctx, cancel := ctxutil.Shorten(ctx, cleanupTime)
	defer cancel()

	if err := runVCPerf(ctx, cs, tconn, bTConn, s, vcURL, newWinURL, params); err != nil {
		return err
	}

	return nil
}

func recordTracing(ctx context.Context, outDir, configFile string) error {
	const (
		tracingInterval = 15 * time.Second // Sleep interval to conduct tracing.
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	testing.ContextLog(ctx, "Start tracing")
	session, err := tracing.StartSession(ctx, configFile,
		tracing.WithTraceDataPath(
			filepath.Join(
				outDir,
				fmt.Sprintf("perfetto-%d.pb", time.Now().Unix()))),
		tracing.WithCompression())
	if err != nil {
		return errors.Wrap(err, "failed to start tracing")
	}
	defer session.Finalize(cleanupCtx)
	// Stop tracing even if context deadline exceeds during sleep.
	stopped := false
	defer func() {
		if !stopped {
			session.Stop()
		}
	}()
	// GoBigSleepLint: sleep to collect tracing events
	if err := testing.Sleep(ctx, tracingInterval); err != nil {
		return errors.Wrap(err, "failed to sleep to wait for the tracing session")
	}
	stopped = true
	if err := session.Stop(); err != nil {
		return errors.Wrap(err, "failed to stop tracing")
	}
	testing.ContextLog(ctx, "Complete tracing")
	return nil
}

// placeHolderValidateFrame is a place holder function passed to webrtc.MeasureRTCDecodeStats().
func placeHolderValidateFrame(ctx context.Context, conn *chrome.Conn, width, height int) error {
	return nil
}

// readRTCReport returns a function to read WebRTC stats for an |id| decoder and encoder.
func readRTCReport(id int, displayCapture bool) webrtc.ReadRTCReportFunc {
	return func(ctx context.Context, conn *chrome.Conn, decode bool, out interface{}) error {
		// Decode: localPeerConnection, "outbound-rtp"
		// Encode: remotePeerConnection, "inbound-rtp"
		var peerConnection string
		var staticType string
		if displayCapture {
			peerConnection = "VC.displayLocalPC"
			staticType = "outbound-rtp"
		} else {
			if decode {
				peerConnection = fmt.Sprintf("VC.remotePCs[%d]", id)
				staticType = "inbound-rtp"
			} else {
				peerConnection = fmt.Sprintf("VC.localPCs[%d]", id)
				staticType = "outbound-rtp"
			}
		}
		return conn.Call(ctx, out, fmt.Sprintf(`async() => {
			const peerConnection = %s;
			const stats = await peerConnection.getStats(null);
			if (stats == null) {
			  throw new Error("getStats() failed");
			}
			var R = null;
			for (const [_, report] of stats) {
			  if (report['type'] === '%s' &&
				 (!R || R['frameHeight'] < report['frameHeight'])) {
				R = report;
			  }
			}
			if (R !== null) {
			  return R;
			}
			throw new Error("Stat not found");
		  }`, peerConnection, staticType))
	}
}

// measureWebRTCStats records the webrtc decoder and encoder performance metrics from webrtc stats.
func measureWebRTCStats(ctx context.Context, conn *chrome.Conn, rtcPerf *perf.Values, params VCTestParams) error {
	type resolution struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	var cameraResolution resolution
	if err := conn.Call(ctx, &cameraResolution, "() => { return VC.getCameraResolution(); }", nil); err != nil {
		return errors.Wrap(err, "failed getting a camera resolution")
	}
	testing.ContextLog(ctx, "Camera resolution: ", cameraResolution)

	type encoderConfig struct {
		Codec           string `json:"codec"`
		InputHeight     int    `json:"inputHeight"`
		OutputHeight    int    `json:"outputHeight"`
		ScalabilityMode string `json:"scalabilityMode"`
	}
	var encCfg encoderConfig
	if err := conn.Call(ctx, &encCfg, fmt.Sprintf("() => { return VC.getEncoderConfig(%d); }", params.NumPeople), nil); err != nil {
		return errors.Wrap(err, "failed getting a camera resolution")
	}
	testing.ContextLogf(ctx, "Video encoder config: %s, %dp, %s, scaleResolutionDownBy: %.2f (=%d/%d)",
		encCfg.Codec,
		encCfg.OutputHeight, encCfg.ScalabilityMode,
		float32(encCfg.InputHeight)/float32(encCfg.OutputHeight), encCfg.InputHeight, encCfg.OutputHeight)

	videoHeight := encCfg.OutputHeight
	videoWidth := videoHeight * 16 / 9
	readCodecPC := fmt.Sprintf("VC.localPCs[%d]", params.NumPeople-2)
	if err := webrtc.WaitForPeerConnectionStabilized(ctx, conn, encCfg.Codec, videoWidth, videoHeight, false, encCfg.ScalabilityMode, readRTCReport(params.NumPeople-2, false), webrtc.CreateReadCodecFunc(readCodecPC)); err != nil {
		return err
	}

	if params.Present {
		var dcResolution resolution
		if err := conn.Call(ctx, &dcResolution, "() => { return VC.getDisplayCaptureResolution(); }", nil); err != nil {
			return errors.Wrap(err, "failed getting a display capture resolution")
		}
		testing.ContextLog(ctx, "Display capture resolution: ", dcResolution)
	}

	const webRTCCoolDownDuration = 5 * time.Second
	testing.ContextLogf(ctx, "Sleep to eliminate the performance effect of rtc peer connection start up(%v)", webRTCCoolDownDuration)
	// GoBigSleepLint: Sleep to eliminate the performance effect on the start
	// up of rtc peer connection. The duration, 5 seconds, is arbitrary
	// selected and will be changed if necessary.
	if err := testing.Sleep(ctx, webRTCCoolDownDuration); err != nil {
		return errors.Wrapf(err, "failed to sleep for %v", webRTCCoolDownDuration)
	}
	var wg sync.WaitGroup
	cameraEncPerf := perf.NewValues()
	var statErrs = make([]error, params.NumPeople-1)
	var decPerfs = make([]*perf.Values, params.NumPeople-1)
	for i := 0; i < params.NumPeople-1; i++ {
		decPerfs[i] = perf.NewValues()
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			readRTCReportFunc := readRTCReport(i, false)
			if i == params.NumPeople-2 {
				var err error
				err = webrtc.MeasureRTCEncodeStats(ctx, conn, readRTCReportFunc, cameraEncPerf)
				if err != nil {
					statErrs[i] = err
					return
				}

			}
			if err := webrtc.MeasureRTCDecodeStats(ctx, conn, videoWidth, videoHeight, readRTCReportFunc, placeHolderValidateFrame, decPerfs[i]); err != nil {
				statErrs[i] = err
				return
			}
		}(i)
	}

	var presentStatErr error
	presentEncPerf := perf.NewValues()
	if params.Present {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			if err = webrtc.MeasureRTCEncodeStats(ctx, conn, readRTCReport(0, true), presentEncPerf); err != nil {
				presentStatErr = err
				return
			}
		}()
	}
	wg.Wait()
	for i, decPerf := range decPerfs {
		if statErrs[i] != nil {
			return statErrs[i]
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("decPerf[%d]: ", i))
		j := 0
		for metric, values := range decPerf.GetValues() {
			if j != 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(fmt.Sprintf("{%s: %v}", metric.Name, values))
			j++
		}
		testing.ContextLog(ctx, sb.String())
	}
	sort.Slice(decPerfs, func(i, j int) bool {
		avgDecodeTime := func(p *perf.Values) float64 {
			var decodeTimes []float64
			for metric, values := range p.GetValues() {
				if metric.Name == "rx.decode_time" {
					decodeTimes = values
					break
				}
			}
			var sum float64
			for _, d := range decodeTimes {
				sum += d
			}
			return sum / float64(len(decodeTimes))
		}

		return avgDecodeTime(decPerfs[i]) < avgDecodeTime(decPerfs[j])
	})
	medDecPerf := decPerfs[len(decPerfs)/2]
	rtcPerf.MergeWithSuffix("_median_dec", medDecPerf)
	if presentStatErr != nil {
		return presentStatErr
	}
	rtcPerf.MergeWithSuffix("_camera_enc", cameraEncPerf)
	rtcPerf.MergeWithSuffix("_present_enc", presentEncPerf)
	return nil
}

// startTyping continues typing until the typeCtx is cancelled or expired.
func startTyping(kbdCtx, typeCtx context.Context) error {
	kb, err := input.Keyboard(kbdCtx)
	if err != nil {
		return errors.Wrap(err, "failed to find keyboard")
	}

	go func() {
		i := 0
		var typingStrs = []string{
			"1234567890 ", "1234567891 ", "1234567892 ", "1234567893 ", "1234567894 \n",
			"1234567895 ", "1234567896 ", "1234567897 ", "1234567898 ", "1234567899 \n",
		}
		for {
			select {
			case <-typeCtx.Done():
				kb.Close(kbdCtx)
				return
			default:
				kb.Type(typeCtx, typingStrs[i%len(typingStrs)])
			}

			// GoBigSleepLint: Sleep between typing text to not input too many texts.
			if err := testing.Sleep(typeCtx, typingInterval); err != nil {
				testing.ContextLog(kbdCtx, "Failed in sleep")
				break
			}

			i++
		}
	}()

	return nil
}

func startMouseOperating(mouseCtx, mouseOpCtx context.Context, tconn *chrome.TestConn) error {
	type mouseOpType int
	const (
		drag mouseOpType = iota
		move
		click
		scroll
		mouseOpTypeMax
	)

	mouseWin, err := ash.WaitForAnyWindowWithTitle(mouseCtx, tconn, mouseTitle)
	if err != nil {
		return errors.Wrap(err, "failed to get the mouse page window")
	}
	centerWinCoord := mouseWin.BoundsInRoot.CenterPoint()
	// The operations except scrolling are executed with uiaout.mouse that invokes
	// a mouse event by autotest private API.
	// First move to the center of "mouse test" window.
	if err := mouse.Move(tconn, centerWinCoord, 0*time.Second)(mouseOpCtx); err != nil {
		return errors.Wrap(err, "failed to move a mouse to center of mouse page")
	}
	// Creates input.Mouse for scrolling operations.
	mw, err := input.Mouse(mouseCtx)
	if err != nil {
		return errors.Wrap(err, "failed to get a virtual mouse")
	}

	go func() {
		i := 0
		for {
			select {
			case <-mouseOpCtx.Done():
				mw.Close(mouseCtx)
				return
			default:
				switch mouseOpType(i % int(mouseOpTypeMax)) {
				case drag:
					const pixelsX, pixelsY = 100, 100
					if err := mouse.Drag(tconn, centerWinCoord, centerWinCoord.Add(coords.NewPoint(pixelsX, pixelsY)), 1*time.Second)(mouseOpCtx); err != nil {
						testing.ContextLog(mouseCtx, "Failed to drag: ", err)
						return
					}
				case move:
					// First move to the center of "mouse test" window.
					if err := mouse.Move(tconn, centerWinCoord, 1*time.Second)(mouseOpCtx); err != nil {
						testing.ContextLog(mouseCtx, "Failed to move: ", err)
						return
					}
				case click:
					if err := mouse.Press(tconn, mouse.LeftButton)(mouseOpCtx); err != nil {
						testing.ContextLog(mouseCtx, "Failed to press: ", err)
						return
					}
					// GoBigSleepLint: Sleep between mouse press and release.
					if err := testing.Sleep(mouseOpCtx, 50*time.Millisecond); err != nil {
						testing.ContextLog(mouseCtx, "Failed in sleep")
						return
					}
					if err := mouse.Release(tconn, mouse.LeftButton)(mouseOpCtx); err != nil {
						testing.ContextLog(mouseCtx, "Failed to press: ", err)
						return
					}
				case scroll:
					for _, scroll := range [](func() error){
						mw.ScrollDown,
						mw.ScrollUp,
					} {
						for i := 0; i < 20; i++ {
							if err := scroll(); err != nil {
								testing.ContextLog(mouseCtx, "Failed to scroll: ", err)
								return
							}
							// GoBigSleepLint: Sleep during scrolling operation
							if err := testing.Sleep(mouseOpCtx, 50*time.Millisecond); err != nil {
								testing.ContextLog(mouseCtx, "Failed in sleep")
								return
							}
						}
					}
				}
			}

			// GoBigSleepLint: Sleep between operating mouse to not operate mouse so much
			if err := testing.Sleep(mouseOpCtx, mouseOperatingInterval); err != nil {
				testing.ContextLog(mouseCtx, "Failed in sleep")
				return
			}

			i++
		}
	}()

	return nil
}
