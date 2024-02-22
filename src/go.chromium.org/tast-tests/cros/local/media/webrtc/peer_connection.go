// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package webrtc provides useful functions for WebRTC peerconnection.
package webrtc

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type peerConnectionType bool

const (
	localPeerConnection  peerConnectionType = false
	remotePeerConnection peerConnectionType = true
)

// WebRTC Stats collected on transmission side.
type txMeas struct {
	// From https://www.w3.org/TR/webrtc-stats/#dom-rtcoutboundrtpstreamstats-totalencodetime:
	// "Total number of seconds that has been spent encoding the framesEncoded
	// frames of this stream. The average encode time can be calculated by
	// dividing this value with framesEncoded."
	TotalEncodeTime float64 `json:"totalEncodeTime"`
	FramesEncoded   float64 `json:"framesEncoded"`
	// See https://www.w3.org/TR/webrtc-stats/#vststats-dict* for the following.
	FrameWidth      float64 `json:"frameWidth"`
	FrameHeight     float64 `json:"frameHeight"`
	FramesPerSecond float64 `json:"framesPerSecond"`
	Encoder         string  `json:"encoderImplementation"`
	Active          bool    `json:"active"`
	ScalabilityMode string  `json:"scalabilityMode"`
}

// WebRTC Stats collected on the receiver side.
type rxMeas struct {
	// From https://w3c.github.io/webrtc-stats/#dom-rtcinboundrtpstreamstats-totaldecodetime
	// "Total number of seconds that have been spent decoding the framesDecoded
	// frames of this stream. The average decode time can be calculated by
	// dividing this value with framesDecoded."
	TotalDecodeTime float64 `json:"totalDecodeTime"`
	FramesDecoded   float64 `json:"framesDecoded"`
	FramesDropped   float64 `json:"framesDropped"`
}

// ReadRTCReportFunc is the type of a function that reads WebRTC stats and fills out in rxMeas if decode is true, or txMeas.
type ReadRTCReportFunc func(ctx context.Context, conn *chrome.Conn, decode bool, out interface{}) error

type validateFrameFunc func(ctx context.Context, conn *chrome.Conn, width, height int) error

// WaitForPeerConnectionStabilized waits up to maxStreamWarmUp for one of the
// following:
//
// - If displayCapture is false (i.e., we're capturing user media), it waits for the
// transmitted resolution to reach streamWidth x streamHeight.
//
// - If displayCapture is true (i.e., we're capturing display media), it waits for
// the transmitted width to reach streamWidth or for the transmitted height to
// reach streamHeight.
//
// Returns error on failure or timeout.
func WaitForPeerConnectionStabilized(ctx context.Context, conn *chrome.Conn,
	streamWidth, streamHeight int, displayCapture bool, scalabilityMode string, readRTCReport ReadRTCReportFunc) error {
	const (
		// Before taking any measurements, we need to wait for the RTCPeerConnection
		// to ramp up the CPU adaptation; until then, the transmitted resolution may
		// be smaller than the one expected.
		maxStreamWarmUp = 60 * time.Second
	)
	testing.ContextLogf(ctx, "Waiting at most %v seconds for tx resolution rampup, target %dx%d", maxStreamWarmUp, streamWidth, streamHeight)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var txm txMeas
		if err := readRTCReport(ctx, conn, false, &txm); err != nil {
			return testing.PollBreak(err)
		}
		// In the case of screen capture (i.e. the source is non-camera), the original
		// source may not be 16:9, so we can't expect the tx stream dimensions to be
		// exactly equal to streamWidth x streamHeight. However, we can expect either the
		// tx stream width or height to match streamWidth or streamHeight respectively
		// because the screen capture will be scaled to match one of the dimensions and
		// keep the original aspect ratio.
		if displayCapture {
			if int(txm.FrameHeight) != streamHeight && int(txm.FrameWidth) != streamWidth {
				return errors.Errorf("still waiting for tx width to reach %d or tx height to reach %d, current: %.0fx%.0f, txm=%v",
					streamWidth, streamHeight, txm.FrameWidth, txm.FrameHeight, txm)
			}
		} else {
			if int(txm.FrameHeight) != streamHeight || int(txm.FrameWidth) != streamWidth {
				return errors.Errorf("still waiting for tx resolution to reach %dx%d, current: %.0fx%.0f, txm=%v",
					streamWidth, streamHeight, txm.FrameWidth, txm.FrameHeight, txm)
			}
		}
		if scalabilityMode != "" && !strings.HasPrefix(scalabilityMode, "L1") && scalabilityMode != txm.ScalabilityMode {
			return errors.Errorf("scalabilityMode is not %s, current: %s, txm=%v", scalabilityMode, txm.ScalabilityMode, txm)
		}

		testing.ContextLogf(ctx, "tx resolution: %.0fx%.0f, scalabilityMode: %s, implementation: %s", txm.FrameWidth, txm.FrameHeight, txm.ScalabilityMode, txm.Encoder)
		return nil
	}, &testing.PollOptions{Timeout: maxStreamWarmUp, Interval: time.Second}); err != nil {
		return errors.Wrap(err, "timeout waiting for tx resolution to stabilize")
	}
	return nil
}

