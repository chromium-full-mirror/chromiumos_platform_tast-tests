// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package resourced

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/resourced"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SetFullscreenVideo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks resourced setting fullscreen video mode",
		Contacts:     []string{"chromeos-memory@google.com", "vovoy@chromium.org"},
		BugComponent: "b:167286", // ChromeOS > Platform > System > Memory Management
		Attr:         []string{"group:mainline"},
		Timeout:      2 * time.Minute,
	})
}

func SetFullscreenVideo(ctx context.Context, s *testing.State) {
	rm, err := resourced.NewClient(ctx)
	if err != nil {
		s.Fatal("Failed to create Resource Manager client: ", err)
	}

	var newFullscreenVideo uint8 = resourced.FullscreenVideoActive
	var timeout uint32 = 1
	if err := rm.SetFullscreenVideoWithTimeout(ctx, newFullscreenVideo, timeout); err != nil {
		// On machines not supporting Intel hardware EPP, SetFullscreenVideoWithTimeout returning error is expected.
		testing.ContextLog(ctx, "Failed to set full screen video active: ", err)
	}
	testing.ContextLogf(ctx, "Set full screen video active to %d with %d second timeout", newFullscreenVideo, timeout)

	// Check full screen video state is set to the new value.
	fullscreenVideo, err := rm.FullscreenVideo(ctx)
	if err != nil {
		s.Fatal("Failed to query full screen video state: ", err)
	}
	if newFullscreenVideo != fullscreenVideo {
		s.Fatalf("Failed to set full screen video state: got %d, want: %d", fullscreenVideo, newFullscreenVideo)
	}

	// Check full screen video state is reset after timeout.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		fullscreenVideo, err := rm.FullscreenVideo(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to query full screen video state")
		}
		if fullscreenVideo != resourced.FullscreenVideoInactive {
			return errors.New("full screen video state is not reset")
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Duration(2*timeout) * time.Second, Interval: 100 * time.Millisecond}); err != nil {
		s.Fatal("Failed to wait for full screen video state reset: ", err)
	}
}
