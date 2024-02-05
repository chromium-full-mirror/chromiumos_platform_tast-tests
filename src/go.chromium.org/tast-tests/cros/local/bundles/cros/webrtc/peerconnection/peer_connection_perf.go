// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Provides code for WebRTC's RTCPeerConnection performance tests.

package peerconnection

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/graphics"
	mediacpu "go.chromium.org/tast-tests/cros/local/media/cpu"
	"go.chromium.org/tast-tests/cros/local/media/webrtc"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// Time to measure CPU usage.
	cpuMeasuring = 30 * time.Second
	// Time to measure GPU usage counters.
	gpuMeasuring = 10 * time.Second
)

func validateFrame(ctx context.Context, conn *chrome.Conn, width, height int) error {
	var isBlackFrame bool
	if err := conn.Call(ctx, &isBlackFrame, "isBlackVideoFrame", width, height); err != nil {
		return errors.Wrap(err, "isBlackVideoFrame() JS failed")
	}
	if isBlackFrame {
		return errors.New("last displayed frame is black")
	}

	var isFrozenFrame bool
	if err := conn.Call(ctx, &isFrozenFrame, "isFrozenVideoFrame", width, height); err != nil {
		return errors.Wrap(err, "isFrozenVideoFrameJS() JS failed")
	}
	if isFrozenFrame {
		return errors.New("last displayed frame is frozen")
	}
	return nil
}

// setupRTCPeerConnectionTest sets up the test page, |videoURL| and waits until
// CPU is ready for the performance test, it fixes the display to landscape-primary
// and maximizes the browser window. When the browser is not suitable for display capture
// test (e.g. the monitor is too small), it returns false in the first value to indicate
// that the test should be skipped.
func setupRTCPeerConnectionTest(ctx context.Context, conn *chrome.Conn, tconn *chrome.TestConn, params RTCTestParams, videoURL string) (bool, error) {
	// For consistency across test runs, let's try to put the UI in a known state:
	// rotate the display to landscape-primary and maximize the browser window
	// (setupCapture() maximizes the window).
	if _, err := display.GetInternalInfo(ctx, tconn); err == nil {
		if err = graphics.RotateDisplayToLandscapePrimary(ctx, tconn); err != nil {
			return false, errors.Wrap(err, "failed to set display to landscape-primary orientation")
		}
	}
	if canCapture, err := setupCapture(ctx, conn, tconn, params.DisplayMediaType, params.StreamWidth, params.StreamHeight); err != nil {
		return false, errors.Wrap(err, "failed to setup capture")
	} else if !canCapture {
		return false, nil
	}

	if err := conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		return false, errors.Wrap(err, "timed out waiting for page loading")
	}

	if err := cpu.Cooldown(ctx); err != nil {
		return false, errors.Wrap(err, "failed waiting for CPU to cool down")
	}

	if params.DisplayMediaType != "" {
		// We would like to stabilize the display capture frequency to 30. We
		// achieve this by changing the canvas content in the captured page by
		// WebGL. The canvas change frequency needs to more than 30 so that the
		// timing issue between display capture and canvas change is mitigated.
		// 40 fps seems to be sufficient.
		const (
			canvasWidth     = 1280
			canvasHeight    = 720
			canvasChangeFPS = 40
		)
		if err := conn.Call(ctx, nil, "drawCanvasAlternatingColours", canvasWidth, canvasHeight, canvasChangeFPS); err != nil {
			return false, errors.Wrap(err, "fail in drawCanvasAlternatingColours")
		}
	}

	if params.VideoGridDimension > 1 {
		if err := conn.Call(ctx, nil, "makeVideoGrid", params.VideoGridDimension, videoURL); err != nil {
			return false, errors.Wrap(err, "javascript error")
		}
	}
	return true, nil
}

// runPeerConnectionAndVerifyImplementation kicks off the RTC PeerConnection.
// It verifies the implementation of the running video decoders and encoders are
// expected ones.
func runPeerConnectionAndVerifyImplementation(ctx context.Context, conn *chrome.Conn, params RTCTestParams) error {
	if isSMode(params.Svc) {
		if err := conn.Call(ctx, nil, "startSMode", params.Profile, params.StreamWidth, params.StreamHeight, params.Svc); err != nil {
			return errors.Wrap(err, "error establishing connection")
		}

	} else {
		if err := conn.Call(ctx, nil, "start", params.Profile, params.StreamWidth, params.StreamHeight, params.Simulcasts, params.Svc, params.DisplayMediaType); err != nil {
			return errors.Wrap(err, "error establishing connection")
		}
	}

	return verifyCodecImplementation(ctx, conn, params.VerifyDecoderMode, params.VerifyEncoderMode, params.Svc, params.SimulcastHWEncs)
}

