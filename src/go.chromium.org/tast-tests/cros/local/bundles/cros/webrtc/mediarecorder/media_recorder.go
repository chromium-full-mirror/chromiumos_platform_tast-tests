// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mediarecorder provides common code for video.MediaRecorder tests.
package mediarecorder

import (
	"context"
	"encoding/base64"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"time"

	"github.com/pixelbender/go-matroska/matroska"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/media/constants"
	mediacpu "go.chromium.org/tast-tests/cros/local/media/cpu"
	"go.chromium.org/tast-tests/cros/local/media/histogram"
	"go.chromium.org/tast-tests/cros/local/media/videotype"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	stabilizationDuration = 5 * time.Second
	measurementDuration   = 15 * time.Second
)

func reportMetric(name, unit string, value float64, direction perf.Direction, p *perf.Values) {
	p.Set(perf.Metric{
		Name:      name,
		Unit:      unit,
		Direction: direction,
	}, value)
}

func codecProfileMime(profile videotype.CodecProfile) (string, error) {
	switch profile {
	case videotype.H264BaselineProf:
		return "avc1.", nil
	case videotype.H264HighProf:
		// https://www.rfc-editor.org/rfc/rfc6381
		// H264 High profile level 4.0 for 1080p@30 encoding.
		return "avc1.640028", nil
	case videotype.VP8Prof:
		return "vp8", nil
	case videotype.VP9Prof:
		return "vp9", nil
	case videotype.AV1MainProf:
		return "av01", nil
	default:
		return "", errors.Errorf("failed to codec meme type: %v", profile)

	}
}

// MeasurePerf measures the frame processing time and CPU usage while recording and report the results.
func MeasurePerf(ctx context.Context, cs ash.ConnSource, tconn, bTconn *chrome.TestConn, fileSystem http.FileSystem, outDir string, profile videotype.CodecProfile, resolution graphics.Size, hwAccelEnabled bool) error {
	codecMeme, err := codecProfileMime(profile)
	if err != nil {
		return err
	}

	// Wait until CPU is idle enough. CPU usage can be high immediately after login for various reasons (e.g. animated images on the lock screen).
	cleanUpBenchmark, err := mediacpu.SetUpBenchmark(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to set up CPU benchmark")
	}
	defer cleanUpBenchmark(ctx)

	// Reserve time for cleanup at the end of the test.
	const cleanupTime = 10 * time.Second
	ctx, cancel := ctxutil.Shorten(ctx, cleanupTime)
	defer cancel()

	if err := cpu.WaitUntilIdle(ctx); err != nil {
		return errors.Wrap(err, "failed waiting for CPU to become idle")
	}

	server := httptest.NewServer(http.FileServer(fileSystem))
	defer server.Close()

	initHistogram, err := metrics.GetHistogram(ctx, bTconn, constants.MediaRecorderVEAUsed)
	if err != nil {
		return errors.Wrap(err, "failed to get initial histogram")
	}

	conn, err := cs.NewConn(ctx, server.URL+"/loopback_media_recorder.html")
	if err != nil {
		return errors.Wrap(err, "failed to open recorder page")
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	if err := conn.WaitForExpr(ctx, "pageLoaded"); err != nil {
		return errors.Wrap(err, "timed out waiting for page loading")
	}
	// startRecording() a video in given format until stopRecording() is called.
	if err := conn.Call(ctx, nil, "startRecording", codecMeme, resolution.Width, resolution.Height); err != nil {
		return errors.Wrapf(err, "failed to evaluate startRecording(%v)", profile)
	}

	p := graphics.NewThreadSafePerfValues()
	var gpuErr, cpuErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		gpuErr = graphics.MeasureGPUCounters(ctx, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		cpuErr = graphics.MeasureCPUUsageAndPower(ctx, stabilizationDuration, measurementDuration, p)
	}()
	wg.Wait()
	if gpuErr != nil {
		return errors.Wrap(gpuErr, "failed to measure GPU counters")
	}
	if cpuErr != nil {
		return errors.Wrap(cpuErr, "failed to measure CPU/Package power")
	}

	// Recorded video will be saved in videoBuffer in base64 format.
	var videoBuffer string
	if err := conn.Eval(ctx, "stopRecording()", &videoBuffer); err != nil {
		return errors.Wrap(err, "failed to stop recording")
	}

	hwAccelUsed, err := histogram.WasHWAccelUsed(ctx, bTconn, initHistogram, constants.MediaRecorderVEAUsed, int64(constants.MediaRecorderVEAUsedSuccess), histogram.SuccessCountAtLeastOne)
	if err != nil {
		return errors.Wrap(err, "failed to get histogram")
	}
	if hwAccelEnabled {
		if !hwAccelUsed {
			return errors.Wrap(err, "Hw accelerator requested but not used")
		}
	} else {
		if hwAccelUsed {
			return errors.Wrap(err, "Hw accelerator not requested but used")
		}
	}

	processingTimePerFrame, err := calculateTimePerFrame(ctx, conn, videoBuffer, outDir)
	if err != nil {
		return errors.Wrap(err, "failed to calculate the processig time per frame")
	}
	reportMetric("frame_processing_time", "millisecond", float64(processingTimePerFrame.Milliseconds()), perf.SmallerIsBetter, p.GetUnderlyingValues())

	if err := p.GetUnderlyingValues().Save(outDir); err != nil {
		return errors.Wrap(err, "failed to store performance data")
	}
	return nil
}

func calculateTimePerFrame(ctx context.Context, conn *chrome.Conn, videoBuffer, outDir string) (timePerFrame time.Duration, err error) {
	elapsedTimeMs := 0
	if err := conn.Eval(ctx, "elapsedTime", &elapsedTimeMs); err != nil {
		return 0, errors.Wrap(err, "failed to evaluate elapsedTime")
	}

	videoBytes, err := base64.StdEncoding.DecodeString(videoBuffer)
	if err != nil {
		return 0, errors.Wrap(err, "failed to decode base64 string into byte array")
	}

	frames := 0
	if frames, err = computeNumFrames(videoBytes, outDir); err != nil {
		return 0, errors.Wrap(err, "failed to compute number of frames")
	}

	return time.Duration(elapsedTimeMs/frames) * time.Millisecond, nil
}

// computeNumFrames computes number of frames in the given MKV video byte array.
func computeNumFrames(videoBytes []byte, tmpDir string) (frameNum int, err error) {
	videoFilePath := filepath.Join(tmpDir, "recorded_video.mkv")
	if err := ioutil.WriteFile(videoFilePath, videoBytes, 0644); err != nil {
		return 0, errors.Wrap(err, "failed to open file")
	}

	doc, err := matroska.Decode(videoFilePath)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse video file")
	}

	videoTrackNum := 0
VideoTrackNumLoop:
	for _, track := range doc.Segment.Tracks {
		for _, entry := range track.Entries {
			if entry.Type == matroska.TrackTypeVideo {
				videoTrackNum = int(entry.Number)
				break VideoTrackNumLoop
			}
		}
	}

	frameNum = 0
	for _, cluster := range doc.Segment.Cluster {
		for _, block := range cluster.SimpleBlock {
			if int(block.TrackNumber) != videoTrackNum {
				continue
			}
			if (block.Flags & matroska.LacingNone) != 0 {
				frameNum++
			} else {
				frameNum += (block.Frames + 1)
			}
		}
		for _, blockGroup := range cluster.BlockGroup {
			if int(blockGroup.Block.TrackNumber) != videoTrackNum {
				continue
			}
			if (blockGroup.Block.Flags & matroska.LacingNone) != 0 {
				frameNum++
			} else {
				frameNum += (blockGroup.Block.Frames + 1)
			}
		}
	}

	return frameNum, nil
}

