// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package peerconnection provides common code for webrtc.* RTCPeerConnection tests.
package peerconnection

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/media/logging"
	"go.chromium.org/tast-tests/cros/local/media/oop"
	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// VerifyDecoderMode is how to verify decoder implementation.
type VerifyDecoderMode int

const (
	// VerifyHWDecoderUsed refers to WebRTC hardware accelerated video decoding.
	VerifyHWDecoderUsed VerifyDecoderMode = iota
	// VerifySWDecoderUsed refers to WebRTC software video decoding.
	VerifySWDecoderUsed
	// NoVerifyDecoderMode means it doesn't matter if WebRTC uses any video decoder.
	NoVerifyDecoderMode
)

// VerifyEncoderMode is how to verify encoder implementation.
type VerifyEncoderMode int

const (
	// VerifyHWEncoderUsed refers to WebRTC hardware accelerated video encoding.
	VerifyHWEncoderUsed VerifyEncoderMode = iota
	// VerifySWEncoderUsed refers to WebRTC software video encoding.
	VerifySWEncoderUsed
	// NoVerifyEncoderMode means it doesn't matter if WebRTC uses any video encoder.
	NoVerifyEncoderMode
)

// DisplayMediaType represents displaySurface property in displayMedia constraints.
// See https://w3c.github.io/mediacapture-screen-share/#dom-displaycapturesurfacetype.
type DisplayMediaType string

const (
	// CaptureMonitor is to capture an entire screen.
	CaptureMonitor DisplayMediaType = "monitor"
	// CaptureWindow is to capture a window.
	CaptureWindow = "window"
	// CaptureTab is to capture tab in a browser.
	CaptureTab = "browser"
)

const (
	// LoopbackFile is the file containing the RTCPeerConnection loopback code.
	LoopbackFile = "loopback_peerconnection.html"

	// SimulcastAdapterName is the name of the RTC Stat value when simulcast encoding is used.
	SimulcastAdapterName = "SimulcastEncoderAdapter"
)

// RTCTestParams is used to describe the config used to run RTCPeerConnectionPerf.
type RTCTestParams struct {
	VerifyDecoderMode VerifyDecoderMode // The verification mode for decoder.
	VerifyEncoderMode VerifyEncoderMode // The verification mode for encoder.
	Profile           string            // Codec to try, e.g. VP8, VP9.
	StreamWidth       int               // Width of video to be sent in the peerconnection.
	StreamHeight      int               // Height of video to be sent in the peerconnection.
	// ScalableVideoCodec "scalabilityMode" identifier.
	// https://www.w3.org/TR/webrtc-svc/#scalabilitymodes
	Svc string
	// The number of video streams in simulcast.
	Simulcasts int
	// If non-empty, the media to send through the RTC connection will be obtained
	// using getDisplayMedia() and the value corresponds to the surface type. If
	// empty, the media to send will be obtained using getUserMedia().
	DisplayMediaType DisplayMediaType

	// Below are used only in performance tests.
	VideoGridDimension int    // Dimension of the grid in which to embed the RTCPeerConnection <video>.
	VideoGridFile      string // Name of the video file to fill up the grid with, if needed.
	// The array each element of which is true iff -th smaller resolution should be a hardware encoder implementation.
	SimulcastHWEncs []bool
	// BrowserType indicates the type of Chrome browser to be used,
	// Ash Chrome or Lacros Chrome.
	BrowserType browser.Type
	// VerifyOutOfProcessVideoEncodingIsUsed denotes if we should verify that a utility encoder process was started.
	VerifyOutOfProcessVideoEncodingIsUsed bool
	TraceChromeEvents                     bool
}