// GetCodecImplementation parses the RTCPeerConnection and returns the implementation name and whether it is
// a hardware implementation (i.e. powerEfficient). If decode is true, this returns decoder implementation and otherwise encoder implementation.
// This method uses the RTCPeerConnection getStats() API [1].
// [1] https://w3c.github.io/webrtc-pc/#statistics-model
func GetCodecImplementation(ctx context.Context, conn *chrome.Conn, decode bool, readRTCReport ReadRTCReportFunc) (string, bool, error) {
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
	readImplAndPowerEfficient := func(ctx context.Context) (string, bool, int, error) {
		var out struct {
			Encoder          string `json:"encoderImplementation"`
			PowerEfficient   bool   `json:"powerEfficientEncoder"`
			NumEncodedFrames int    `json:"framesEncoded"`
		}
		if err := readRTCReport(ctx, conn, false, &out); err != nil {
			return "", false, 0, err
		}
		return out.Encoder, out.PowerEfficient, out.NumEncodedFrames, nil
	}

	if decode {
		readImplAndPowerEfficient = func(ctx context.Context) (string, bool, int, error) {
			var out struct {
				Decoder          string `json:"decoderImplementation"`
				PowerEfficient   bool   `json:"powerEfficientDecoder"`
				NumDecodedFrames int    `json:"framesDecoded"`
			}
			if err := readRTCReport(ctx, conn, true, &out); err != nil {
				return "", false, 0, err
			}
			return out.Decoder, out.PowerEfficient, out.NumDecodedFrames, nil
		}
	}

	processedStr := "encoded"
	if decode {
		processedStr = "decoded"
	}
	// Poll getStats() to wait until {decoder,encoder}Implementation gets filled in
	// and the {decoder,encode} {decodes,encodes} minNumFrames: RTCPeerConnection needs
	// a few frames to start up encoding/decoding; in the meantime it returns "unknown".
	// Wait until |minNumFrames| are decoded/encoded so that the hardware codecs
	// have the chance to possibly fall back to software codecs.
	const pollInterval = 200 * time.Millisecond
	const pollTimeout = 200 * pollInterval
	const minNumFrames = 30
	var impl string
	numFrames := 0
	powerEfficient := false
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error

		impl, powerEfficient, numFrames, err = readImplAndPowerEfficient(ctx)
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
		// Waiting until minNumFrames are decoded/encoded.
		if numFrames < minNumFrames {
			return errors.Errorf("still waiting for %d frames to be %s; got %d frames so far", minNumFrames, processedStr, numFrames)
		}
		return nil
	}, &testing.PollOptions{Interval: pollInterval, Timeout: pollTimeout}); err != nil {
		return "", false, err
	}
	testing.ContextLog(ctx, "Implementation: ", impl)
	testing.ContextLog(ctx, "PowerEfficient: ", powerEfficient)
	testing.ContextLogf(ctx, "Number of %s frames: %d", processedStr, numFrames)

	return impl, powerEfficient, nil
}

