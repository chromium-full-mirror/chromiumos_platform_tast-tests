// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package effects contains common code used by the effects tests.
package effects

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/power"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// File watched by EffectsStreamManipulator to configure platform effects.
	platformEffectsOverridePath = "/run/camera/effects/effects_config_override.json"
	platformEffectsOverrideDir  = "/run/camera/effects"
	// OpenCLCacheDir is the path to the opencl_cache directory.
	OpenCLCacheDir = "/var/lib/ml_core/opencl_cache"
)

// DataResult returns the result of frame metrics measured.
type DataResult struct {
	FpsAverage   float64   `json:"fpsAverage"`
	DurationData []float64 `json:"durationData"`
	FpsData      []float64 `json:"fpsData"`
}

// ModelType is an enum to select the segmentation model type.
type ModelType string

const (
	// KNone is the empty string used for no effects.
	KNone ModelType = ""
	// KAuto is the string used for the system default model.
	KAuto ModelType = "auto"
	// KHd is the standard "HD" model.
	KHd ModelType = "hd"
	// KFull is the standard "low res" model.
	KFull ModelType = "full"
	// KEffnet256 is a 256x160 resolution model.
	KEffnet256 ModelType = "effnet256"
	// KEffnet384 is a 384x224 resolution model.
	KEffnet384 ModelType = "effnet384"
)

// ApplyPlatformEffects applies the configured platform effects.
func ApplyPlatformEffects(ctx context.Context, blur, relight bool, modelType ModelType) (func(ctx context.Context) error, error) {
	testing.ContextLog(ctx, "Configuring platform effects")
	if err := os.Mkdir(platformEffectsOverrideDir, 0755); err != nil && !os.IsExist(err) {
		return nil, errors.Wrap(err, "failed to write platform override")
	}

	// This configuration format may change, update as needed.
	platformEffects := struct {
		Effect                string `json:"effect"`
		SegmentationModelType string `json:"segmentation_model_type"`
		GpuAPI                string `json:"gpu_api"`
	}{
		Effect: "none",
	}
	if blur && relight {
		platformEffects.Effect = "blur_relight"
	} else if blur {
		platformEffects.Effect = "blur"
	} else if relight {
		platformEffects.Effect = "relight"
	}

	platformEffects.GpuAPI = "vulkan"
	if modelType == KAuto {
		platformEffects.SegmentationModelType = string(KAuto)
	} else if modelType == KHd {
		platformEffects.SegmentationModelType = string(KHd)
	} else if modelType == KEffnet256 {
		platformEffects.SegmentationModelType = string(KEffnet256)
	} else if modelType == KEffnet384 {
		platformEffects.SegmentationModelType = string(KEffnet384)
	} else if modelType == KFull {
		platformEffects.SegmentationModelType = string(KFull)
		platformEffects.GpuAPI = "opencl"
	} else if modelType == KNone {
		// Do nothing.
	} else {
		return nil, errors.Wrap(errors.New("invalid model config"), "invalid model config")
	}

	platformEffectsJSON, err := json.Marshal(platformEffects)
	if err != nil {
		return nil, errors.Wrap(err, "failed to serialize platform config")
	}
	if err := ioutil.WriteFile(platformEffectsOverridePath, platformEffectsJSON, 0644); err != nil {
		return nil, errors.Wrap(err, "failed to write platform config")
	}
	cleanup := func(ctx context.Context) error {
		return os.Remove(platformEffectsOverridePath)
	}
	testing.ContextLog(ctx, "Platform effects configured: ", string(platformEffectsJSON))

	return cleanup, nil
}

// CaptureFPSData records the current FPS and returns the data.
func CaptureFPSData(ctx context.Context, conn *browser.Conn, file string, seconds int) (DataResult, error) {
	testing.ContextLog(ctx, "Start capturing FPS over ", seconds, " seconds")
	var result DataResult
	script, err := ioutil.ReadFile(file)
	if err != nil {
		return result, errors.Wrap(err, "failed to read FPS script")
	}
	if err := conn.Call(ctx, &result, string(script), seconds); err != nil {
		return result, errors.Wrap(err, "failed to call FPS script")
	}
	testing.ContextLog(ctx, "FPS Average: ", result.FpsAverage)
	return result, nil
}

// ReportFramePerfMetrics reports FPS and Frame duration metrics.
func ReportFramePerfMetrics(ctx context.Context, p *perf.Values, conn *browser.Conn, file string, testDuration int) error {
	results, err := CaptureFPSData(ctx, conn, file, testDuration)
	if err != nil {
		return errors.Wrap(err, "failed to run FPS script")
	}

	p.Set(perf.Metric{
		Name:      "FPS_average",
		Unit:      "fps",
		Direction: perf.BiggerIsBetter,
		Multiple:  false},
		results.FpsAverage)

	var percentileMap map[int]float64
	if percentileMap, err = CalculatePercentileData(ctx, results.FpsData); err != nil {
		return errors.Wrap(err, "failed to capture fps data")

	}
	for percentile, value := range percentileMap {
		p.Set(perf.Metric{
			Name:      "FPS_p" + strconv.Itoa(percentile),
			Unit:      "fps",
			Direction: perf.BiggerIsBetter,
			Multiple:  false,
		}, value)
	}

	if percentileMap, err = CalculatePercentileData(ctx, results.DurationData); err != nil {
		return errors.Wrap(err, "failed to capture frame duration data")

	}
	for percentile, value := range percentileMap {
		p.Set(perf.Metric{
			Name:      "Duration_p" + strconv.Itoa(percentile),
			Unit:      "ms",
			Direction: perf.SmallerIsBetter,
			Multiple:  false,
		}, value)
	}

	return nil
}

// ReportMemoryMetrics reports peak memory usage.
func ReportMemoryMetrics(ctx context.Context, p *perf.Values, memoryChannel chan PeakMemoryResult) error {
	var err error
	peakMemoryUsage := <-memoryChannel
	if peakMemoryUsage.Err != nil {
		return errors.Wrap(err, "memory capture failed")
	}

	p.Set(perf.Metric{
		Name:      "PeakMemoryUsage",
		Unit:      "Byte",
		Direction: perf.SmallerIsBetter,
		Multiple:  false},
		float64(peakMemoryUsage.Value))
	testing.ContextLog(ctx, "max Memory usage: ", peakMemoryUsage.Value)

	return nil
}

// ReportPowerDiffMetrics reports changes since initial rapl snapshot.
func ReportPowerDiffMetrics(ctx context.Context, p *perf.Values, raplEnergyBefore *power.RAPLSnapshot, testDuration int) error {
	energyDiff, err := raplEnergyBefore.DiffWithCurrentRAPL()
	if err != nil {
		return errors.Wrap(err, "failed to get RAPL power usage difference")
	}
	energyDiff.ReportPerfMetrics(p, "joules-")
	energyDiff.ReportWattPerfMetrics(p, "watts-", time.Duration(testDuration)*time.Second)

	return nil
}

// ClearOpenCLCache clears only the files in the opencl_cache directory.
func ClearOpenCLCache() error {
	files, err := os.ReadDir(OpenCLCacheDir)
	if err != nil {
		return errors.Wrap(err, "failed to read opencl_cache directory")
	}

	for _, file := range files {
		if err := os.Remove(OpenCLCacheDir + "/" + file.Name()); err != nil {
			return errors.Wrap(err, "failed to remove file "+file.Name())
		}
	}
	return nil
}
