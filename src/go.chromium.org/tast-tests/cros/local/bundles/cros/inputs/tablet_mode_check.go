// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
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

	keyboardAvailable, keyboardDevPath, err := input.FindPhysicalKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create keyboard event: ", err)
	}

	if !keyboardAvailable {
		s.Fatal("Failed to find keyboard input device")
	}

	touchpadAvailable, touchpadDevPath, err := input.FindPhysicalTrackpad(ctx)
	if err != nil {
		s.Fatal("Failed to create touchpad event: ", err)
	}

	if !touchpadAvailable {
		s.Fatal("Failed to find touchpad input device")
	}

	const (
		devRoot              = "/dev"
		enabledStatusString  = "enabled"
		disabledStatusString = "disabled"
	)

	onboardKeyboardEventPath, err := filepath.Rel(devRoot, keyboardDevPath)
	if err != nil {
		s.Fatal("Failed to get keyboard event relative path: ", err)
	}

	if err := inputDeviceDetectionCheck(ctx, onboardKeyboardEventPath, enabledStatusString); err != nil {
		s.Fatal("Failed to verify keyboard event in evtest in clamshell mode: ", err)
	}

	touchpadEventPath, err := filepath.Rel(devRoot, touchpadDevPath)
	if err != nil {
		s.Fatal("Failed to get touchpad event relative path: ", err)
	}

	if err := inputDeviceDetectionCheck(ctx, touchpadEventPath, enabledStatusString); err != nil {
		s.Fatal("Failed to verify touchpad event in evtest in clamshell mode: ", err)
	}

	testing.ContextLog(ctx, "Put DUT into tablet mode")
	cleanUp, err := ash.EnsureTabletModeEnabledWithKeyboardDisabled(ctx)
	if err != nil {
		s.Fatal("Failed to put DUT in tablet mode: ", err)
	}
	defer cleanUp(cleanupCtx)

	if err := inputDeviceDetectionCheck(ctx, onboardKeyboardEventPath, disabledStatusString); err != nil {
		s.Fatal("Failed to verify keyboard event in evtest in tablet mode: ", err)
	}

	if err := inputDeviceDetectionCheck(ctx, touchpadEventPath, disabledStatusString); err != nil {
		s.Fatal("Failed to verify touchpad event in evtest in tablet mode: ", err)
	}
}

// inputDeviceDetectionCheck verifies input device eventPath has expectedDetectionStatus.
func inputDeviceDetectionCheck(ctx context.Context, eventPath, expectedDetectionStatus string) error {
	wakeSourceFile := fmt.Sprintf("/sys/class/%s/device/device/power/wakeup", eventPath)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		sourceOut, err := ioutil.ReadFile(wakeSourceFile)
		if err != nil {
			return errors.Wrapf(err, "failed to read %q file", wakeSourceFile)
		}
		got := strings.TrimSpace(string(sourceOut))
		if !strings.Contains(got, expectedDetectionStatus) {
			return errors.Errorf("unexpected detection status: got %q; want %q", got, expectedDetectionStatus)
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
		return err
	}
	return nil
}
