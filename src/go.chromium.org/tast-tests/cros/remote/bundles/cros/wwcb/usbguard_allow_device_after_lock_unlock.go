// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/services/cros/nearbyservice"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         USBGuardAllowDeviceAfterLockUnlock,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify USB device storage partitions are mounted while the DUT is at the lock screen and after unlocking",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit", "pasit_storage"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "USBID"},
		ServiceDeps:  []string{"tast.cros.nearbyservice.NearbyShareService"},
	})
}
func USBGuardAllowDeviceAfterLockUnlock(ctx context.Context, s *testing.State) {
	/*
		1. Connect the storage media to the Chromebook.
		2. Power the Chrombook On.
		3. Sign-in account.
		4. Check all partitions are mounted.
		5. Lock the Chromebook.
		6. Check all partitions are still mounted.
		7. Unplug & plug the USB storage device from Chromebook.
		8. Check all partitions are still mounted.
		9. Unlock the Chromebook.
		10. Check all partitions are mounted.
	*/

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	usbDeviceID := s.RequiredVar("USBID")

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	defer func(ctx context.Context) {
		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

	if err := utils.ShutdownDUT(ctx, pxy, dut); err != nil {
		s.Fatal("Failed to shutdown DUT: ", err)
	}
	defer func(ctx context.Context) {
		if !dut.Connected(ctx) {
			if err := utils.PowerOnDUT(ctx, pxy, dut); err != nil {
				s.Error("Failed to power on DUT: ", err)
			}
		}
	}(cleanupCtx)

	if err := utils.ControlFixture(ctx, usbDeviceID, "on"); err != nil {
		s.Fatal("Failed to control fixture to plug in the USB device: ", err)
	}

	if err := utils.PowerOnDUT(ctx, pxy, dut); err != nil {
		s.Fatal("Failed to power on DUT: ", err)
	}

	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	if err := utils.VerifyExternalStorageMounted(ctx, dut); err == nil {
		s.Fatal("Failed to verify nothing is mounted at the beginning of the test")
	}

	// Start Chrome on the DUT.
	ns := nearbyservice.NewNearbyShareServiceClient(cl.Conn)
	loginReq := &nearbyservice.CrOSLoginRequest{KeepState: true}
	if _, err := ns.NewChromeLogin(ctx, loginReq); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer ns.CloseChrome(ctx, &empty.Empty{})

	if err := utils.VerifyExternalStorageMounted(ctx, dut); err != nil {
		s.Fatal("Failed to verify the external storage is mounted after logging in the DUT: ", err)
	}

	s.Log("Lock the screen")
	if _, err := ns.LockScreen(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to lock screen: ", err)
	}
	if err := utils.VerifyExternalStorageMounted(ctx, dut); err != nil {
		s.Fatal("Failed to verify the external storage is mounted after locking the DUT: ", err)
	}

	s.Log("Unplug the USB device")
	beforeUnplug, err := utils.GetUSBDevice(ctx, dut)
	if err != nil {
		s.Fatal("Failed to lsusb before unplugging the USB device: ", err)
	}
	if err := utils.ControlFixture(ctx, usbDeviceID, "off"); err != nil {
		s.Fatal("Failed to control fixture to unplug the USB device: ", err)
	}
	afterUnplug, err := utils.GetUSBDevice(ctx, dut)
	if err != nil {
		s.Fatal("Failed to lsusb after unplugging the USB device: ", err)
	}
	if len(beforeUnplug) <= len(afterUnplug) {
		s.Fatalf("Expect number of USB devices before unplugging the device is more than after unplugging the device; before: %d, after: %d", len(beforeUnplug), len(afterUnplug))
	}

	s.Log("Plug in the USB device")
	if err := utils.ControlFixture(ctx, usbDeviceID, "on"); err != nil {
		s.Fatal("Failed to control fixture to re-plug in the USB device: ", err)
	}
	if err := utils.VerifyExternalStorageMounted(ctx, dut); err != nil {
		s.Fatal("Failed to verify the external storage is mounted after re-plugging in the DUT: ", err)
	}

	s.Log("Unlock the screen with the password")
	unlockReq := &nearbyservice.UnlockWithPasswordRequest{Username: "testuser@gmail.com", Password: "testpass"}
	if _, err := ns.UnlockWithPassword(ctx, unlockReq); err != nil {
		s.Fatal("Failed to unlock the screen: ", err)
	}
	if err := utils.VerifyExternalStorageMounted(ctx, dut); err != nil {
		s.Fatal("Failed to verify the external storage is mounted after unlocking the DUT: ", err)
	}
}
