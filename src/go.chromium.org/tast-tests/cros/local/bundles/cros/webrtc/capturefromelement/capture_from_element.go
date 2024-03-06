// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package capturefromelement provides common code for WebRTC's captureStream()
// tests; this API is used for <video> and <canvas> capture, see e.g.
// https://developer.mozilla.org/en-US/docs/Web/API/HTMLCanvasElement/captureStream
// and https://w3c.github.io/mediacapture-fromelement/.
package capturefromelement

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// htmlFile is the file containing the HTML+JS code exercising captureStream().
	htmlFile = "capturefromelement.html"
)

// CanvasSource defines what is fed to the <canvas> that we can capture from.
type CanvasSource int

const (
	// UseGlClearColor indicates flipping colours using WebGL's clearColor().
	UseGlClearColor CanvasSource = iota
	// UseVideo indicates using a GetUserMedia() captured stream as canvas source.
	UseVideo
)

// TestParam is used to describe the config used to run CaptureFromElement and CaptureFromElementPerf.
type TestParam struct {
	// CanvasSource indicates what is fed to the <canvas> that we can capture from.
	CanvasSource CanvasSource
	// BrowserType indicates the type of Chrome browser to be used,
	// Ash Chrome or Lacros Chrome.
	BrowserType browser.Type
}

// RunCaptureStream drives the code verifying the captureStream() functionality.
// measurementDuration, if != 0, specifies the time to collect performance
// metrics; conversely if == 0 the test instructs JS code to verify the capture
// correctness and finishes immediately.
func RunCaptureStream(ctx context.Context, s *testing.State, cs ash.ConnSource, source CanvasSource, measurementDuration time.Duration) error {
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	conn, err := cs.NewConn(ctx, server.URL+"/"+htmlFile)
	if err != nil {
		return errors.Wrapf(err, "failed to open %v", htmlFile)
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	// Only validate contents if we're not measuring perf.
	validate := measurementDuration == 0
	if source == UseGlClearColor {
		if err := conn.Call(ctx, nil, "captureFromCanvasWithAlternatingColoursAndInspect", validate); err != nil {
			return errors.Wrap(err, "failed to run test HTML")
		}
	} else if source == UseVideo {
		if err := conn.Call(ctx, nil, "captureFromCanvasWithVideoAndInspect", validate); err != nil {
			return errors.Wrap(err, "failed to run test HTML")
		}
	} else {
		return errors.Errorf("unknown CanvasSource: %d", source)
	}
	// If the caller specifies no measurementDuration, then return immediately.
	if validate {
		return nil
	}

	p := graphics.NewThreadSafePerfValues()

	var gpuErr, cStateErr, cpuErr error
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		gpuErr = graphics.MeasureGPUCounters(ctx, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		cStateErr = graphics.MeasurePackageCStateCounters(ctx, measurementDuration, p)
	}()
	go func() {
		defer wg.Done()
		cpuErr = graphics.MeasureCPUUsageAndPower(ctx, 0, measurementDuration, p)
	}()
	wg.Wait()
	if gpuErr != nil {
		return errors.Wrap(gpuErr, "failed to measure GPU counters")
	}
	if cStateErr != nil {
		return errors.Wrap(cStateErr, "failed to measure Package C-State residency")
	}
	if cpuErr != nil {
		return errors.Wrap(cpuErr, "failed to measure CPU/Package power")
	}

	p.GetUnderlyingValues().Save(s.OutDir())
	return nil
}

// DataFiles returns a list of files required to run the tests in this package.
func DataFiles() []string {
	return []string{
		htmlFile,
		"canvas_animation.js",
		"third_party/blackframe.js",
		"third_party/three.js/three.module.js",
	}
}
