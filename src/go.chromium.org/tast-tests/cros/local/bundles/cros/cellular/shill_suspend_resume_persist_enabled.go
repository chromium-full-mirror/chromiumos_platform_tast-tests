// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/power/suspend"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type persistEnabledTestParams struct {
	enabledState bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillSuspendResumePersistEnabled,
		Desc:         "Verifies that cellular maintains enabled state around Suspend/Resume",
		Contacts:     []string{"chromeos-cellular-team@google.com", "danielwinkler@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active", "cellular_carrier_agnostic", "group:release-health", "release-health_cellular"},
		Fixture:      "cellularSuspendLocal",
		Timeout:      2 * time.Minute,
		// TODO(b/217106877): Skip on herobrine as S/R is unstable
		HardwareDeps: hwdep.D(hwdep.SkipOnPlatform("herobrine")),
		Params: []testing.Param{{
			Name: "enabled",
			Val: persistEnabledTestParams{
				enabledState: true,
			},
		}, {
			Name: "disabled",
			Val: persistEnabledTestParams{
				enabledState: false,
			},
		}},
	})
}

func ShillSuspendResumePersistEnabled(ctx context.Context, s *testing.State) {
	params := s.Param().(persistEnabledTestParams)

	helper := s.FixtValue().(*cellular.FixtData).Helper

	restoreWakeup := disableModemHostWakeup(ctx, s, helper.Device)
	defer restoreWakeup()

	// Apply required enabled state
	if params.enabledState {
		if _, err := helper.Enable(ctx); err != nil {
			s.Fatal("Unable to enable Cellular: ", err)
		}
	} else {
		if _, err := helper.Disable(ctx); err != nil {
			s.Fatal("Unable to disable Cellular: ", err)
		}
	}

	// Request suspend for 10 seconds.
	// Use wake up timeout to ensure device recovery.
	if _, err := suspend.Request(ctx, suspend.Delay(0), suspend.For(10*time.Second), suspend.WakeUpTimeout(15*time.Second)); err != nil {
		s.Fatal("Failed to perform system suspend (precondition): ", err)
	}

	// Verify enabled setting persisted
	if err := helper.Device.WaitForProperty(ctx, shillconst.DevicePropertyPowered, params.enabledState, shillconst.DefaultTimeout); err != nil {
		s.Fatal("Failed to set enabled to ", params.enabledState, ": ", err)
	}

	// Return to enabled state and confirm service available
	if _, err := helper.Enable(ctx); err != nil {
		s.Fatal("Failed to re-enable modem after resume: ", err)
	}

	if _, err := helper.FindServiceForDevice(ctx); err != nil {
		s.Fatal("Unable to find Cellular Service after resume: ", err)
	}
}

// disableModemHostWakeup temporarily disables system wakeup on sysfs ancestors
// (e.g. PCI xHCI controller) of the modem's network interface during RTC-timed
// suspend so spurious USB 3.0 link wakeups from modems such as Fibocom L850-GL
// do not cause hcd_pci_suspend to abort s2idle with -EBUSY (-16).
func disableModemHostWakeup(ctx context.Context, s *testing.State, dev *shill.Device) func() {
	props, err := dev.GetProperties(ctx)
	if err != nil {
		s.Log("Unable to read shill device properties for wakeup override: ", err)
		return func() {}
	}
	ifName, err := props.GetString(shillconst.DevicePropertyInterface)
	if err != nil || ifName == "" {
		return func() {}
	}
	devPath, err := filepath.EvalSymlinks(filepath.Join("/sys/class/net", ifName, "device"))
	if err != nil {
		return func() {}
	}

	var toRestore []string
	for dir := devPath; strings.HasPrefix(dir, "/sys/devices/"); dir = filepath.Dir(dir) {
		wakeupPath := filepath.Join(dir, "power", "wakeup")
		data, err := os.ReadFile(wakeupPath)
		if err != nil || strings.TrimSpace(string(data)) != "enabled" {
			continue
		}
		if err := os.WriteFile(wakeupPath, []byte("disabled"), 0644); err != nil {
			s.Logf("Failed to disable wakeup at %s: %v", wakeupPath, err)
			continue
		}
		s.Logf("Temporarily disabled wakeup at %s", wakeupPath)
		toRestore = append(toRestore, wakeupPath)
	}
	return func() {
		restoreFailed := false
		for _, p := range toRestore {
			if err := os.WriteFile(p, []byte("enabled"), 0644); err != nil {
				s.Errorf("Failed to restore wakeup at %s: %v", p, err)
				restoreFailed = true
				continue
			}
			s.Logf("Restored wakeup at %s", p)
		}
		if restoreFailed {
			if err := testexec.CommandContext(ctx, "reboot").Run(); err != nil {
				s.Error("Failed to reboot DUT after wakeup restore failure: ", err)
			}
		}
	}
}
