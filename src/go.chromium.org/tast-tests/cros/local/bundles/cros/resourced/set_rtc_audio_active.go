// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package resourced

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/resourced"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SetRTCAudioActive,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks resourced setting RTC audio active",
		Contacts:     []string{"chromeos-memory@google.com", "vovoy@chromium.org"},
		BugComponent: "b:167286", // ChromeOS > Platform > System > Memory Management
		Attr:         []string{"group:mainline", "group:criticalstaging", "informational"},
		Timeout:      2 * time.Minute,
	})
}

func SetRTCAudioActive(ctx context.Context, s *testing.State) {
	rm, err := resourced.NewClient(ctx)
	if err != nil {
		s.Fatal("Failed to create Resource Manager client: ", err)
	}

	// Get the original RTC audio active.
	origRTCAudioActive, err := rm.RTCAudioActive(ctx)
	if err != nil {
		s.Fatal("Failed to query RTC audio active: ", err)
	}
	testing.ContextLog(ctx, "Original RTC audio active: ", origRTCAudioActive)

	defer func() {
		// Restore RTC audio active.
		if err = rm.SetRTCAudioActive(ctx, origRTCAudioActive); err != nil {
			testing.ContextLog(ctx, "Failed to reset RTC audio active: ", err)
		}
	}()

	// Set RTC audio active to different value.
	newRTCAudioActive := resourced.RTCAudioActiveOff
	if origRTCAudioActive == resourced.RTCAudioActiveOff {
		newRTCAudioActive = resourced.RTCAudioActiveOn
	}
	if err = rm.SetRTCAudioActive(ctx, newRTCAudioActive); err != nil {
		// On machines not supporting Intel hardware EPP, SetRTCAudioActive returning error is expected.
		testing.ContextLog(ctx, "Failed to set RTC audio active: ", err)
	}
	testing.ContextLog(ctx, "Set RTC audio active: ", newRTCAudioActive)

	// Check RTC audio active is set to the new value.
	rtcAudioActive, err := rm.RTCAudioActive(ctx)
	if err != nil {
		s.Fatal("Failed to query RTC audio active: ", err)
	}
	if newRTCAudioActive != rtcAudioActive {
		s.Fatalf("Failed to set RTC audio active: got %d, want: %d", rtcAudioActive, newRTCAudioActive)
	}
}