// MeasureRTCEncodeStats parses the WebRTC Tx stats, and stores them into p.
// See https://www.w3.org/TR/webrtc-stats/#stats-dictionaries for more info.
func MeasureRTCEncodeStats(ctx context.Context, conn *chrome.Conn, readRTCReport ReadRTCReportFunc, p *perf.Values) error {
	const (
		// timeSamples specifies number of frame decode time samples to get.
		timeSamples = 10
	)

	var txMeasurements []txMeas
	for i := 0; i < timeSamples; i++ {
		// GoBigSleepLint: sleep 1 second so that getStats() is not called too many times in a short term.
		if err := testing.Sleep(ctx, time.Second); err != nil {
			return err
		}
		var txm txMeas
		if err := readRTCReport(ctx, conn, false, &txm); err != nil {
			return errors.Wrap(err, "failed to retrieve and/or parse getStats()")
		}
		testing.ContextLogf(ctx, "Measurement: %+v", txm)
		txMeasurements = append(txMeasurements, txm)
	}
	framesPerSecond := perf.Metric{
		Name:      "tx.frames_per_second",
		Unit:      "fps",
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
	}
	for _, txMeasurement := range txMeasurements {
		p.Append(framesPerSecond, txMeasurement.FramesPerSecond)
	}

	encodeTime := perf.Metric{
		Name:      "tx.encode_time",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
	}
	for i := 1; i < len(txMeasurements); i++ {
		if txMeasurements[i].FramesEncoded == txMeasurements[i-1].FramesEncoded {
			continue
		}
		averageEncodeTime := (txMeasurements[i].TotalEncodeTime - txMeasurements[i-1].TotalEncodeTime) / (txMeasurements[i].FramesEncoded - txMeasurements[i-1].FramesEncoded) * 1000
		p.Append(encodeTime, averageEncodeTime)
	}
	return nil
}

// MeasureRTCDecodeStats parses the WebRTC Rx stats, and stores them into p.
// See https://www.w3.org/TR/webrtc-stats/#stats-dictionaries for more info.
func MeasureRTCDecodeStats(ctx context.Context, conn *chrome.Conn, streamWidth, streamHeight int,
	readRTCReport ReadRTCReportFunc, validateFrame validateFrameFunc, p *perf.Values) error {
	const (
		// timeSamples specifies number of frame decode time samples to get.
		timeSamples = 10
	)
	var rxMeasurements []rxMeas
	for i := 0; i < timeSamples; i++ {
		// GoBigSleepLint: sleep 1 second so that getStats() is not called too many times in a short term.
		if err := testing.Sleep(ctx, time.Second); err != nil {
			return err
		}
		var rxm rxMeas
		if err := readRTCReport(ctx, conn, true, &rxm); err != nil {
			return errors.Wrap(err, "failed to retrieve and/or parse getStats()")
		}
		testing.ContextLogf(ctx, "Measurement: %+v", rxm)
		rxMeasurements = append(rxMeasurements, rxm)

		if rxm.FramesDecoded == 0 {
			// Wait until the first frame is decoded before analyzind its contents.
			// Slow devices might take a substantial amount of time: b/158848650.
			continue
		}
		if err := validateFrame(ctx, conn, streamWidth/8, streamHeight/8); err != nil {
			return err
		}
	}
	decodeTime := perf.Metric{
		Name:      "rx.decode_time",
		Unit:      "ms",
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
	}
	for i := 1; i < len(rxMeasurements); i++ {
		if rxMeasurements[i].FramesDecoded == rxMeasurements[i-1].FramesDecoded {
			continue
		}
		averageDecodeTime := (rxMeasurements[i].TotalDecodeTime - rxMeasurements[i-1].TotalDecodeTime) / (rxMeasurements[i].FramesDecoded - rxMeasurements[i-1].FramesDecoded) * 1000
		p.Append(decodeTime, averageDecodeTime)
	}

	lastRxm := rxMeasurements[len(rxMeasurements)-1]
	p.Set(perf.Metric{
		Name:      "rx.dropped_frames_percentage",
		Unit:      "percent",
		Direction: perf.SmallerIsBetter,
	}, lastRxm.FramesDropped/lastRxm.FramesDecoded)
	return nil
}

// MeasureRTCStats parses the WebRTC Tx and Rx Stats, and stores them into p.
// See https://www.w3.org/TR/webrtc-stats/#stats-dictionaries for more info.
func MeasureRTCStats(ctx context.Context, conn *chrome.Conn, streamWidth, streamHeight int, displayCapture bool, scalabilityMode string,
	readRTCReport ReadRTCReportFunc, validateFrame validateFrameFunc, p *perf.Values) error {
	if err := WaitForPeerConnectionStabilized(ctx, conn, streamWidth, streamHeight, displayCapture, scalabilityMode, readRTCReport); err != nil {
		return err
	}

	err := MeasureRTCEncodeStats(ctx, conn, readRTCReport, p)
	if err != nil {
		return errors.Wrap(err, "failed measuring webrtc encode stats")
	}
	if err := MeasureRTCDecodeStats(ctx, conn, streamWidth, streamHeight, readRTCReport, validateFrame, p); err != nil {
		return errors.Wrap(err, "failed measuring webrtc decode stats")
	}
	return nil
}
