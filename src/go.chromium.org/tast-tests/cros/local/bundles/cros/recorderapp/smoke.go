// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package recorderapp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/recorderapp"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Smoke,
		Desc:         "Verify basic functionality of Recorder app",
		Contacts:     []string{"chromeos-recorder-app@google.com", "hsuanling@google.com"},
		BugComponent: "b:1522466", // ChromeOS > Platform > Technologies > Audio > Recorder App
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Microphone()),
		Fixture:      "recorderAppPrepared",
	})
}

func Smoke(ctx context.Context, s *testing.State) {
	// Launch Recorder App
	cr := s.FixtValue().(recorderapp.FixtureData).Chrome

	app, err := recorderapp.StartApp(ctx, cr)
	if err != nil {
		s.Fatal("Failed to launch Recorder App: ", err)
	}

	// Record for 3 seconds
	if err := app.RecordAudio(ctx, 3*time.Second); err != nil {
		s.Fatal("Failed to record audio: ", err)
	}

	// Navigate to main page
	if err := app.GoBackToMainPage()(ctx); err != nil {
		s.Fatal("Failed to go back to main page: ", err)
	}

	// Check if recording is saved
	if err := app.WaitUntilExists(recorderapp.FirstRecordingCard)(ctx); err != nil {
		s.Fatal("Failed to wait for first recording exists: ", err)
	}
}
