// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UtilCheck,
		Desc:         "Check the health of power utils for reading device information",
		BugComponent: "b:1361410",
		Contacts: []string{
			"chromeos-platform-power@google.com",
			"yanyeli@google.com",
		},
		// This test collects hardware info that does not apply to virtual
		// machines (betty, tast-vm) and devices that don't have ChromeOS
		// firmware (reven).
		SoftwareDeps: []string{"crossystem", "chromeos_firmware"},
		Attr:         []string{"group:mainline"},
		Timeout:      1 * time.Minute,
	})
}

func UtilCheck(ctx context.Context, s *testing.State) {
	failedCheck := power.DeviceInfoUtilCheck(ctx)
	if len(failedCheck) != 0 {
		s.Fatal("Following power utils are broken: ", strings.Join(failedCheck, ", "))
	}
}
