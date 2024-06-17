// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package touchpad

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FlexCheck,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify Flex touchpad requirements",
		Contacts:     []string{"jdenose@google.com"},
		BugComponent: "b:998633", // ChromeOS > Platform > Enablement > ChromeOS Flex
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational"},
		HardwareDeps: hwdep.D(hwdep.Touchpad()),
	})
}

func FlexCheck(ctx context.Context, s *testing.State) {
	// Validate if the device's touchpad is properly recognized by the OS
	if hasPhysicalTouchpad, _, _ := input.FindPhysicalTrackpad(ctx); !hasPhysicalTouchpad {
		s.Fatal("Device touchpad was not detected")
	}

	tpw, err := input.Trackpad(ctx)
	if err != nil {
		s.Fatal("Error creating touchpad device: ", err)
	}
	defer tpw.Close(ctx)

	tw, err := tpw.NewMultiTouchWriter(2)
	if err != nil {
		s.Fatal("Failed to create a multi touch writer: ", err)
	}
	defer tw.Close()

	// Validate left click
	if err := tpw.PressButton(input.BTN_LEFT); err != nil {
		s.Fatal("Failed to perform left click: ", err)
	}

	// Validate right click
	if err := tpw.PressButton(input.BTN_RIGHT); err != nil {
		s.Fatal("Failed to perform right click: ", err)
	}

	// Validate two finger swipe
	fingerSpacing := tpw.Width() / 16
	fingerDistance := fingerSpacing * 4

	x := tpw.Width() / 2
	y1 := tpw.Height() - fingerDistance

	if err := tw.Swipe(ctx, x, 0, x, y1, fingerSpacing, 0, 2, time.Second); err != nil {
		s.Fatal("Failed to perform two finger swipe: ", err)
	}

	if err := tw.End(); err != nil {
		s.Fatal("Failed to finish touchpad swipe: ", err)
	}
}
