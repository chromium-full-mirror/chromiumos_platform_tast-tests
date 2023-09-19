// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/perf"
	cp "go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast-tests/cros/local/chrome"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// WebRTCMetrics records the WebRTC stat in a page.
type WebRTCMetrics struct {
	conn    *chrome.Conn
	metrics map[string]perf.Metric
}

// Assert that WebRTCMetrics can be used in perf.Timeline.
var _ perf.TimelineDatasource = &WebRTCMetrics{}

// NewWebRTCMetrics creates the struct to store WebRTC metrics.
func NewWebRTCMetrics(conn *chrome.Conn) *WebRTCMetrics {
	newMetrics := &WebRTCMetrics{
		conn:    conn,
		metrics: make(map[string]perf.Metric),
	}
	return newMetrics
}

// Setup creates metrics.
func (v *WebRTCMetrics) Setup(ctx context.Context, prefix, intervalName string) error {
	v.metrics["bitrate"] = perf.Metric{
		Name:      prefix + cp.WebrtcBitrateMetricType + "bitrate",
		Unit:      cp.WebrtcBitrateMetricTypeUnit,
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
		Interval:  intervalName,
	}
	v.metrics["frame_encode_time"] = perf.Metric{
		Name:      prefix + cp.WebrtcTimeMetricType + "frame_encode_time",
		Unit:      cp.WebrtcTimeMetricTypeUnit,
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
		Interval:  intervalName,
	}
	v.metrics["packet_send_delay"] = perf.Metric{
		Name:      prefix + cp.WebrtcTimeMetricType + "packet_send_delay",
		Unit:      cp.WebrtcTimeMetricTypeUnit,
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
		Interval:  intervalName,
	}
	v.metrics["out_fps"] = perf.Metric{
		Name:      prefix + cp.WebrtcFpsMetricType + "out_fps",
		Unit:      cp.WebrtcFpsMetricTypeUnit,
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
		Interval:  intervalName,
	}
	v.metrics["src_fps"] = perf.Metric{
		Name:      prefix + cp.WebrtcFpsMetricType + "src_fps",
		Unit:      cp.WebrtcFpsMetricTypeUnit,
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
		Interval:  intervalName,
	}
	v.metrics["out_width"] = perf.Metric{
		Name:      prefix + cp.WebrtcPixelMetricType + "out_width",
		Unit:      cp.WebrtcPixelMetricTypeUnit,
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
		Interval:  intervalName,
	}
	v.metrics["out_height"] = perf.Metric{
		Name:      prefix + cp.WebrtcPixelMetricType + "out_height",
		Unit:      cp.WebrtcPixelMetricTypeUnit,
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
		Interval:  intervalName,
	}
	v.metrics["src_width"] = perf.Metric{
		Name:      prefix + cp.WebrtcPixelMetricType + "src_width",
		Unit:      cp.WebrtcPixelMetricTypeUnit,
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
		Interval:  intervalName,
	}
	v.metrics["src_height"] = perf.Metric{
		Name:      prefix + cp.WebrtcPixelMetricType + "src_height",
		Unit:      cp.WebrtcPixelMetricTypeUnit,
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
		Interval:  intervalName,
	}
	v.metrics["qp"] = perf.Metric{
		Name:      prefix + cp.WebrtcQPMetricType + "qp",
		Unit:      cp.WebrtcQPMetricTypeUnit,
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
		Interval:  intervalName,
	}
	v.metrics["limitation_none"] = perf.Metric{
		Name:      prefix + cp.WebrtcLimitationMetricType + "limitation_none",
		Unit:      cp.WebrtcLimitationMetricTypeUnit,
		Direction: perf.BiggerIsBetter,
		Multiple:  true,
		Interval:  intervalName,
	}
	v.metrics["limitation_other"] = perf.Metric{
		Name:      prefix + cp.WebrtcLimitationMetricType + "limitation_other",
		Unit:      cp.WebrtcLimitationMetricTypeUnit,
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
		Interval:  intervalName,
	}
	v.metrics["limitation_cpu"] = perf.Metric{
		Name:      prefix + cp.WebrtcLimitationMetricType + "limitation_cpu",
		Unit:      cp.WebrtcLimitationMetricTypeUnit,
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
		Interval:  intervalName,
	}
	v.metrics["limitation_bandwidth"] = perf.Metric{
		Name:      prefix + cp.WebrtcLimitationMetricType + "limitation_bandwidth",
		Unit:      cp.WebrtcLimitationMetricTypeUnit,
		Direction: perf.SmallerIsBetter,
		Multiple:  true,
		Interval:  intervalName,
	}
	return nil
}

// Start logs the start of WebRTC stats tracker.
func (v *WebRTCMetrics) Start(ctx context.Context) error {
	testing.ContextLog(ctx, "Start tracking WebRTC stats")
	return nil
}

// Snapshot takes one snapshot of WebRTC stats.
func (v *WebRTCMetrics) Snapshot(ctx context.Context, values *perf.Values) error {
	const js = "window.outboundWebRTCStats"
	var nameMap = map[string]string{
		"bitRateKbps":        "bitrate",
		"frameEncodeTimeMs":  "frame_encode_time",
		"packetSendDelayMs":  "packet_send_delay",
		"framesPerSecond":    "out_fps",
		"srcFramesPerSecond": "src_fps",
		"frameWidth":         "out_width",
		"frameHeight":        "out_height",
		"srcWidth":           "src_width",
		"srcHeight":          "src_height",
		"qp":                 "qp",
	}
	var limitationValues = map[string]float64{
		"limitation_none":      0,
		"limitation_other":     0,
		"limitation_cpu":       0,
		"limitation_bandwidth": 0,
	}
	var stats map[string]interface{}
	if err := v.conn.Eval(ctx, js, &stats); err != nil {
		return err
	}

	// parse float stats
	for jsKey, metricsKey := range nameMap {
		stat := stats[jsKey]
		floatStat, ok := stat.(float64)
		if !ok {
			return errors.Errorf("Metrics %s from js %s has data %v which is not float64", metricsKey, jsKey, stat)
		}
		values.Append(v.metrics[metricsKey], floatStat)
	}

	// parse limitation stat
	limitation := stats["qualityLimitationReason"]
	limitationStr, ok := limitation.(string)
	if !ok {
		return errors.Errorf("Metrics qualityLimitationReason has data %v which is not string", limitation)
	}

	switch limitationStr {
	case "none":
		limitationValues["limitation_none"] = 100
	case "cpu":
		limitationValues["limitation_cpu"] = 100
	case "bandwidth":
		limitationValues["limitation_bandwidth"] = 100
	default:
		limitationValues["limitation_other"] = 100
	}
	for metricsKey, percentStat := range limitationValues {
		values.Append(v.metrics[metricsKey], percentStat)
	}

	return nil
}

// Stop logs the stop of WebRTC stats tracker.
// This function is required by perf.Timeline. It does not need to make another snapshot.
func (v *WebRTCMetrics) Stop(ctx context.Context, values *perf.Values) error {
	testing.ContextLog(ctx, "Stop tracking WebRTC stats")
	return nil
}
