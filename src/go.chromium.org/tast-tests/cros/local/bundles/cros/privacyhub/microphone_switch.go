// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package privacyhub contains tests for privacy hub.
package privacyhub

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/privacyhub/privacyhubutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MicrophoneSwitch,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that PrivacyHub microphone toggle switches off the audio input",
		Contacts:     []string{"chromeos-privacyhub@google.com", "kisliaks@google.com"},
		// ChromeOS > Privacy > ChromeOS Privacy Feature Development.
		BugComponent: "b:1178745",
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Attr:         []string{"group:mainline", "informational"},
		HardwareDeps: hwdep.D(hwdep.Microphone()),
	})
}

func MicrophoneSwitch(ctx context.Context, s *testing.State) {
	// Shorten deadline to leave time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, chrome.EnableFeatures("CrosPrivacyHub"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	settings, err := ossettings.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch OS settings: ", err)
	}
	defer settings.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to connect to cras: ", err)
	}

	s.Log("Enabling the microphone in Privacy Hub")
	if err := privacyhubutil.SetToggleState(ctx, tconn, "Microphone", true); err != nil {
		s.Fatal("Failed to switch the microphone toggle in the Privacy Hub: ", err)
	}
	if enabled, err := privacyhubutil.IsToggleOn(ctx, tconn, "Microphone"); err != nil {
		s.Fatal("Failed to read the microphone toggle state: ", err)
	} else if !enabled {
		s.Fatal("Failed to enable the microphone")
	}

	if volState, err := cras.GetVolumeState(ctx); err != nil {
		s.Fatal("Failed to get volume state")
	} else if volState.InputMute {
		s.Fatal("Microphone is muted in the system (but shall not be)")
	}

	s.Log("Disabling the microphone in Privacy Hub")
	if err := privacyhubutil.ClickToggle(ctx, tconn, "Microphone"); err != nil {
		s.Fatal("Failed to switch the microphone toggle in the Privacy Hub: ", err)
	}
	if enabled, err := privacyhubutil.IsToggleOn(ctx, tconn, "Microphone"); err != nil {
		s.Fatal("Failed to read the microphone toggle state: ", err)
	} else if enabled {
		s.Fatal("Failed to disable the microphone")
	}

	if volState, err := cras.GetVolumeState(ctx); err != nil {
		s.Fatal("Failed to get volume state")
	} else if !volState.InputMute {
		s.Fatal("Microphone is not muted in the system (but shall be)")
	}
}
