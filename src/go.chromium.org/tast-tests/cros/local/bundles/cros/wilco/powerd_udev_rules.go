// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wilco

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"syscall"

	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     PowerdUdevRules,
		Desc:     "Tests whether the rules under 93-powerd-wilco-ec-files.rules are correctly applied to power management related files exposed by Dell EC drivers",
		Contacts: []string{"chromeos-oem-services@google.com"},
		// ChromeOS > Software > Commercial (Enterprise) > OEM Services.
		BugComponent: "b:1256717",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		SoftwareDeps: []string{"vm_host", "wilco"},
	})
}

func PowerdUdevRules(ctx context.Context, s *testing.State) {
	userID, err := sysutil.GetUID("power")
	if err != nil {
		s.Fatal("Failed to get power user ID")
	}
	groupID, err := sysutil.GetGID("power")
	if err != nil {
		s.Fatal("Failed to get power group ID")
	}

	for _, tc := range []struct {
		name  string
		perm  fs.FileMode
		paths []string
	}{
		{
			"write_only",
			0220,
			[]string{
				// Boot on AC:
				"/sys/bus/platform/devices/GOOG000C:00/boot_on_ac",
			},
		},
		{
			"read_write",
			0664,
			[]string{
				// USB Charge:
				"/sys/bus/platform/devices/GOOG000C:00/usb_charge",
				// Battery Charge:
				"/sys/class/power_supply/wilco-charger/charge_type",
				"/sys/class/power_supply/wilco-charger/charge_control_start_threshold",
				"/sys/class/power_supply/wilco-charger/charge_control_end_threshold",
				// Advanced Charging:
				"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/advanced_charging/enable",
				"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/advanced_charging/monday",
				"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/advanced_charging/tuesday",
				"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/advanced_charging/wednesday",
				"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/advanced_charging/thursday",
				"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/advanced_charging/friday",
				"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/advanced_charging/saturday",
				"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/advanced_charging/sunday",
				// Peak Shift:
				"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/battery_threshold",
				"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/enable",
				"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/monday",
				"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/tuesday",
				"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/wednesday",
				"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/thursday",
				"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/friday",
				"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/saturday",
				"/sys/bus/platform/devices/GOOG000C:00/wilco-charge-schedule/peak_shift/sunday",
			},
		},
	} {
		for i, path := range tc.paths {
			s.Run(ctx, fmt.Sprintf("%s_%d", tc.name, i), func(ctx context.Context, s *testing.State) {
				info, err := os.Stat(path)
				if err != nil {
					s.Fatalf("Failed to get %v file stat: %v", path, err)
				}
				perm := info.Mode().Perm()
				if perm != tc.perm {
					s.Errorf("Unexpected perm of %v file: got %04o, want %04o", path, perm, tc.perm)
				}
				stat := info.Sys().(*syscall.Stat_t)
				if stat.Uid != userID {
					s.Errorf("Unexpected %v file user ID: got %d, want %d", path, stat.Uid, userID)
				}
				if stat.Gid != groupID {
					s.Errorf("Unexpected %v file group ID: got %d, want %d", path, stat.Gid, groupID)
				}
			})
		}
	}
}
