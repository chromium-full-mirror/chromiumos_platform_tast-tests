// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/media/webrtc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func logResolution(ctx context.Context, conn *chrome.Conn, jsFunc, source string) error {
	type resolution struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	var res resolution
	if err := conn.Call(ctx, &res, fmt.Sprintf("() => { return %s; }", jsFunc), nil); err != nil {
		return errors.Wrapf(err, "failed getting a %s resolution", source)
	}
	testing.ContextLogf(ctx, "Resolution (%s): %v", source, res)
	return nil
}

// logCameraResolution outputs the camera resolution in the log.
func logCameraResolution(ctx context.Context, conn *chrome.Conn) error {
	return logResolution(ctx, conn, "VC.getCameraResolution()", "camera")
}

// logDisplayCaptureResolution outputs the display capture resolution in the log.
func logDisplayCaptureResolution(ctx context.Context, conn *chrome.Conn) error {
	return logResolution(ctx, conn, "VC.getDisplayCaptureResolution()", "display capture")
}

// waitForPeerConnectionStabilized waits until the encoder resolution in the peer connection becomes
// the expected resolution. The expected resolution is computed from the encoder config.
// The expected resolution is returned.
func waitForPeerConnectionStabilized(ctx context.Context, conn *chrome.Conn, numPeople int) (int, int, error) {
	type encoderConfig struct {
		Codec           string `json:"codec"`
		InputHeight     int    `json:"inputHeight"`
		OutputHeight    int    `json:"outputHeight"`
		ScalabilityMode string `json:"scalabilityMode"`
	}
	var encCfg encoderConfig
	if err := conn.Call(ctx, &encCfg, fmt.Sprintf("() => { return VC.getEncoderConfig(%d); }", numPeople), nil); err != nil {
		return -1, -1, errors.Wrap(err, "failed getting the encoder configuration")
	}
	testing.ContextLogf(ctx, "Video encoder config: %s, %dp, %s, scaleResolutionDownBy: %.2f (=%d/%d)",
		encCfg.Codec, encCfg.OutputHeight, encCfg.ScalabilityMode,
		float32(encCfg.InputHeight)/float32(encCfg.OutputHeight),
		encCfg.InputHeight, encCfg.OutputHeight)
	streamHeight := encCfg.OutputHeight
	streamWidth := encCfg.OutputHeight * 16 / 9

	mainPeerConnectionIndex := numPeople - 2
	if err := webrtc.WaitForPeerConnectionStabilized(ctx, conn,
		encCfg.Codec, streamWidth, streamHeight, false, encCfg.ScalabilityMode,
		readRTCReport(mainPeerConnectionIndex, false),
		webrtc.CreateReadCodecFunc(fmt.Sprintf("VC.localPCs[%d]", mainPeerConnectionIndex))); err != nil {
		return -1, -1, err
	}
	return streamWidth, streamHeight, nil
}

// collectWebRTCVideoStats collects the video encoder and decoder performance:
// - the encoder performance for the camera source
// - the encoder performance for the display source if present is true.
// - the decoders performance
// The performance measures after the resolution of a decoder and an encoder is streamWidth and streamHeight.
func collectWebRTCVideoStats(ctx context.Context, conn *chrome.Conn, present bool, streamWidth, streamHeight, numDecoders int) ([]*perf.Values, *perf.Values, *perf.Values, error) {
	var wg sync.WaitGroup

	var statErrs = make([]error, numDecoders)
	var decPerfs = make([]*perf.Values, numDecoders)
	cameraEncPerf := perf.NewValues()
	for i := 0; i < numDecoders; i++ {
		decPerfs[i] = perf.NewValues()
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			readRTCReportFunc := readRTCReport(i, false)
			if err := webrtc.MeasureRTCDecodeStats(ctx, conn, streamWidth, streamHeight, readRTCReportFunc, placeHolderValidateFrame, decPerfs[i]); err != nil {
				statErrs[i] = err
				return
			}
			if i == numDecoders-1 {
				if err := webrtc.MeasureRTCEncodeStats(ctx, conn, readRTCReportFunc, cameraEncPerf); err != nil {
					statErrs[i] = err
					return
				}
			}
		}(i)
	}

	var presentStatErr error
	presentEncPerf := perf.NewValues()
	if present {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := webrtc.MeasureRTCEncodeStats(ctx, conn, readRTCReport(0, true), presentEncPerf); err != nil {
				presentStatErr = err
				return
			}
		}()
	}
	wg.Wait()
	for _, err := range statErrs {
		if err != nil {
			return nil, nil, nil, err
		}
	}
	if presentStatErr != nil {
		return nil, nil, nil, presentStatErr
	}
	return decPerfs, cameraEncPerf, presentEncPerf, nil
}

// medianDecoderPerf returns the decoder performance, which is the median in the order of decode time.
func medianDecoderPerf(ctx context.Context, decPerfs []*perf.Values) *perf.Values {
	for i, decPerf := range decPerfs {
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
	return decPerfs[len(decPerfs)/2]
}

// measureWebRTCPerf records the webrtc decoder and encoder performance metrics from webrtc stats.
func measureWebRTCPerf(ctx context.Context, conn *chrome.Conn, rtcPerf *perf.Values, present bool, numPeople int) error {
	if err := logCameraResolution(ctx, conn); err != nil {
		return err
	}
	streamWidth, streamHeight, err := waitForPeerConnectionStabilized(ctx, conn, numPeople)
	if err != nil {
		return err
	}
	if present {
		if err := logDisplayCaptureResolution(ctx, conn); err != nil {
			return err
		}
	}

	const webRTCCoolDownDuration = 5 * time.Second
	testing.ContextLogf(ctx, "Sleep to eliminate the performance effect of rtc peer connection start up(%v)", webRTCCoolDownDuration)
	// GoBigSleepLint: Sleep to eliminate the performance effect on the start
	// up of rtc peer connection. The duration, 5 seconds, is arbitrary
	// selected and will be changed if necessary.
	if err := testing.Sleep(ctx, webRTCCoolDownDuration); err != nil {
		return errors.Wrapf(err, "failed to sleep for %v", webRTCCoolDownDuration)
	}

	decPerfs, cameraEncPerf, presentEncPerf, err := collectWebRTCVideoStats(ctx, conn, present, streamWidth, streamHeight, numPeople-1)
	if err != nil {
		return err
	}
	rtcPerf.MergeWithSuffix("_median_dec", medianDecoderPerf(ctx, decPerfs))
	rtcPerf.MergeWithSuffix("_camera_enc", cameraEncPerf)
	rtcPerf.MergeWithSuffix("_present_enc", presentEncPerf)
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