// measurePerformance measures the webrtc stats and system performance, and records
// the results to the perf.Values.
func measurePerformance(ctx context.Context, s *testing.State, conn *chrome.Conn, tconn *chrome.TestConn, params RTCTestParams, p *perf.Values) error {
	pcID := 0
	if isSMode(params.Svc) {
		numStreams, err := numSModeLayers(params.Svc)
		if err != nil {
			return err
		}
		// Collect the performance of only the decoder for the largest resolution stream.
		// TODO(bugs.webrtc.org/15795): After each decoder decodes different
		// resolution stream from each eother, collect all the decoders' performance.
		pcID = numStreams - 1
	}

	if err := webrtc.MeasureRTCStats(ctx, conn, params.StreamWidth, params.StreamHeight, params.DisplayMediaType != "", readRTCReport(pcID), validateFrame, p); err != nil {
		return errors.Wrap(err, "failed to measure RTCStats")
	}

	var gpuErr, i915IRQErr, cStateErr, cpuErr, batErr error
	var wg sync.WaitGroup
	wg.Add(5)
	go func() {
		defer wg.Done()
		gpuErr = graphics.MeasureGPUCounters(ctx, gpuMeasuring, p)
	}()
	go func() {
		defer wg.Done()
		i915IRQErr = graphics.MeasureI915IRQs(ctx, gpuMeasuring, p)
	}()
	go func() {
		defer wg.Done()
		cStateErr = graphics.MeasurePackageCStateCounters(ctx, gpuMeasuring, p)
	}()
	go func() {
		defer wg.Done()
		cpuErr = graphics.MeasureCPUUsageAndPower(ctx, 0 /*stabilization*/, cpuMeasuring, p)
	}()
	go func() {
		defer wg.Done()
		batErr = graphics.MeasureSystemPowerConsumption(ctx, tconn, cpuMeasuring, p)
	}()

	wg.Wait()
	if gpuErr != nil {
		return errors.Wrap(gpuErr, "failed to measure GPU counters")
	}
	if i915IRQErr != nil {
		return errors.Wrap(i915IRQErr, "failed to measure i915 IRQs/s")
	}
	if cStateErr != nil {
		return errors.Wrap(cStateErr, "failed to measure Package C-State residency")
	}
	if cpuErr != nil {
		return errors.Wrap(cpuErr, "failed to measure CPU/Package power")
	}
	if batErr != nil {
		return errors.Wrap(batErr, "failed to measure system power consumption")
	}

	resolutions, err := encoderResolutions(params.StreamWidth, params.StreamHeight, params.Simulcasts, params.Svc)
	if err != nil {
		return errors.Wrap(err, "failed to get expected resolutions")
	}
	if traceErr := measureChromeTraceEvents(ctx, s, resolutions, p); traceErr != nil {
		return errors.Wrap(traceErr, "failed to measure decoding/encoding chrome trace events")
	}

	return nil
}

// peerConnectionPerf opens a WebRTC Loopback connection and streams while collecting
// statistics. If |params.videoGridDimension| is larger than 1, then the real time <video>
// is plugged into a |params.videoGridDimension| x |params.videoGridDimension| grid with copies
// of videoURL being played, similar to a mosaic video call.
func peerConnectionPerf(ctx context.Context, cs ash.ConnSource, cr *chrome.Chrome,
	s *testing.State, loopbackURL, videoURL string, params RTCTestParams, p *perf.Values) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to test API")
	}

	// Reserve one second for closing tab.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	// The page repeatedly plays a loopback video stream.
	// To stop it, we defer conn.CloseTarget() to close the tab.
	conn, err := cs.NewConn(ctx, loopbackURL)
	if err != nil {
		return errors.Wrapf(err, "failed to open %s", loopbackURL)
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	canCapture, err := setupRTCPeerConnectionTest(ctx, conn, tconn, params, videoURL)
	if err != nil || !canCapture {
		return err
	}

	if err := runPeerConnectionAndVerifyImplementation(ctx, conn, params); err != nil {
		return err
	}

	if err := measurePerformance(ctx, s, conn, tconn, params, p); err != nil {
		return err
	}

	testing.ContextLogf(ctx, "Metric: %+v", p)
	return nil
}

func encoderResolutions(width, height, simulcasts int, svc string) ([]graphics.Size, error) {

	streams := 1
	if simulcasts > 1 {
		streams = simulcasts
	}
	if len(svc) >= 2 {
		switch svc[:2] {
		case "L1":
			streams = 1
		case "L2", "S2":
			streams = 2
		case "L3", "S3":
			streams = 3
		default:
			return nil, errors.Errorf("unknown SVC = %s", svc)
		}
	}
	var resolutions []graphics.Size
	for i := 1; i <= streams; i++ {
		w := width >> (streams - i)
		h := height >> (streams - i)
		resolutions = append(resolutions, graphics.Size{Width: w, Height: h})
	}
	return resolutions, nil
}

// RunRTCPeerConnectionPerf starts a Chrome instance (with or without hardware video decoder and encoder),
// opens a WebRTC loopback page and collects performance measures in p.
func RunRTCPeerConnectionPerf(ctx context.Context, cs ash.ConnSource, cr *chrome.Chrome,
	s *testing.State, params RTCTestParams) error {
	// Time reserved for cleanup.
	const cleanupTime = 5 * time.Second

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()
	loopbackURL := server.URL + "/" + LoopbackFile

	cleanUpBenchmark, err := mediacpu.SetUpBenchmark(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to set up CPU benchmark")
	}
	defer cleanUpBenchmark(ctx)

	// Leave a bit of time to tear down benchmark mode.
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTime)
	defer cancel()

	var videoGridURL string
	if params.VideoGridDimension > 1 {
		videoGridURL = server.URL + "/" + params.VideoGridFile
	}
	p := perf.NewValues()
	if err := peerConnectionPerf(ctx, cs, cr, s, loopbackURL, videoGridURL, params, p); err != nil {
		return err
	}

	p.Save(s.OutDir())
	return nil
}
