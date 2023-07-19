// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cujrecorder

import "go.chromium.org/tast-tests/cros/common/perf"

// MetricInfo records the unit and the direction of the histogram.
type MetricInfo map[string]struct {
	Unit      string
	Direction perf.Direction
}

// WebRTCMetricInfo records the WebRTC metrics.
var WebRTCMetricInfo = MetricInfo{
	"WebRTC.Video.BandwidthLimitedResolutionInPercent":             {"percent", perf.SmallerIsBetter},
	"WebRTC.Video.BandwidthLimitedResolutionsDisabled":             {"count", perf.SmallerIsBetter},
	"WebRTC.Video.CpuLimitedResolutionInPercent":                   {"percent", perf.SmallerIsBetter},
	"WebRTC.Video.DecodedFramesPerSecond":                          {"fps", perf.BiggerIsBetter},
	"WebRTC.Video.DroppedFrames.Capturer":                          {"count", perf.SmallerIsBetter},
	"WebRTC.Video.DroppedFrames.Encoder":                           {"count", perf.SmallerIsBetter},
	"WebRTC.Video.DroppedFrames.EncoderQueue":                      {"count", perf.SmallerIsBetter},
	"WebRTC.Video.DroppedFrames.Ratelimiter":                       {"count", perf.SmallerIsBetter},
	"WebRTC.Video.DroppedFrames.Receiver":                          {"count", perf.SmallerIsBetter},
	"WebRTC.Video.InputFramesPerSecond":                            {"fps", perf.BiggerIsBetter},
	"WebRTC.Video.NumberResolutionDownswitchesPerMinute":           {"count_per_minute", perf.SmallerIsBetter},
	"WebRTC.Video.QualityLimitedResolutionDownscales":              {"count", perf.SmallerIsBetter},
	"WebRTC.Video.QualityLimitedResolutionInPercent":               {"percent", perf.SmallerIsBetter},
	"WebRTC.Video.RenderFramesPerSecond":                           {"fps", perf.BiggerIsBetter},
	"WebRTC.Video.Screenshare.BandwidthLimitedResolutionInPercent": {"percent", perf.SmallerIsBetter},
	"WebRTC.Video.Screenshare.BandwidthLimitedResolutionsDisabled": {"count", perf.SmallerIsBetter},
	"WebRTC.Video.Screenshare.InputFramesPerSecond":                {"fps", perf.BiggerIsBetter},
	"WebRTC.Video.Screenshare.QualityLimitedResolutionDownscales":  {"count", perf.SmallerIsBetter},
	"WebRTC.Video.Screenshare.QualityLimitedResolutionInPercent":   {"percent", perf.SmallerIsBetter},
	"WebRTC.Video.Screenshare.SentFramesPerSecond":                 {"fps", perf.BiggerIsBetter},
	"WebRTC.Video.Screenshare.SentToInputFpsRatioPercent":          {"percent", perf.BiggerIsBetter},
	"WebRTC.Video.SentFramesPerSecond":                             {"fps", perf.BiggerIsBetter},
	"WebRTC.Video.SentToInputFpsRatioPercent":                      {"percent", perf.BiggerIsBetter},
	"WebRTC.Video.TimeInHdPercentage":                              {"percent", perf.BiggerIsBetter},
}

// WebRTCMetrics returns WebRTC common metrics which are required to be collected by conference CUJ tests.
func WebRTCMetrics() []MetricConfig {
	var configs []MetricConfig
	for name, info := range WebRTCMetricInfo {
		configs = append(configs, NewCustomMetricConfig(name, info.Unit, info.Direction))
	}
	return configs
}
