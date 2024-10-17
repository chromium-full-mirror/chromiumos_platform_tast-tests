// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TabletModeCheck,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies tablet mode functionality with checking input devices behavior",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"inputs_deps", "chrome"},
		Attr:         []string{"group:intel-convertible"},
		Fixture:      "chromeLoggedIn",
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Touchpad(), hwdep.Keyboard(), hwdep.FormFactor(hwdep.Convertible)),
	})
}

func TabletModeCheck(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	keyboardAvailable, _, err := input.FindPhysicalKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create keyboard event: ", err)
	}

	if !keyboardAvailable {
		s.Fatal("Failed to find keyboard input device")
	}

	touchpadAvailable, _, err := input.FindPhysicalTrackpad(ctx)
	if err != nil {
		s.Fatal("Failed to create touchpad event: ", err)
	}

	if !touchpadAvailable {
		s.Fatal("Failed to find touchpad input device")
	}

	testing.ContextLog(ctx, "Put DUT into tablet mode")
	cleanUp, err := ash.EnsureTabletModeEnabledWithKeyboardDisabled(ctx)
	if err != nil {
		s.Fatal("Failed to put DUT in tablet mode: ", err)
	}
	defer cleanUp(cleanupCtx)
}
