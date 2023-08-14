// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package webrtc provides functions to measure performance metrics in a video
// conference using WebRTC API.
package webrtc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// VCTestParams is a structure about test parameters.
type VCTestParams struct {
	// Place holder. The parameters e.g. execute JS blurring or platform blurring will be added later.
	placeHolder bool
}

const (
	// VCHTML is the HTML holding a video conference using WebRTC API.
	VCHTML = "webrtc/video_conference.html"

	powerInterval   = 5 * time.Second // Power library metrics collection interval.
	profileInterval = 30 * time.Second
)

// TestFiles returns the files required running the test.
func TestFiles() []string {
	return []string{
		VCHTML,
		"webrtc/video_conference.js",
		"webrtc/third_party/munge_sdp.js",
	}
}

func runVCPerf(ctx context.Context, cs ash.ConnSource, cr *chrome.Chrome,
	s *testing.State, vcURL string, params VCTestParams) error {
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

	conn, err := cs.NewConn(ctx, vcURL)
	if err != nil {
		return errors.Wrapf(err, "failed to open %s", vcURL)
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

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

	if err := r.Start(ctx); err != nil {
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
		testing.ContextLog(ctx, "starting ", step.name)
		if err := conn.Eval(ctx, step.evalStr, nil); err != nil {
			return errors.Wrap(err, step.errMsg)
		}
		testing.ContextLog(ctx, "measuring performance metrics ", step.name)
		// GoBigSleepLint: Sleep to measure the performance metrics
		if err := testing.Sleep(ctx, profileInterval); err != nil {
			return errors.Wrapf(err, "failed to sleep for %v", profileInterval)
		}
	}

	if err := r.Finish(ctx); err != nil {
		return errors.Wrap(err, "cannot finish collecting power metrics")
	}

	return nil
}

// RunVideoConference runs a video conference using WebRTC API and measures the
// performance metrics while enabling features in order.
func RunVideoConference(ctx context.Context, cs ash.ConnSource, cr *chrome.Chrome,
	s *testing.State, params VCTestParams) error {
	const cleanupTime = 5 * time.Second

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()
	vcURL := server.URL + "/" + VCHTML

	ctx, cancel := ctxutil.Shorten(ctx, cleanupTime)
	defer cancel()

	if err := runVCPerf(ctx, cs, cr, s, vcURL, params); err != nil {
		return err
	}

	return nil
}
