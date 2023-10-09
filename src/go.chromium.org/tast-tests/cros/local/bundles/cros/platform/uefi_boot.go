// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"os"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UEFIBoot,
		Desc: "Tests that the device is UEFI-booted",
		Contacts: []string{
			"chromeos-flex-eng@google.com",
			"josephsussman@google.com", // Test author
		},
		BugComponent: "b:998633", // ChromeOS > Platform > Enablement > ChromeOS Flex
		SoftwareDeps: []string{"uefi_firmware"},
		Attr:         []string{"group:mainline"},
	})
}

func UEFIBoot(ctx context.Context, s *testing.State) {
	efiFirmware := "/sys/firmware/efi"
	_, err := os.Stat(efiFirmware)
	if err != nil {
		s.Fatalf("Unable to locate EFI firmware at %s", efiFirmware)
	} else {
		s.Logf("Located EFI firmware at %s", efiFirmware)
	}
}