// VerifyMediaRecorderUsesEncodeAccelerator checks whether MediaRecorder uses HW encoder for codec and resolution
func VerifyMediaRecorderUsesEncodeAccelerator(ctx context.Context, cs ash.ConnSource, tconn, bTconn *chrome.TestConn, fileSystem http.FileSystem, profile videotype.CodecProfile, resolution graphics.Size, recordTime time.Duration) error {
	server := httptest.NewServer(http.FileServer(fileSystem))
	defer server.Close()

	// Real webcams on tablets might capture a rotated feed and make one of the
	// dimensions too large for the hardware encoder, see crbug.com/1071979. Set
	// the device in landscape mode to match the expected video call orientation.
	tabletModeEnabled, err := ash.TabletModeEnabled(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get tablet mode")
	}
	if tabletModeEnabled {
		dispInfo, err := display.GetInternalInfo(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get internal display info")
		}
		// Ideally we'd use screen.orientation.lock("landscape"), but that needs the
		// content to be in full screen (requestFullscreen()), which needs a user
		// gesture. Instead, implement the algorithm: landscape is, by definition,
		// when the screen's width is larger than the height, see
		// https://w3c.github.io/screen-orientation/#dfn-landscape-primary
		var width, height int64
		if err := tconn.Eval(ctx, "window.screen.width", &width); err != nil {
			return errors.Wrap(err, "failed to retrieve screen width")
		}
		if err := tconn.Eval(ctx, "window.screen.height", &height); err != nil {
			return errors.Wrap(err, "failed to retrieve screen height")
		}
		rotation := display.Rotate0
		if height > width {
			rotation = display.Rotate90
		}

		if err := display.SetDisplayRotationSync(ctx, tconn, dispInfo.ID, rotation); err != nil {
			return errors.Wrap(err, "failed to rotate display")
		}
	}

	codecMeme, err := codecProfileMime(profile)
	if err != nil {
		return err
	}

	initHistogram, err := metrics.GetHistogram(ctx, bTconn, constants.MediaRecorderVEAUsed)
	if err != nil {
		return errors.Wrap(err, "failed to get initial histogram")
	}

	conn, err := cs.NewConn(ctx, server.URL+"/loopback_media_recorder.html")
	if err != nil {
		return errors.Wrap(err, "failed to open recorder page")
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	if err := conn.WaitForExpr(ctx, "pageLoaded"); err != nil {
		return errors.Wrap(err, "timed out waiting for page loading")
	}

	// GoBigSleepLint: Hardware encoding is not available right after login on
	// some devices (e.g. krane) so the encoding capabilities are enumerated
	// asynchronously, see b/147404923. Sadly, MediaRecorder doesn't know about
	// this and this code is racy. Insert a sleep() temporarily until Blink code
	// is fixed: b/158858449.
	testing.Sleep(ctx, 2*time.Second)

	if err := conn.Call(ctx, nil, "startRecordingForResult", codecMeme, resolution.Width, resolution.Height, recordTime.Milliseconds()); err != nil {
		return errors.Wrapf(err, "failed to evaluate startRecordingForResult(%q, %d)", profile, recordTime.Milliseconds())
	}

	if hwUsed, err := histogram.WasHWAccelUsed(ctx, bTconn, initHistogram, constants.MediaRecorderVEAUsed, int64(constants.MediaRecorderVEAUsedSuccess), histogram.SuccessCountAtLeastOne); err != nil {
		return errors.Wrap(err, "failed to verify histogram")
	} else if !hwUsed {
		return errors.New("hardware accelerator was not used")
	}
	return nil
}
