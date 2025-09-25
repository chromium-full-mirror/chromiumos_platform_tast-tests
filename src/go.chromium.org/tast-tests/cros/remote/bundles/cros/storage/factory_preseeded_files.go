// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package storage

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/storage/util"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FactoryPreseededFiles,
		Desc: "Validation test for preseeded files",
		Contacts: []string{
			"chromeos-storage@google.com",
			"asavery@google.com",
		},
		Attr:         []string{"group:storage_destructive"},
		BugComponent: "b:974567", // ChromeOS > Platform > baseOS > Storage
	})
}

func FactoryPreseededFiles(ctx context.Context, s *testing.State) {
	dut := s.DUT()
	hasDefaultKey := util.HasDefaultKeyStatefulDiskLayout(ctx, dut)
	if !hasDefaultKey {
		s.Log("Skipping test: DUT does not have default-key-stateful disk layout")
		return
	}

	// Creating rma-data directory inside unencrypted.
	rmaDir := "/mnt/stateful_partition/unencrypted/rma-data"
	cmd := fmt.Sprintf("mkdir -p %v", rmaDir)
	_, err := util.RunCmdWithOutput(ctx, dut, "sh", "-c", cmd)
	if err != nil {
		s.Fatal("Unable to create  dir: ", err)
	}

	// Create a file named state inside the rma-data directory.
	var content string = "foobar"
	stateFile := rmaDir + "/state"
	cmd = fmt.Sprintf(`echo %s > %v`, content, stateFile)
	_, err = util.RunCmdWithOutput(ctx, dut, "sh", "-c", cmd)
	if err != nil {
		s.Fatal("Unable to write data to the file: ", err)
	}

	// Create or overwrite the factory reset marker file.
	cmd = fmt.Sprintf(`echo "fast safe keepimg rma" > /mnt/stateful_partition/factory_install_reset`)
	_, err = util.RunCmdWithOutput(ctx, dut, "sh", "-c", cmd)
	if err != nil {
		s.Fatal("Unable to write to factory_install_reset: ", err)
	}

	// Reboot the system.
	err = util.RebootDUT(ctx, dut)
	if err != nil {
		s.Fatal("Error while rebooting device: ", err)
	}

	// This function is to validate the file content.
	err = util.VerifyFileContent(ctx, dut, stateFile, content)
	if err != nil {
		s.Fatal("Error while verifying the file content: ", err)
	}
}
