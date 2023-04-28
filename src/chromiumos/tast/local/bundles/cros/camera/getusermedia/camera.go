// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package getusermedia

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"chromiumos/tast/common/perf"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
)

// RunTest checks if the given WebRTC tests work correctly.
// htmlName is a filename of an HTML file in data directory.
// entryPoint is a JavaScript expression that starts the test there.
func RunTest(ctx context.Context, fileSystem http.FileSystem, cr ChromeInterface,
	htmlName, entryPoint string, results, logs interface{}) error {

	server := httptest.NewServer(http.FileServer(fileSystem))
	defer server.Close()

	conn, err := cr.NewConn(ctx, server.URL+"/"+htmlName)
	if err != nil {
		return errors.Wrap(err, "faild to create renderer")
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	if err := conn.WaitForExpr(ctx, "scriptReady"); err != nil {
		return errors.Wrap(err, "timed out waiting for scripts ready")
	}

	if err := conn.WaitForExpr(ctx, "checkVideoInput()"); err != nil {
		var msg string
		subErr := error(nil)
		if err2 := conn.Eval(ctx, "enumerateDevicesError", &msg); err2 != nil {
			subErr = errors.Wrap(err2, "failed to evaluate enumerateDevicesError")
		} else if len(msg) > 0 {
			subErr = errors.Errorf("enumerateDevices failed: %s", msg)
		}
		if subErr != nil {
			return errors.Errorf("Timed out waiting for video device to be available: %v and %v", err, subErr)
		}
		return errors.Wrap(err, "timed out waiting for video device to be available")
	}

	if err := conn.Eval(ctx, entryPoint, nil); err != nil {
		return errors.Wrap(err, "failed to start test")
	}

	rctx, rcancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer rcancel()
	if err := conn.WaitForExpr(rctx, "isTestDone"); err != nil {
		// If test didn't finish within the deadline, display error messages stored in "globalErrors".
		var globalerrors []string
		subErr := error(nil)
		if err2 := conn.Eval(ctx, "globalErrors", &globalerrors); err2 == nil {
			for _, msg := range globalerrors {
				subErr = errors.Wrapf(err2, "got JS error: %s", msg)
			}
		}
		if subErr != nil {
			return errors.Errorf("Timed out waiting for test completed: %v and %v", err, subErr)
		}
		return errors.Wrap(err, "timed out waiting for test completed")
	}

	if err := conn.Eval(ctx, "getResults()", results); err != nil {
		return errors.Wrap(err, "failed to get results from JS")
	}

	if err := conn.Eval(ctx, "getLogs()", logs); err != nil {
		return errors.Wrap(err, "failed to get logs from JS")
	}
	return nil
}

func percentage(num, total int) float64 {
	if total == 0 {
		return 100.0
	}
	return 100.0 * float64(num) / float64(total)
}

// FrameStats is a struct for statistics of frames.
type FrameStats struct {
	TotalFrames  int `json:"totalFrames"`
	BlackFrames  int `json:"blackFrames"`
	FrozenFrames int `json:"frozenFrames"`
}

// blackFramesPercentage returns the ratio of black frames to total frames
func (s *FrameStats) blackFramesPercentage() float64 {
	return percentage(s.BlackFrames, s.TotalFrames)
}

// frozenFramesPercentage returns the ratio of frozen frames to total frames
func (s *FrameStats) frozenFramesPercentage() float64 {
	return percentage(s.FrozenFrames, s.TotalFrames)
}

// CheckTotalFrames checks whether video frames were displayed.
func (s *FrameStats) CheckTotalFrames() error {
	if s.TotalFrames == 0 {
		return errors.New("no frame was displayed")
	}
	return nil
}

// CheckBrokenFrames checks that there were less than threshold frozen or black
// frames. This test might be too strict for real cameras, but should work fine
// with the Fake video/audio capture device that should be used for WebRTC
// tests.
func (s *FrameStats) CheckBrokenFrames() error {
	const threshold = 1.0
	blackPercentage := s.blackFramesPercentage()
	frozenPercentage := s.frozenFramesPercentage()
	if threshold < blackPercentage+frozenPercentage {
		return errors.Errorf("too many broken frames: black %.1f%%, frozen %.1f%% (total %d)",
			blackPercentage, frozenPercentage, s.TotalFrames)
	}
	return nil
}

// SetPerf records performance data in FrameStats to perf.Values.
// p is a pointer for perf.Values where data will be stored.
// suffix is a string that will be used as sufixes of metrics' names.
func (s *FrameStats) SetPerf(p *perf.Values, suffix string) {
	blackFrames := perf.Metric{
		Name:      "tast_black_frames_percentage_" + suffix,
		Unit:      "percent",
		Direction: perf.SmallerIsBetter,
	}
	frozenFrames := perf.Metric{
		Name:      "tast_frozen_frames_percentage_" + suffix,
		Unit:      "percent",
		Direction: perf.SmallerIsBetter,
	}

	p.Set(blackFrames, s.blackFramesPercentage())
	p.Set(frozenFrames, s.frozenFramesPercentage())
}
