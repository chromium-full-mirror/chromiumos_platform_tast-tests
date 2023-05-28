// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ScreenResolutionChange,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies all supported screen resolution on an external display can be set",
		Contacts: []string{
			"chromeos-velocity@google.com",
			"yixie@google.com", // Test author
		},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "vkms"},
		BugComponent: "b:633439",
		Fixture:      fixture.ChromeLoggedInWithForceVMDisplayExternal,
		Timeout:      3 * time.Minute,
	})
}

func ScreenResolutionChange(ctx context.Context, s *testing.State) {
	// Ensure display on.
	if err := power.TurnOnDisplay(ctx); err != nil {
		s.Fatal("Failed to turn on display: ", err)
	}

	cr := s.FixtValue().(*chrome.Chrome)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	dispInfo, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get primary display info: ", err)
	}

	if len(dispInfo.Modes) <= 1 {
		s.Fatalf("Count of available modes %d is not more than 1", len(dispInfo.Modes))
	}

	// Get current mode for restoring after test.
	selectedMode, err := dispInfo.GetSelectedMode()
	if err != nil {
		s.Fatal("Failed to get current display mode: ", err)
	}
	s.Log("Selected mode: ", selectedMode.Width, " x ", selectedMode.Height, " ", selectedMode)

	// Shorten context to allow for restoration.
	cleanUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	defer func() {
		if err := display.SetDisplayMode(cleanUpCtx, tconn, dispInfo.ID, selectedMode); err != nil {
			s.Log("Failed to restore display mode: ", err)
		}
	}()

	for _, mode := range dispInfo.Modes {
		s.Log("Set mode: ", mode.Width, " x ", mode.Height, " ", mode)
		if err := display.SetDisplayMode(ctx, tconn, dispInfo.ID, mode); err != nil {
			s.Fatal("Failed to set display mode: ", err)
		}
	}
}