// RunRTCPeerConnection launches a loopback RTCPeerConnection and inspects that the
// VerifyHWAcceleratorMode codec is hardware accelerated if profile is not NoVerifyHWAcceleratorUsed.
func RunRTCPeerConnection(ctx context.Context, cs ash.ConnSource, cr *chrome.Chrome, fileSystem http.FileSystem, params RTCTestParams) error {
	// verifyMode VerifyHWAcceleratorMode, profile string, simulcast bool, svc string, displayMediaType DisplayMediaType)
	if params.Simulcasts > 1 && params.Svc != "" {
		return errors.New("|simulcast| and |svc| cannot be set simultaneously")
	}
	if params.DisplayMediaType != "" && (params.Simulcasts > 1 || params.Svc != "") {
		return errors.New("Screen capture can't be used with simulcast or SVC")
	}
	if params.VerifyDecoderMode != NoVerifyDecoderMode && params.VerifyEncoderMode != NoVerifyEncoderMode {
		return errors.New("Decoder and encoder implementation cannot be verified simultaneously")
	}

	vl, err := logging.NewVideoLogger()
	if err != nil {
		return errors.Wrap(err, "failed to set values for verbose logging")
	}
	defer vl.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to test API")
	}

	if _, err := display.GetInternalInfo(ctx, tconn); err == nil {
		// The device has an internal display.
		// For consistency across test runs, ensure that the device is in landscape-primary orientation.
		if err = graphics.RotateDisplayToLandscapePrimary(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to set display to landscape-primary orientation")
		}
	}

	server := httptest.NewServer(http.FileServer(fileSystem))
	defer server.Close()

	conn, err := cs.NewConn(ctx, server.URL+"/"+LoopbackFile)
	if err != nil {
		return errors.Wrap(err, "failed to open video page")
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	if canCapture, err := setupCapture(ctx, conn, tconn, params.DisplayMediaType, params.StreamWidth, params.StreamHeight); err != nil {
		return errors.Wrap(err, "failed to setup capture")
	} else if !canCapture {
		return nil
	}

	if err := conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		return errors.Wrap(err, "timed out waiting for page loading")
	}

	if err := conn.Call(ctx, nil, "start", params.Profile, params.StreamWidth, params.StreamHeight, params.Simulcasts, params.Svc, params.DisplayMediaType); err != nil {
		return errors.Wrap(err, "error establishing connection")
	}

	if params.VerifyOutOfProcessVideoEncodingIsUsed {
		if err := oop.VerifyOneUtilityEncoderProcessWasStarted(); err != nil {
			return errors.Wrap(err, "VerifyOneUtilityEncoderProcessWasStarted failed")
		}
	}

	if err := verifyDecoderImplementation(ctx, conn, params.VerifyDecoderMode); err != nil {
		return err
	}
	if err := verifyEncoderImplementation(ctx, conn, params.VerifyEncoderMode, params.SimulcastHWEncs); err != nil {
		return err
	}

	return nil
}

func verifyDecoderImplementation(ctx context.Context, conn *chrome.Conn, verifyDecoderMode VerifyDecoderMode) error {
	if verifyDecoderMode == NoVerifyDecoderMode {
		return nil
	}

	decImplName, hwDecoderUsed, err := getCodecImplementation(ctx, conn /*decode=*/, true)
	if err != nil {
		return errors.Wrap(err, "failed to get decoder implementation name")
	}
	if verifyDecoderMode == VerifyHWDecoderUsed && !hwDecoderUsed {
		return errors.Errorf("hardware decode accelerator wasn't used, got %s", decImplName)
	}
	if verifyDecoderMode == VerifySWDecoderUsed && hwDecoderUsed {
		return errors.Errorf("software decode wasn't used, got %s", decImplName)
	}
	return nil
}

func verifyEncoderImplementation(ctx context.Context, conn *chrome.Conn, verifyEncoderMode VerifyEncoderMode, simulcastHWEncs []bool) error {
	if verifyEncoderMode == NoVerifyEncoderMode {
		return nil
	}

	encImplName, hwEncoderUsed, err := getCodecImplementation(ctx, conn, false)
	if err != nil {
		return errors.Wrap(err, "failed to get encoder implementation name")
	}
	if len(simulcastHWEncs) > 1 {
		return checkSimulcastEncImpl(encImplName, simulcastHWEncs)
	}

	if verifyEncoderMode == VerifyHWEncoderUsed && !hwEncoderUsed {
		return errors.Errorf("hardware encode accelerator wasn't used, got %s", encImplName)
	}
	if verifyEncoderMode == VerifySWEncoderUsed && hwEncoderUsed {
		return errors.Errorf("software encode wasn't used, got %s", encImplName)
	}
	return nil
}

type peerConnectionType bool

const (
	localPeerConnection  peerConnectionType = false
	remotePeerConnection peerConnectionType = true
)

// readRTCReport reads an RTCStat report of the given typ from the specified peer connection.
// Since there are multiple outbound-rtp in the case of simulcast, the stat is selected whose frame height is the largest.
// The out can be an arbitrary struct whose members are 'json' tagged, so that they will be filled.
func readRTCReport(ctx context.Context, conn *chrome.Conn, pc peerConnectionType, typ string, out interface{}) error {
	return conn.Call(ctx, out, `async(isRemote, type) => {
	  const peerConnection = isRemote ? remotePeerConnection : localPeerConnection;
	  const stats = await peerConnection.getStats(null);
	  if (stats == null) {
	    throw new Error("getStats() failed");
	  }
      var R = null;
	  for (const [_, report] of stats) {
	    if (report['type'] === type &&
            (!R || R['frameHeight'] < report['frameHeight'])) {
          R = report;
	    }
	  }
      if (R !== null) {
        return R;
      }
	  throw new Error("Stat not found");
	}`, pc, typ)
}

