// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast-tests/cros/remote/fileutils"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: StartupPreseededFiles,
		Desc: "Validation tests for preservation of preseeded files",
		Contacts: []string{
			"chromeos-storage@google.com",
			"asavery@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		BugComponent: "b:974567", // ChromeOS > Platform > baseOS > Storage
	})
}

func StartupPreseededFiles(ctx context.Context, s *testing.State) {

	dut := s.DUT()

	// This is to setting up boot from usb
	err := util.BootFromUSB(ctx, dut)
	if err != nil {
		s.Fatal("Unable to set boot from usb: ", err)
	}

	// This function installs chromeOS using usb drive.
	err = util.InstallChromeOS(ctx, dut)
	if err != nil {
		s.Fatal("Error while installing the chromeOS: ", err)
	}

	// Checking if dev_image.block is exists at path /mnt/stateful_partition/unencrypted/.
	exists, err := fileutils.HostFileExists(ctx, dut.Conn(), "/mnt/stateful_partition/unencrypted/dev_image.block")
	if err != nil {
		s.Fatal("File dev_image.block could not be checked: ", err)
	} else if !exists {
		s.Fatal("File dev_image.block does not exist")
	}
}
