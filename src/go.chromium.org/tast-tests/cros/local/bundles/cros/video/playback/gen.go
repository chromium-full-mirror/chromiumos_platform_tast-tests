// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package playback provides common code for video.Playback* tests.
package playback

import (
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/coords"
)

// ParamData is used for video.Playback* tests to generate parameterized subtests.
type ParamData struct {
	Name string

	// playback.Config
	File                      string
	DecoderType               DecoderType
	BrowserType               string
	Grid                      coords.Size
	PerfMeasurement           bool
	PerfTracing               bool
	MeasureSteadyStateMetrics bool
	MeasureRoughness          bool
	SuspendResume             bool
	Duration                  time.Duration

	SoftwareDeps []string
	HardwareDeps string
	Data         []string
	Attr         []string
	Fixture      string
	ExtraAttr    []string
	Timeout      time.Duration
}

// GenDataPath returns the data path for specific codec/resolution/fps combination.
func GenDataPath(codec string, resolution, fps int) string {
	if fps != 30 && fps != 60 && fps != 120 {
		panic("Unexpected fps")
	}
	numFrames := fps * 10

	extension := ""
	switch codec {
	case "h264", "hevc", "hevc10", "av1":
		extension = codec + ".mp4"
	case "vp8", "vp9":
		extension = codec + ".webm"
	default:
		panic("Unexpected codec")
	}

	return fmt.Sprintf("perf/%s/%dp_%dfps_%dframes.%s",
		codec, resolution, fps, numFrames, extension)
}

// GenSwDeps returns the SoftwareDeps for specific codec/resolution/fps combination.
func GenSwDeps(codec string, resolution, fps int, dec string) []string {
	var swDeps []string

	if codec == "h264" || codec == "hevc" || codec == "hevc10" {
		swDeps = append(swDeps, "proprietary_codecs")
	}

	if dec != "hw" {
		return swDeps
	}

	if fps == 120 {
		fps = 60 // For SwDeps purposes (a.k.a. capabilities), 60 is good enough for 120fps.
	}

	if resolution < 1080 {
		resolution = 1080
	}
	if codec == "hevc10" {
		swDeps = append(swDeps, fmt.Sprintf("autotest-capability:hw_dec_hevc_%d_%d_10bpp", resolution, fps))
	} else {
		swDeps = append(swDeps, fmt.Sprintf("autotest-capability:hw_dec_%s_%d_%d", codec, resolution, fps))
	}
	return swDeps
}
