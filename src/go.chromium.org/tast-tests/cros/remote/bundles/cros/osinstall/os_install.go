// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package osinstall

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/flex/fixture"
	"go.chromium.org/tast-tests/cros/services/cros/osinstall"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: OsInstall,
		Desc: "Test OS install (the DUT must be started back up again after install succeeds)",
		Contacts: []string{
			"chromeos-flex-eng+oncall@google.com",
			"nicholasbishop@google.com",
			"josephsussman@google.com",
		},
		BugComponent: "b:998633", // ChromeOS > Platform > Enablement > ChromeOS Flex
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"chrome", "flex_device", "no_qemu"},
		ServiceDeps:  []string{"tast.cros.osinstall.OsInstallService"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Fixture:      fixture.FlexWithServo,
		// Allow up to 20 minutes for install, plus some extra time for the DUT
		// to be started back up.
		Timeout: 25 * time.Minute,
	})
}

func runOsInstallAndRestart(ctx context.Context, s *testing.State) *osinstall.GetOsInfoResponse {
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	client := osinstall.NewOsInstallServiceClient(cl.Conn)

	preInstallInfo, err := client.GetOsInfo(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to get pre-install OS info: ", err)
	}
	s.Log("Pre-install OS info: ", preInstallInfo)

	// Check that the DUT is running from an installer.
	if !preInstallInfo.IsRunningFromInstaller {
		s.Fatal("The OS is not running from an installer")
	}

	// Launch Chrome OOBE.
	req := osinstall.StartChromeRequest{
		SigninProfileTestExtensionID: s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
	}
	s.Log("Starting Chrome")
	if _, err := client.StartChrome(ctx, &req); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	// Start the installation process.
	s.Log("Running OS install and waiting for it to complete")
	if _, err := client.RunOsInstall(ctx, &empty.Empty{}); err != nil {
		s.Fatal("OS install failed: ", err)
	}

	// Restart.
	if _, err := client.Restart(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to restart: ", err)
	}

	return preInstallInfo
}

func prepareUsbAndRestart(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.FixtData).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	cs := s.CloudStorage()
	if err := h.SetupUSBKey(ctx, cs); err != nil {
		s.Fatal("Failed to setup USB key: ", err)
	}

	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		s.Fatal("Failed to set dut_sees_usbkey: ", err)
	}

	// Use SSH to reboot the DUT because we don't know what state it is in.
	if err := h.DUT.Conn().CommandContext(ctx, "reboot").Run(); err != nil {
		s.Fatal("Failed to run reboot command: ", err)
	}

	// Wait for the DUT to shut down, then wait for it to come back up
	s.Log("Waiting for the DUT to shut down and then become reachable again")
	h.DUT.WaitUnreachable(ctx)
	h.DUT.WaitConnect(ctx)
}

func OsInstall(ctx context.Context, s *testing.State) {
	// Prepare the USB storage and restart, to get into a known state.
	prepareUsbAndRestart(ctx, s)

	// Get pre install info, run the OS install, then restart.
	preInstallInfo := runOsInstallAndRestart(ctx, s)

	// Once the installation is complete we need to remove the USB storage
	// as quickly as possible so the BIOS does not recognize it and attempt
	// to boot from it.
	h := s.FixtValue().(*fixture.FixtData).Helper
	// Remove the USB so the DUT does not boot to it.
	// GoBigSleepLint: Wait an arbitrary, small amount of time.
	testing.Sleep(ctx, 1*time.Second)
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxHost); err != nil {
		s.Fatal("Failed to set servo_sees_usbkey: ", err)
	}

	s.Log("Waiting for the DUT to shut down and then become reachable again")
	s.DUT().WaitUnreachable(ctx)
	s.DUT().WaitConnect(ctx)

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Get the system info to verify that the install succeeded.
	client := osinstall.NewOsInstallServiceClient(cl.Conn)
	info, err := client.GetOsInfo(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to get OS info: ", err)
	}

	if info.IsRunningFromInstaller {
		s.Fatal("Still running from an installer")
	}

	if preInstallInfo.Version != info.Version {
		s.Fatalf("Installed OS version does not match installer version: %s != %s", preInstallInfo.Version, info.Version)
	}
}
