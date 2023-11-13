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
	"strconv"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/media/webrtc"
	"go.chromium.org/tast-tests/cros/local/power"
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
	// Numpeope is the number of persons in a video conference.
	// This can be set only if Step is false and it must be more than two.
	NumPeople int
	// If Trace is true, then perfetto tracing is executed and the tracing
	// result is saved in the result directory.
	Trace bool
	// If Present is true, then opens a tab in background and capture the tab.
	// One more encoder for the tab capture will run, but no decoder runs for it.
	Present bool
}

const (
	// vcHTML is the HTML holding a video conference using WebRTC API.
	vcHTML = "webrtc/video_conference.html"
	// presentHTML is the HTML changing the content in 30fps for presentation.
	presentHTML = "webrtc/presentation.html"

	// Peretto configuration file.
	traceConfigFile = "webrtc/perfetto_trace.txtpb"

	powerInterval = 5 * time.Second // Power library metrics collection interval.
)

// TestFiles returns the files required running the test.
func TestFiles() []string {
	return []string{
		vcHTML,
		presentHTML,
		"webrtc/canvas_animation.js",
		"webrtc/video_conference.js",
		"webrtc/third_party/munge_sdp.js",
		traceConfigFile,
	}
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
	// GoBigSleepLint: Sleep for profiling idle state.
	if err := testing.Sleep(ctx, profileInterval); err != nil {
		return errors.Wrapf(err, "failed to sleep for %v", profileInterval)
	}

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
		testing.ContextLog(ctx, "Measuring performance metrics ", step.name)
		// GoBigSleepLint: Sleep to measure the performance metrics
		if err := testing.Sleep(ctx, profileInterval); err != nil {
			return errors.Wrapf(err, "failed to sleep for %v", profileInterval)
		}
	}
	if err := pr.Finish(ctx); err != nil {
		return errors.Wrap(err, "cannot finish collecting power metrics")
	}
	return nil
}

// runNonStep holds a conference video call in which |numPeople| persons attends
// and thus |numPeople-1| decoders and 1 encoder run.
func runNonStep(ctx context.Context, s *testing.State, tconn *chrome.TestConn, conn *chrome.Conn, pr *power.Recorder, presentURL string, params VCTestParams) error {
	const profileInterval = 100 * time.Second // Sleep interval to measure the performance metrics.
	if params.NumPeople <= 1 {
		return errors.Errorf("the number of people must be more than 1: NumPeople=%d", params.NumPeople)
	}
	if err := conn.Eval(ctx, "VC.micOn()", nil); err != nil {
		return errors.Wrap(err, "failed start camera and mic")
	}
	if err := conn.Eval(ctx, "VC.showCameraPreview()", nil); err != nil {
		return errors.Wrap(err, "failed showing camera preview")
	}
	if err := conn.Eval(ctx, fmt.Sprintf("VC.holdCall(%d, %t)", params.NumPeople, params.Present), nil); err != nil {
		return errors.Wrapf(err, "failed holding %dp call", params.NumPeople)
	}

	if params.Present {
		// Capturing a tab activates the captured tab. Back to the video conference tab.
		if err := browser.ActivateTabByTitle(ctx, tconn, "WebRTC VideoConference"); err != nil {
			return errors.Wrap(err, "failed activating video conference tab")
		}
	}

	rtcPerf := perf.NewValues()
	if err := measureWebRTCStats(ctx, conn, rtcPerf, params); err != nil {
		return errors.Wrap(err, "failed collecting webrtc stats")
	}

	if err := pr.Start(ctx); err != nil {
		return errors.Wrap(err, "cannot start collecting power metrics")
	}

	// GoBigSleepLint: Sleep to measure the performance metrics
	if err := testing.Sleep(ctx, profileInterval*2); err != nil {
		return errors.Wrapf(err, "failed to sleep for %v", profileInterval)
	}

	if err := pr.Finish(ctx, rtcPerf); err != nil {
		return errors.Wrap(err, "cannot finish collecting power metrics")
	}
	if params.Trace {
		if err := recordTracing(ctx, s.OutDir(), s.DataPath(traceConfigFile)); err != nil {
			return errors.Wrap(err, "failed in tracing")
		}
	}
	return nil
}