// getCodecImplementation parses the RTCPeerConnection and returns the implementation name and whether it is
// a hardware implementation. If decode is true, this returns decoder implementation and otherwise encoder implementation.
// This method uses the RTCPeerConnection getStats() API [1].
// [1] https://w3c.github.io/webrtc-pc/#statistics-model
func getCodecImplementation(ctx context.Context, conn *chrome.Conn, decode bool) (string, bool, error) {
	// See [1] and [2] for the statNames to use here. The values are browser
	// specific, for Chrome, "ExternalDecoder" and "{V4L2,Vaapi, etc.}VideoEncodeAccelerator"
	// means that WebRTC is using hardware acceleration and anything else
	// (e.g. "libvpx", "ffmpeg", "unknown") means it is not.
	// A SimulcastEncoderAdapter is actually a grouping of implementations, so it can read e.g.
	// "SimulcastEncoderAdapter (libvpx, VaapiVideoEncodeAccelerator, VaapiVideoEncodeAccelerator)"
	// (note that there isn't a SimulcastDecoderAdapter).
	//
	// [1] https://w3c.github.io/webrtc-stats/#dom-rtcinboundrtpstreamstats-decoderimplementation
	// [2] https://w3c.github.io/webrtc-stats/#dom-rtcoutboundrtpstreamstats-encoderimplementation
	hwImplName := "EncodeAccelerator"
	readImpl := func(ctx context.Context) (string, error) {
		var out struct {
			Encoder string `json:"encoderImplementation"`
		}
		if err := readRTCReport(ctx, conn, localPeerConnection, "outbound-rtp", &out); err != nil {
			return "", err
		}
		return out.Encoder, nil
	}

	if decode {
		hwImplName = "ExternalDecoder"
		readImpl = func(ctx context.Context) (string, error) {
			var out struct {
				Decoder string `json:"decoderImplementation"`
			}
			if err := readRTCReport(ctx, conn, remotePeerConnection, "inbound-rtp", &out); err != nil {
				return "", err
			}
			return out.Decoder, nil
		}
	}

	// Poll getStats() to wait until {decoder,encoder}Implementation gets filled in:
	// RTCPeerConnection needs a few frames to start up encoding/decoding; in the
	// meantime it returns "unknown".
	const pollInterval = 100 * time.Millisecond
	const pollTimeout = 200 * pollInterval
	var impl string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		impl, err = readImpl(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to retrieve and/or parse RTCStatsReport")
		}
		if impl == "" || impl == "unknown" {
			return errors.New("getStats() didn't fill in the codec implementation (yet)")
		}
		// "ExternalEncoder" is the default value for encoder implementations
		// before filling the actual one. We need to wait until the implementation name
		// doesn't contain ExternalEncoder because in simulcast case it is like
		// SimulcastEncoderAdapter (ExternalEncoder, ExternalEncoder, ExternalEncoder).
		if strings.Contains(impl, "ExternalEncoder") {
			return errors.New("getStats() didn't fill in the encoder implementation (yet)")
		}
		return nil
	}, &testing.PollOptions{Interval: pollInterval, Timeout: pollTimeout}); err != nil {
		return "", false, err
	}
	testing.ContextLog(ctx, "Implementation: ", impl)

	isHWImpl := strings.Contains(impl, hwImplName)
	return impl, isHWImpl, nil
}

