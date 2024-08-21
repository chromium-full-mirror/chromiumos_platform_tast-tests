// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package recorderapp

import (
	"context"
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
	// in the test.
	PreloadDataPath *string
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

	// TODO(b/355374546): Handle the preload data.
	return nil
}
