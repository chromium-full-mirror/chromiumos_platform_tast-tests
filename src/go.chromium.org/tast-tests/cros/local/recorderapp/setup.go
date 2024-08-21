// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package recorderapp

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	summaryModelDLC         = "ml-dlc-73caa678-45cb-4007-abb9-f04e431376da"
	titleSuggestionModelDLC = "ml-dlc-ee7c31c2-18e5-405a-b54e-f2607130a15d"
)

// LaunchConfig is the configuration sent to the app to modify the local
// settings before starting the test.
type LaunchConfig struct {
	IncludeSystemAudio        bool `json:"includeSystemAudio"`
	ShowOnboardingDialog      bool `json:"showOnboardingDialog"`
	SpeakerLabelForceEnabled  bool `json:"speakerLabelForceEnabled"`
	SummaryForceEnabled       bool `json:"summaryForceEnabled"`
	TranscriptionForceEnabled bool `json:"transcriptionForceEnabled"`
}

// Setup contains the setup to be configured for the Recorder App before
// starting the test.
type Setup struct {
	// Config is the settings to be configured before starting the app.
	Config LaunchConfig
	// PreloadDataPath is the path containing recording files that will be used
	// in the test. If specify, the file must contain JSON in the same format
	// with `RecordingData`,
	PreloadDataPath string
}

// TokenTimeRange represents the time range inside `RecordingTextToken`.
type TokenTimeRange struct {
	StartMs int64 `json:"startMs"`
	EndMs   int64 `json:"endMs"`
}

// RecordingTextToken represents the tokens in `RecordingData`.
type RecordingTextToken struct {
	Kind         string          `json:"kind"`
	LeadingSpace *bool           `json:"leadingSpace"`
	Text         *string         `json:"text"`
	TimeRange    *TokenTimeRange `json:"timeRange"`
	SpeakerLabel *string         `json:"speakerLabel"`
}

// RecordingData represents the recording data that can be imported to the
// Recorder App for testing.
type RecordingData struct {
	Audio      string               `json:"audio"`
	DurationMs int                  `json:"durationMs"`
	Powers     []int64              `json:"powers"`
	TextTokens []RecordingTextToken `json:"textTokens,omitempty"`
	Title      string               `json:"title"`
}

func ensureModelInstalled(ctx context.Context, setup Setup) error {
	if setup.Config.TranscriptionForceEnabled || setup.Config.SpeakerLabelForceEnabled {
		// Wait until dlc libsoda and libsoda-model-en-us are installed.
		if err := testing.Poll(ctx, a11y.VerifySodaInstalled, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 10 * time.Second}); err != nil {
			return errors.Wrap(err, "failed to wait for libsoda dlc to be installed")
		}
	}

	if setup.Config.SummaryForceEnabled {
		if err := launcher.InstallDlc(ctx, []string{summaryModelDLC, titleSuggestionModelDLC}); err != nil {
			return errors.Wrap(err, "failed to ensure summary models installed")
		}
	}
	return nil
}

func (a *App) performSetup(ctx context.Context, setup Setup) error {
	if err := a.conn.Call(ctx, nil, "TestHelper.configureSettingsForTest", setup.Config); err != nil {
		return errors.Wrap(err, "failed to configure the settings")
	}

	if setup.PreloadDataPath != "" {
		if err := a.ImportRecordingData(ctx, setup.PreloadDataPath); err != nil {
			return errors.Wrap(err, "failed to import recording")
		}
	}
	return nil
}

// ImportRecordingData imports the prepared recording that will be used in the test.
func (a *App) ImportRecordingData(ctx context.Context, dataPath string) error {
	recorderData, err := os.ReadFile(dataPath)
	if err != nil {
		return errors.Wrap(err, "failed to read the prepared data")
	}

	var recorderResult []RecordingData
	if err := json.Unmarshal(recorderData, &recorderResult); err != nil {
		return errors.Wrap(err, "failed to unmarshal the prepared data")
	}

	if err := a.conn.Call(ctx, nil, "TestHelper.importRecordings", recorderResult); err != nil {
		return errors.Wrap(err, "failed to import the recordings")
	}

	return nil
}
