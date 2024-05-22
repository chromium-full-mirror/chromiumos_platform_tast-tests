// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook.
package wwcb

import (
	"context"
	"os"
	"path"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils/topology"
	"go.chromium.org/tast-tests/cros/services/cros/nearbyservice"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/exec"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UsbguardBlocksUnknownDevicesAtLockScreen,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify no USB devices sotrage partitions mounted on the lock screnn,but they should be mounted on the unlock screen",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit", "pasit_storage"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.nearbyservice.NearbyShareService"},
		Vars:         []string{"servo"},
		Fixture:      "wwcbStorage",
		Params: []testing.Param{{
			Name: "normal",
			Val:  false,
		}, {
			Name: "suspend_resume",
			Val:  true,
		}},
	})
}

func UsbguardBlocksUnknownDevicesAtLockScreen(ctx context.Context, s *testing.State) {
	/*
		This test check no USB storage device partitions mounted on lock screen:
		1- Reboot reset device cache.
		2- Check there is no crash report file.
		3- Check /run/usb_bouncer have devices.proto file.
		4- Execute devices.proto file and record the content.
		5- Check /media/removable is empty.
		6- Connect the USB storage device to chromebook.
		7- Check /media/removable is empty.
		8- Check there have the target.new='block' in the messages file.
		9- Unlock the chromebook.
		10- Check USB storage device partitions are mounted.
	*/
	// Reboot the DUT, in case of DUT unable to work properly consistently.
	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatal("Failed to initialize reboot the DUT: ", err)
	}
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	isSuspend := s.Param().(bool)
	dut := s.DUT()
	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	defer func(ctx context.Context) {
		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

	// Clear old crash file.
	if err := dut.Conn().CommandContext(ctx, "sudo", "rm", "-r", "-f", "/var/spool/crash/").Run(exec.DumpLogOnError); err != nil {
		s.Fatal("Failed to clear folders under /var/spool/crash: ", err)
	}

	// Start Chrome on the DUT.
	ns := nearbyservice.NewNearbyShareServiceClient(cl.Conn)
	loginReq := &nearbyservice.CrOSLoginRequest{KeepState: false}
	if _, err := ns.NewChromeLogin(ctx, loginReq); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer ns.CloseChrome(ctx, &empty.Empty{})

	// Lock the chromebook.
	if _, err := ns.LockScreen(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to lock Chrome: ", err)
	}
	// Check no crash report file.
	if out, err := dut.Conn().CommandContext(ctx, "sudo", "ls", "/var/spool/crash").Output(exec.DumpLogOnError); err != nil {
		s.Fatal("Failed to listing the folders under /var/spool/crash: ", err)
	} else if string(out) != "" {
		s.Fatal("Crash folder not empty")
	}
	// Check devices.proto exsit.
	if out, err := dut.Conn().CommandContext(ctx, "sudo", "ls", "/run/usb_bouncer").Output(exec.DumpLogOnError); err != nil {
		s.Fatal("Failed to listing the folders under /run/usb_bouncer: ", err)
	} else if !strings.Contains(string(out), "devices.proto") {
		s.Fatal("Can't found devices.proto file")
	}
	// Execute devices.proto and record the content.
	out, err := dut.Conn().CommandContext(ctx, "sudo", "cat", "/run/usb_bouncer/devices.proto").Output(exec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to cat the file /run/usb_bouncer/devices.proto: ", err)
	} else if strings.TrimSpace(string(out)) == "" {
		s.Fatal("Can't record the devices.proto file execute content")
	}
	recordContent, err := os.Create(path.Join(s.OutDir(), "devicesContent.txt"))
	if err != nil {
		s.Fatal("Failed to create devicesContent.txt: ", err)
	}
	defer recordContent.Close()
	if _, err := recordContent.Write(out); err != nil {
		s.Fatal("Failed to write data to devicesContent.txt: ", err)
	}
	// Check no USB storage mounted.
	wentMountPoints := 0
	if err := checkMountPoints(ctx, dut, wentMountPoints); err != nil {
		s.Fatal("Failed to check mount points should not be USB storage mounted: ", err)
	}

	// Suspend and resume.
	if isSuspend {
		if err := suspendAndResume(ctx, dut, pxy); err != nil {
			s.Fatal("Failed to suspend/resume on DUT after check USB storage unmounted: ", err)
		}
	}
	tf := s.FixtValue().(*topology.TestFixture)
	if _, err := tf.Helper.ActivateDeviceByType(ctx, topology.DeviceTypeStorage); err != nil {
		s.Fatal("Failed to open usb type c fixture: ", err)
	}

	// Check no USB storage mounted.
	if err := checkMountPoints(ctx, dut, wentMountPoints); err != nil {
		s.Fatal("Failed to check mount points should not be USB storage mounted after open fixture: ", err)
	}

	// Check the messages have target.new=block.
	out, err = dut.Conn().CommandContext(ctx, "cat", "/var/log/messages").Output(exec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to cat /var/log/messages: ", err)
	}
	recordMessages, err := os.Create(path.Join(s.OutDir(), "messages.txt"))
	if err != nil {
		s.Fatal("Failed to create messages.txt: ", err)
	}
	defer recordMessages.Close()
	if _, err := recordMessages.Write(out); err != nil {
		s.Fatal("Failed to write data to messages.txt: ", err)
	}
	if content, err := os.ReadFile(path.Join(s.OutDir(), "messages.txt")); err != nil {
		s.Fatal("Failed to read messages.txt: ", err)
	} else if !strings.Contains(string(content), "target.new='block'") {
		s.Fatal("The messages should have target.new='block'")
	}

	// Suspend and resume.
	if isSuspend {
		if err := suspendAndResume(ctx, dut, pxy); err != nil {
			s.Fatal("Failed to suspend/resume on DUT after check messages: ", err)
		}
		// Connect to the gRPC server on the DUT.
		cl, err = rpc.Dial(ctx, s.DUT(), s.RPCHint())
		if err != nil {
			s.Fatal("Failed to initialize connect to the RPC service on the DUT: ", err)
		}
		ns = nearbyservice.NewNearbyShareServiceClient(cl.Conn)
		if _, err := ns.NewChromeLogin(ctx, loginReq); err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
	} else {
		// Unlock the chromebook.
		s.Log("Unlocking the screen with password")
		unlockReq := &nearbyservice.UnlockWithPasswordRequest{Username: "testuser@gmail.com", Password: "testpass"}
		if _, err := ns.UnlockWithPassword(ctx, unlockReq); err != nil {
			s.Fatal("Failed to unlock the screen: ", err)
		}
	}
	wentMountPoints = 1
	// Check USB storage mounted.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := checkMountPoints(ctx, dut, wentMountPoints); err != nil {
			return errors.Wrap(err, "check mount points")
		}
		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to check USB storage mounted: ", err)
	}
}

func checkMountPoints(ctx context.Context, dut *dut.DUT, count int) error {
	if mountPoints, err := utils.GetMountPoints(ctx, dut); err != nil && count > 0 {
		return errors.Wrap(err, "get mount points")
	} else if count > 0 && len(mountPoints) < count {
		return errors.Errorf("mount points got:%d went:%d", len(mountPoints), count)
	}
	return nil
}
func suspendAndResume(ctx context.Context, dut *dut.DUT, pxy *servo.Proxy) error {
	if err := utils.SuspendDUT(ctx, dut, pxy); err != nil {
		return errors.Wrap(err, "suspend DUT")
	}
	if err := utils.PowerOnDUT(ctx, pxy, dut); err != nil {
		return errors.Wrap(err, "power on DUT")
	}
	return nil
}
