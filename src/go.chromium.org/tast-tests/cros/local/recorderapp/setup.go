// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package recorderapp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"go.chromium.org/tast/core/errors"
)

// TranscriptionLanguage is locale string of the transcription language.
type TranscriptionLanguage string

const (
	// EnUs is locale string of en-US.
	EnUs TranscriptionLanguage = `en-US`
	// JaJp is locale string of ja-JP.
	JaJp TranscriptionLanguage = `ja-JP`
)

// LaunchConfig is the configuration sent to the app to modify the local
// settings before starting the test.
type LaunchConfig struct {
	IncludeSystemAudio        bool                  `json:"includeSystemAudio"`
	ShowOnboardingDialog      bool                  `json:"showOnboardingDialog"`
	SpeakerLabelForceEnabled  bool                  `json:"speakerLabelForceEnabled"`
	SummaryForceEnabled       bool                  `json:"summaryForceEnabled"`
	TranscriptionForceEnabled bool                  `json:"transcriptionForceEnabled"`
	TranscriptionLanguage     TranscriptionLanguage `json:"transcriptionLanguage"`
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
	Audio                 string                `json:"audio"`
	DurationMs            int                   `json:"durationMs"`
	Powers                []int64               `json:"powers"`
	TextTokens            []RecordingTextToken  `json:"textTokens,omitempty"`
	Title                 string                `json:"title"`
	TranscriptionLanguage TranscriptionLanguage `json:"transcriptionLanguage,omitempty"`
}

func (a *App) ensureModelInstalled(ctx context.Context, setup Setup) error {
	if setup.Config.TranscriptionForceEnabled || setup.Config.SpeakerLabelForceEnabled {
		transcriptionLanguage := EnUs
		if setup.Config.TranscriptionLanguage != "" {
			transcriptionLanguage = setup.Config.TranscriptionLanguage
		}
		if err := a.conn.Call(ctx, nil, "TestHelper.installTranscriptionModel", transcriptionLanguage); err != nil {
			return errors.Wrapf(err, "failed to install %s transcription model", transcriptionLanguage)
		}
		if err := a.conn.WaitForExprWithTimeout(ctx, fmt.Sprintf("TestHelper.isTranscriptionModelInstalled('%s')", transcriptionLanguage), time.Minute); err != nil {
			return errors.Wrapf(err, "failed to wait for %s transcription model to be installed", transcriptionLanguage)
		}
	}

	if setup.Config.SummaryForceEnabled {
		// Install summary model.
		if err := a.conn.Eval(ctx, "TestHelper.installSummaryModel()", nil); err != nil {
			return errors.Wrap(err, "failed to install summary model")
		}
		if err := a.conn.WaitForExprWithTimeout(ctx, "TestHelper.isSummaryModelInstalled()", time.Minute); err != nil {
			return errors.Wrap(err, "failed to wait for the summary model to be installed")
		}

		// Install title suggestion model.
		if err := a.conn.Eval(ctx, "TestHelper.installTitleSuggestionModel()", nil); err != nil {
			return errors.Wrap(err, "failed to install title suggestion model")
		}
		if err := a.conn.WaitForExprWithTimeout(ctx, "TestHelper.isTitleSuggestionModelInstalled()", time.Minute); err != nil {
			return errors.Wrap(err, "failed to wait for the title suggestion model to be installed")
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
