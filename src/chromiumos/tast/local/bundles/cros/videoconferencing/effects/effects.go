// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package effects contains common code used by the effects tests.
package effects

import (
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/testing"
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
)

const (
	// File watched by EffectsStreamManipulator to configure platform effects.
	platformEffectsOverridePath = "/run/camera/effects/effects_config_override.json"
	platformEffectsOverrideDir  = "/run/camera/effects"
)

// FakeCameraImageConfig represents the config of the path to a real image.
type FakeCameraImageConfig struct {
	Path string `json:"path"`
}

// DataResult returns the result of FPS value measured.
type DataResult struct {
	Average float64   `json:"average"`
	Data    []float64 `json:"data"`
}

// ApplyPlatformEffects applies the configured platform effects.
func ApplyPlatformEffects(ctx context.Context, blur, relight bool) (func(ctx context.Context) error, error) {
	testing.ContextLog(ctx, "Configuring platform effects")
	if err := os.Mkdir(platformEffectsOverrideDir, 0755); err != nil && !os.IsExist(err) {
		return nil, errors.Wrap(err, "failed to write platform override")
	}

	// This configuration format may change, update as needed.
	platformEffects := struct {
		Effect string `json:"effect"`
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
	testing.ContextLog(ctx, "FPS: ", result.Average)
	return result, nil
}