func runVCPerf(ctx context.Context, cr *chrome.Chrome, s *testing.State, vcURL, presentURL string, params VCTestParams) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to test API")
	}
	if _, err := display.GetInternalInfo(ctx, tconn); err == nil {
		if err = graphics.RotateDisplayToLandscapePrimary(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to set display to landscape-primary orientation")
		}
	}

	closeCtx := ctx

	// Reserve time for closing tab and cleaning up a power library.
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer cancel()

	r := power.NewRecorder(ctx, powerInterval, s.OutDir(), s.TestName())
	defer r.Close(closeCtx)

	conn, err := cr.NewConn(ctx, vcURL)
	if err != nil {
		return errors.Wrapf(err, "failed to open %s", vcURL)
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	if params.Present {
		// Opens a presentation tab in background.
		presentConn, err := cr.NewBackgroundConn(ctx, presentURL)
		if err != nil {
			return errors.Wrapf(err, "failed to open %s", presentURL)
		}
		defer presentConn.Close()
		defer presentConn.CloseTarget(ctx)
	}

	// Maximize window size so that the window size to be captured is maximized
	// and also camera and display capturing is executed in the same situation.
	if err := ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
		return ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized)
	}); err != nil {
		return errors.Wrap(err, "failed to maximize window")
	}

	if err := conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		return errors.Wrap(err, "timed out waiting for page loading")
	}

	if params.Step {
		return runStep(ctx, conn, r)
	}
	return runNonStep(ctx, s, tconn, conn, r, presentURL, params)
}

// RunVideoConference runs a video conference using WebRTC API and measures the
// performance metrics while enabling features in order.
func RunVideoConference(ctx context.Context, cr *chrome.Chrome, s *testing.State, params VCTestParams) error {
	const cleanupTime = 5 * time.Second

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()
	vcURL := server.URL + "/" + vcHTML
	presentURL := server.URL + "/" + presentHTML

	ctx, cancel := ctxutil.Shorten(ctx, cleanupTime)
	defer cancel()

	if err := runVCPerf(ctx, cr, s, vcURL, presentURL, params); err != nil {
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
		InputHeight     int    `json:"inputHeight"`
		OutputHeight    int    `json:"outputHeight"`
		ScalabilityMode string `json:"scalabilityMode"`
	}
	var encCfg encoderConfig
	if err := conn.Call(ctx, &encCfg, fmt.Sprintf("() => { return VC.getEncoderConfig(%d); }", params.NumPeople), nil); err != nil {
		return errors.Wrap(err, "failed getting a camera resolution")
	}
	testing.ContextLogf(ctx, "Video encoder config: %dp %s , scaleResolutionDownBy: %.2f (=%d/%d)",
		encCfg.OutputHeight, encCfg.ScalabilityMode,
		float32(encCfg.InputHeight)/float32(encCfg.OutputHeight), encCfg.InputHeight, encCfg.OutputHeight)

	videoHeight := encCfg.OutputHeight
	videoWidth := videoHeight * 16 / 9
	if err := webrtc.WaitForPeerConnectionStabilized(ctx, conn, videoWidth, videoHeight, false, readRTCReport(params.NumPeople-2, false)); err != nil {
		return err
	}
	var wg sync.WaitGroup
	var ps = make([]*perf.Values, params.NumPeople-1)
	var statErrs = make([]error, params.NumPeople-1)
	for i := 0; i < params.NumPeople-1; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			framesEncoded := 0
			readRTCReportFunc := readRTCReport(i, false)
			if i == params.NumPeople-2 {
				p := perf.NewValues()
				var err error
				framesEncoded, err = webrtc.MeasureRTCEncodeStats(ctx, conn, readRTCReportFunc, p)
				if err != nil {
					statErrs[i] = err
					return
				}
				rtcPerf.MergeWithSuffix("_camera", p)
			}
			p := perf.NewValues()
			if err := webrtc.MeasureRTCDecodeStats(ctx, conn, framesEncoded, videoWidth, videoHeight, readRTCReportFunc, placeHolderValidateFrame, p); err != nil {
				statErrs[i] = err
				return
			}
		}(i)
	}
	var presentStatErr error
	if params.Present {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			p := perf.NewValues()
			if _, err = webrtc.MeasureRTCEncodeStats(ctx, conn, readRTCReport(0, true), p); err != nil {
				presentStatErr = err
				return
			}
			rtcPerf.MergeWithSuffix("_present", p)
		}()
	}
	wg.Wait()
	for i, p := range ps {
		if statErrs[i] != nil {
			return statErrs[i]
		}
		rtcPerf.MergeWithSuffix("_"+strconv.Itoa(i), p)
	}
	if presentStatErr != nil {
		return presentStatErr
	}
	return nil
}
