// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package touchpad

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type touchpadTestParams struct {
	tabletMode      bool
	detectionStatus string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         FunctionalityCheck,
		Desc:         "Verify Touchpad primary button clicks",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "pathan.jilani@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Touchpad(), hwdep.FormFactor(hwdep.Convertible)),
		Fixture:      "chromeLoggedIn",
		Params: []testing.Param{{
			Name: "clamshell_mode",
			Val: touchpadTestParams{
				tabletMode:      false,
				detectionStatus: "enabled",
			},
			ExtraAttr: []string{"group:intel-nda"},
		}, {
			Name: "tablet_mode",
			Val: touchpadTestParams{
				tabletMode:      true,
				detectionStatus: "disabled",
			},
			ExtraAttr: []string{"group:intel-convertible"},
		}},
	})
}

func FunctionalityCheck(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	testOpt := s.Param().(touchpadTestParams)

	// Set tabletModeAngle to 0 to force the DUT into tablet mode.
	if testOpt.tabletMode {
		testing.ContextLog(ctx, "Put DUT into tablet mode")
		cleanUp, err := ash.EnsureTabletModeEnabledWithKeyboardDisabled(ctx)
		if err != nil {
			s.Fatal("Failed to put DUT in tablet mode: ", err)
		}
		defer cleanUp(cleanupCtx)
	}

	// Verifies touchpad detection with expected detectionStatus.
	wakeUpCommand := "cat /sys/devices/pci0000:00/0000:00:15.0/i2c_designware.0/*/*/power/wakeup"
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		sourceOut, err := testexec.CommandContext(ctx, "sh", "-c", wakeUpCommand).Output()
		if err != nil {
			return testing.PollBreak(errors.Wrapf(err, "failed to read %q file", wakeUpCommand))
		}
		actualStatus := strings.TrimSpace(string(sourceOut))
		if !strings.Contains(actualStatus, testOpt.detectionStatus) {
			return errors.Errorf("unexpected detection status, want %q; got %q", testOpt.detectionStatus, actualStatus)
		}
		return nil
	}, &testing.PollOptions{
		Timeout: 10 * time.Second,
	}); err != nil {
		s.Error("Failed touchpad detection: ", err)
	}
}
