// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package peerconnection provides common code for webrtc.* RTCPeerConnection tests.
package peerconnection

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/media/logging"
	"go.chromium.org/tast-tests/cros/local/media/oop"
	"go.chromium.org/tast-tests/cros/local/media/webrtc"
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

// readRTCReport returns a function to read WebRTC stats for an |id| decoder and encoder.
// |id| is an integer rating from 0 to N-1, where N is the number of spatial layers.
// Since there are multiple outbound-rtp in the case of simulcast, the stat is selected whose frame height is the largest.
// TODO(b/322436617): Verify all the decoders in simulcast.
// The out can be an arbitrary struct whose members are 'json' tagged, so that they will be filled.
func readRTCReport(id int) webrtc.ReadRTCReportFunc {
	return func(ctx context.Context, conn *chrome.Conn, decode bool, out interface{}) error {
		// Decode: remotePeerConnection, "inbound-rtp"
		// Encode: localPeerConnection, "outbound-rtp"
		var peerConnection string
		var staticType string
		if decode {
			peerConnection = fmt.Sprintf("testVisible.remotePeerConnections[%d]", id)
			staticType = "inbound-rtp"
		} else {
			peerConnection = fmt.Sprintf("testVisible.localPeerConnections[%d]", id)
			staticType = "outbound-rtp"
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

func isSMode(scalabilityMode string) bool {
	return strings.HasPrefix(scalabilityMode, "S")
}

func numSModeLayers(scalabilityMode string) (int, error) {
	numLayers, err := strconv.Atoi(string(scalabilityMode[1]))
	if err != nil {
		return 0, errors.Wrapf(err, "failed to get the numbner of layers in s-mode encoding: %s", scalabilityMode)
	}
	return numLayers, nil
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

	if isSMode(params.Svc) {
		if err := conn.Call(ctx, nil, "startSMode", params.Profile, params.StreamWidth, params.StreamHeight, params.Svc); err != nil {
			return errors.Wrap(err, "error establishing connection")
		}
	} else {
		if err := conn.Call(ctx, nil, "start", params.Profile, params.StreamWidth, params.StreamHeight, params.Simulcasts, params.Svc, params.DisplayMediaType); err != nil {
			return errors.Wrap(err, "error establishing connection")
		}
	}

	if params.VerifyOutOfProcessVideoEncodingIsUsed {
		if err := oop.VerifyOneUtilityEncoderProcessWasStarted(); err != nil {
			return errors.Wrap(err, "VerifyOneUtilityEncoderProcessWasStarted failed")
		}
	}

	return verifyCodecImplementation(ctx, conn, params.VerifyDecoderMode,
		params.VerifyEncoderMode, params.Svc, params.SimulcastHWEncs)
}

func verifyCodecImplementation(ctx context.Context,
	conn *chrome.Conn, verifyDecoderMode VerifyDecoderMode,
	verifyEncoderMode VerifyEncoderMode,
	scalabilityMode string,
	simulcastHWEncs []bool) error {
	if err := verifyDecoderImplementation(ctx, conn, verifyDecoderMode, scalabilityMode); err != nil {
		return err
	}
	if err := verifyEncoderImplementation(ctx, conn, verifyEncoderMode, scalabilityMode, simulcastHWEncs); err != nil {
		return err
	}
	return nil
}

func verifyDecoderImplementation(ctx context.Context, conn *chrome.Conn, verifyDecoderMode VerifyDecoderMode, scalabilityMode string) error {
	if verifyDecoderMode == NoVerifyDecoderMode {
		return nil
	}

	numDecoders := 1
	// TODO(b/322436617): Verify all the decoders in simulcast and non S-mode SVC encoding.
	if isSMode(scalabilityMode) {
		var err error
		numDecoders, err = numSModeLayers(scalabilityMode)
		if err != nil {
			return err
		}
	}
	for i := 0; i < numDecoders; i++ {
		decImplName, hwDecoderUsed, err := webrtc.GetCodecImplementation(ctx, conn, true, readRTCReport(i))
		if err != nil {
			return errors.Wrapf(err, "failed to get decoder implementation name for remotePeerConnections[%d]", i)
		}
		if verifyDecoderMode == VerifyHWDecoderUsed && !hwDecoderUsed {
			return errors.Errorf("hardware decode accelerator wasn't used, got %s for remotePeerConnections[%d]", decImplName, i)
		}
		if verifyDecoderMode == VerifySWDecoderUsed && hwDecoderUsed {
			return errors.Errorf("software decode wasn't used, got %s for localPeerConnections[%d]", decImplName, i)
		}
	}
	return nil
}

func verifyEncoderImplementation(ctx context.Context, conn *chrome.Conn, verifyEncoderMode VerifyEncoderMode, scalabilityMode string, simulcastHWEncs []bool) error {
	if verifyEncoderMode == NoVerifyEncoderMode {
		return nil
	}

	id := 0
	if isSMode(scalabilityMode) {
		numStreams, err := numSModeLayers(scalabilityMode)
		if err != nil {
			return err
		}
		// The main local peer connection in S mode encoding is set to
		// testVisible.localPeerConnections[numStreams - 1].
		id = numStreams - 1
	}
	encImplName, hwEncoderUsed, err := webrtc.GetCodecImplementation(ctx, conn, false, readRTCReport(id))
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
		var err error
		width, height, err = displayResolution(ctx)
		if err != nil {
			return false, errors.Wrap(err, "failed to get screen resolution")
		}
	}

	if width < streamWidth && height < streamHeight {
		testing.ContextLogf(ctx, "Skip the test because the both width and height of %s, %dx%d, are less than the capture resolution %dx%d", captureSource, width, height, streamWidth, streamHeight)
		return false, nil
	}

	return true, nil
}

// displayResolution gets the resolution of the primary connected display.
func displayResolution(ctx context.Context) (width, height int, err error) {
	displays, err := graphics.ModetestConnectedDisplays(ctx)
	if err != nil {
		return -1, -1, err
	}
	if len(displays) == 0 {
		return -1, -1, errors.New("no connected display, please add hwdep.InternalDisplay() to HardwareDeps")
	}
	if len(displays) > 1 {
		return -1, -1, errors.New("multiple displays are connected, please add hwdep.NoExternalDisplay() to HardwareDeps")
	}
	d := displays[0]
	width = int(d.Crtc.Width)
	height = int(d.Crtc.Height)
	return width, height, nil
}

// DataFiles returns a list of required files that tests that use this package
// should include in their Data fields.
func DataFiles() []string {
	return []string{
		"canvas_animation.js",
		"loopback_peerconnection.js",
		"loopback_peerconnection_util.js",
		"third_party/blackframe.js",
		"third_party/munge_sdp.js",
		"third_party/sdp/sdp.js",
		"third_party/simulcast/simulcast.js",
		"third_party/ssim.js",
	}
}
