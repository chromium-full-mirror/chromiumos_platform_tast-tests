// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"os"

	"go.chromium.org/tast/core/testing"
)

const mtd0Path = "/sys/class/mtd/mtd0"

func init() {
	testing.AddTest(&testing.Test{
		Func: CheckMTD,
		Desc: "Verifies that mtd0 exists on ARM devices",
		Contacts: []string{
			"cros-flashrom-team@google.com", // CrOS Flashrom Maintainer
			"chromeos-firmware@google.com",  // CrOS Firmware Developers
			"eizan@chromium.org",            // Test Author
		},
		BugComponent: "b:750299", // ChromeOS > Platform > Enablement > Firmware > Flashrom
		Attr:         []string{"group:flashrom"},
		SoftwareDeps: []string{"flashrom", "arm"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func CheckMTD(ctx context.Context, s *testing.State) {
	_, err := os.Stat(mtd0Path)
	if os.IsNotExist(err) {
		s.Fatal("Path doesn't exist: ", mtd0Path)
	}
	if err != nil {
		s.Fatal("Unknown error occurred: ", err)
	}
}