// checkSimulcastEncImpl checks that the given implName is used in given simulcast scenario.
// isImplHWInAdapter[i] stands for whether i-th stream in the simulcast is expected to be produced by a hardware encoder.
func checkSimulcastEncImpl(implName string, isImplHWInAdapter []bool) error {
	isAllSWEnc := true
	for _, isHW := range isImplHWInAdapter {
		if isHW {
			isAllSWEnc = false
			break
		}
	}
	// If all the streams in the simulcast are produced by a software encoder.
	// The implementation name is libvpx because a libvpx encoder supports simulcast.
	if isAllSWEnc {
		if implName == "libvpx" {
			return nil
		}
		return errors.Errorf("unexpected simulcast encoder adapter name: %s", implName)
	}

	// If the streams in the simulcast are produced by software and hardware encoders or
	// only hardware encoders, SimulcastEncoderAdapter is used to bundle the streams.
	// The implementation name is like SimulcastEncoderAdapter(libvpx, VaapiVideoEncodeAccelerator, VaapiVideoEncodeAccelerator).
	implName = strings.ReplaceAll(implName, " ", "")
	re := regexp.MustCompile(`SimulcastEncoderAdapter\(([\S, ]+)\)`)
	inStrs := re.FindStringSubmatch(implName)
	if inStrs == nil {
		return errors.Errorf("failed to extract internal encoder names: %s", implName)
	}

	implNames := strings.Split(inStrs[1], ",")
	if len(implNames) != len(isImplHWInAdapter) {
		return errors.Errorf("the number of simulcast streams mismatches: got %d (%v), expected %d", len(implNames), implNames, len(isImplHWInAdapter))
	}

	const hwImplName = "EncodeAccelerator"
	for i, implName := range implNames {
		if isImplHWInAdapter[i] != strings.Contains(implName, hwImplName) {
			return errors.Errorf("unexpected implementations within simulcast adapter: %v", implNames)
		}
	}
	return nil
}

// setupCapture sets up the capture environment and returns whether streamWidth x streamHeight capture is possible.
// TODO(b/292617867): Skip by using HardwareDeps.
func setupCapture(ctx context.Context, conn *chrome.Conn, tconn *chrome.TestConn, dm DisplayMediaType, streamWidth, streamHeight int) (bool, error) {
	// Maximize window size so that the window size to be captured is maximized
	// and also camera and display capturing is executed in the same situation.
	if err := ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
		return ash.SetWindowStateAndWait(ctx, tconn, w.ID, ash.WindowStateMaximized)
	}); err != nil {
		return false, errors.Wrap(err, "failed to maximize window")
	}

	// We don't need to check window and monitor resolution in camera capture.
	if dm == "" {
		return true, nil
	}

	// We verify later if either width or height reaches streamWidth or streamHeight in screen capture.
	// If it cannot be reached, we can skip the test.
	var width, height int
	var captureSource string
	if dm == CaptureTab || dm == CaptureWindow {
		captureSource = "tab"
		widthStr := "window.innerWidth"
		heightStr := "window.innerHeight"
		if dm == CaptureWindow {
			captureSource = "window"
			widthStr = "window.outerWidth"
			heightStr = "window.outerHeight"
		}

		if err := conn.Eval(ctx, widthStr, &width); err != nil {
			return false, errors.Wrap(err, "failed to retrieve height")
		}
		if err := conn.Eval(ctx, heightStr, &height); err != nil {
			return false, errors.Wrap(err, "failed to retrieve width")
		}
		var pixelRatio float32
		if err := conn.Eval(ctx, "window.devicePixelRatio", &pixelRatio); err != nil {
			return false, errors.Wrap(err, "failed to retrieve pixel ratio")
		}
		width = int(float32(width) * pixelRatio)
		height = int(float32(height) * pixelRatio)
		testing.ContextLogf(ctx, "%s resolution: %dx%d (pixel ratio=%.3f)", captureSource, width, height, pixelRatio)
	} else if dm == CaptureMonitor {
		captureSource = "monitor"
		screenRes := util.GetScreenResolution(ctx)
		testing.ContextLog(ctx, "Screen resolution: ", screenRes)
		if screenRes == "" {
			return false, errors.New("failed to get screen resolution")
		}
		screenWH := strings.Split(screenRes, "x")
		if len(screenWH) != 2 {
			return false, errors.Errorf("failed to split resolution by 'x': %v", screenRes)
		}
		var err error
		width, err = strconv.Atoi(screenWH[0])
		if err != nil {
			return false, errors.Wrapf(err, "failed to convert to int: %s", screenWH[0])
		}
		height, err = strconv.Atoi(screenWH[1])
		if err != nil {
			return false, errors.Wrapf(err, "failed to convert to int: %s", screenWH[1])
		}
	}

	if width < streamWidth && height < streamHeight {
		testing.ContextLogf(ctx, "Skip the test because the both width and height of %s, %dx%d, are less than the capture resolution %dx%d", captureSource, width, height, streamWidth, streamHeight)
		return false, nil
	}

	return true, nil
}

// DataFiles returns a list of required files that tests that use this package
// should include in their Data fields.
func DataFiles() []string {
	return []string{
		"canvas_animation.js",
		"loopback_peerconnection.js",
		"third_party/blackframe.js",
		"third_party/munge_sdp.js",
		"third_party/sdp/sdp.js",
		"third_party/simulcast/simulcast.js",
		"third_party/ssim.js",
	}
}
