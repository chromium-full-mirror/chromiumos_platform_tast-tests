// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usb

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: EthDriverPresence,
		Desc: "Test that Ethernet tethering drivers can be loaded",
		// ChromeOS > Platform > Connectivity > Ethernet
		BugComponent: "b:958036",
		Contacts: []string{
			"chromeos-usb-champs@google.com",
			"danielgeorgem@google.com",
		},
		Attr: []string{"group:mainline", "informational"},
		Params: []testing.Param{
			{
				Name: "rndis_host",
				Val:  "rndis_host",
			},
			{
				Name: "cdc_ether",
				Val:  "cdc_ether",
			},
			{
				Name: "cdc_ncm",
				Val:  "cdc_ncm",
			},
		},
	})
}

func EthDriverPresence(ctx context.Context, s *testing.State) {
	// This function will test that a selected ethernet driver can be loaded.
	// If the driver is already loaded, the test will be marked as passed.
	// The purpose of this test is to test the presence of tethering drivers
	// in the build, not the actual loading/unloading routine.

	// Save 10 seconds for cleanup
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	driver := s.Param().(string)

	if driverPresence(ctx, driver) == nil {
		s.Logf("Driver: %v was already loaded in, test passed", driver)
		return
	}

	// When modprobing a module we are expecting that the command will return
	// no errors and no output. Check for the opposite of these conditions and fail
	// if that's the case.
	s.Log("Trying to load driver: ", driver)
	out, err := testexec.CommandContext(ctx, "modprobe", driver).Output()
	if err != nil || len(out) > 0 {
		s.Fatalf("Failed to load driver: %v, modprobe, err was: %v", driver, err)
	}

	if err = driverPresence(ctx, driver); err != nil {
		s.Fatalf("Driver: %v modprobe was successful but driver's not present in sysfs, error was: %v", driver, err)
	}

	s.Logf("Driver: %v was loaded successfully", driver)

	defer func(ctx context.Context) {
		// Unload the driver. If we got to this point it means that
		// we have loaded a new driver. Removing it should not break anything
		// as it was unused when we started the test.
		s.Log("Trying to unload driver: ", driver)
		out, err = testexec.CommandContext(ctx, "modprobe", "-r", driver).Output()

		if err != nil || len(out) > 0 {
			s.Errorf("Failed to execute driver %v modprobe -r: %v", driver, err)
		} else {
			s.Log("Unloaded driver: ", driver)
		}
	}(cleanupCtx)
}

func driverPresence(ctx context.Context, driver string) error {
	// Check if a driver is loaded. Look in sysfs for the corresponding module.
	// If the driver is missing, we expect to find nothing with the driver's name
	// in /sys/module/.
	// The function returns an error if the driver is missing in sysfs, nil otherwise

	command := "ls -la /sys/module/ | grep -i " + driver
	out, err := testexec.CommandContext(ctx, "sh", "-c", command).Output()
	if err != nil {
		return err
	}

	if len(out) == 0 {
		return errors.Errorf("driver %v not present in sysfs", driver)
	}

	return nil
}
