// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type usbDeviceTestParam struct {
	iter                int
	usbSpeed            string
	noOfConnectedDevice int
	usbDeviceClassName  string
	pendriveVersion     string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         USBDeviceFunctionality,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies USB device functionality before and after cold boot",
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		ServiceDeps:  []string{"tast.cros.ui.AudioService", "tast.cros.browser.ChromeService"},
		SoftwareDeps: []string{"chrome", "reboot"},
		VarDeps:      []string{"servo"},
		Vars:         []string{"intel.USB2", "intel.USB3"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Params: []testing.Param{{
			Name: "hid_coldboot",
			Val: usbDeviceTestParam{
				iter:                1,
				usbSpeed:            "1.5M",
				noOfConnectedDevice: 1, // Test H/W tolopoly requires One USB Type-A Human Interface Device like Keyboard/Mouse.
				usbDeviceClassName:  "Human Interface Device",
			},
			ExtraAttr: []string{"group:intel-usb-set2"},
			Timeout:   5 * time.Minute,
		}, {
			Name: "hid_coldboot_stress",
			Val: usbDeviceTestParam{
				iter:                10,
				usbSpeed:            "1.5M",
				noOfConnectedDevice: 2, // Test H/W tolopoly requires Two USB Type-A Human Interface Device like Keyboard/Mouse.
				usbDeviceClassName:  "Human Interface Device",
			},
			ExtraAttr: []string{"group:intel-usb-set2"},
			Timeout:   20 * time.Minute,
		}, {
			Name: "usb2_pendrive_coldboot",
			Val: usbDeviceTestParam{
				iter:                10,
				usbSpeed:            "480M",
				noOfConnectedDevice: 1, // Test H/W tolopoly requires One USB Type-A 2.0 pendrive.
				usbDeviceClassName:  "Mass Storage",
				pendriveVersion:     "2.0",
			},
			ExtraAttr: []string{"group:intel-usb-set1"},
			Timeout:   20 * time.Minute,
		}, {
			Name: "usb3_pendrive_coldboot",
			Val: usbDeviceTestParam{
				iter:                10,
				usbSpeed:            "5000M",
				noOfConnectedDevice: 1, // Test H/W tolopoly requires One USB Type-A 3.0 pendrive.
				usbDeviceClassName:  "Mass Storage",
				pendriveVersion:     "3.0",
			},
			ExtraAttr: []string{"group:intel-usb-set1"},
			Timeout:   20 * time.Minute,
		},
		}})
}

func USBDeviceFunctionality(ctx context.Context, s *testing.State) {
	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	dut := s.DUT()
	testParam := s.Param().(usbDeviceTestParam)

	// intel.USB2 or intel.USB3 variable is required for USB storage related tests.
	var usbStorageName string
	if testParam.pendriveVersion == "2.0" {
		usbStorageName = s.RequiredVar("intel.USB2")
	} else if testParam.pendriveVersion == "3.0" {
		usbStorageName = s.RequiredVar("intel.USB3")
	}

	servoSpec := s.RequiredVar("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(ctxForCleanUp)

	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Performing cleanup")
		if !dut.Connected(ctx) {
			if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
				s.Fatal("Failed to power-on DUT at cleanup: ", err)
			}
		}
	}(ctxForCleanUp)

	// Performs a Chrome login.
	loginChrome := func() (*rpc.Client, error) {
		testing.ContextLog(ctx, "Login to Chrome")
		cl, err := rpc.Dial(ctx, dut, s.RPCHint())
		if err != nil {
			return nil, errors.Wrap(err, "failed to connect to the RPC service on the DUT")
		}
		chromeService := ui.NewChromeServiceClient(cl.Conn)
		if _, err := chromeService.New(ctx, &ui.NewRequest{}); err != nil {
			s.Fatal("Failed to login Chrome: ", err)
		}
		return cl, nil
	}

	// Perform initial Chrome login.
	cl, err := loginChrome()
	if err != nil {
		s.Fatal("Failed to login Chrome: ", err)
	}

	iter := testParam.iter
	for i := 1; i <= iter; i++ {
		testing.ContextLogf(ctx, "Iteration: %d/%d", i, iter)
		// Check for USB device(s) detection before cold boot.
		usbDevicesList, err := usbutils.ListDevicesInfo(ctx, dut)
		if err != nil {
			s.Fatal("Failed to get USB devices list: ", err)
		}

		got := usbutils.NumberOfUSBDevicesConnected(usbDevicesList, testParam.usbDeviceClassName, testParam.usbSpeed)
		if want := testParam.noOfConnectedDevice; got < want {
			s.Fatalf("Unexpected number of USB devices connected: got %d, want %d", got, want)
		}

		// Check USB storage device is shown in filesapp before cold boot.
		if testParam.usbDeviceClassName == "Mass Storage" {
			audioService := ui.NewAudioServiceClient(cl.Conn)
			dirName := &ui.AudioServiceRequest{DirectoryName: usbStorageName, FileName: ""}
			if _, err := audioService.OpenDirectoryAndFile(ctx, dirName); err != nil {
				s.Fatalf("Failed to open %q directory: %v", usbStorageName, err)
			}
		}

		powerState := "S5"
		if err := powercontrol.ShutdownAndWaitForPowerState(ctx, pxy, dut, powerState); err != nil {
			s.Fatalf("Failed to shutdown and wait for %q powerstate: %v", powerState, err)
		}

		if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
			s.Fatal("Failed to power on DUT: ", err)
		}

		// Performing chrome login after powering on DUT from cold boot.
		cl, err = loginChrome()
		if err != nil {
			s.Fatal("Failed to login Chrome: ", err)
		}

		// Check for USB device(s) detection after cold boot.
		usbDevicesList, err = usbutils.ListDevicesInfo(ctx, dut)
		if err != nil {
			s.Fatal("Failed to get USB devices list: ", err)
		}

		got = usbutils.NumberOfUSBDevicesConnected(usbDevicesList, testParam.usbDeviceClassName, testParam.usbSpeed)
		if want := testParam.noOfConnectedDevice; got < want {
			s.Fatalf("Unexpected number of USB devices connected after cold boot: got %d, want %d", got, want)
		}

		// Check USB storage device is shown in filesapp after cold boot.
		if testParam.usbDeviceClassName == "Mass Storage" {
			audioService := ui.NewAudioServiceClient(cl.Conn)
			dirName := &ui.AudioServiceRequest{DirectoryName: usbStorageName, FileName: ""}
			if _, err := audioService.OpenDirectoryAndFile(ctx, dirName); err != nil {
				s.Fatalf("Failed to open %q directory after cold boot: %v", usbStorageName, err)
			}
		}

		valid, err := powercontrol.IsPrevSleepStateAvailable(ctx, dut)

		if err != nil {
			s.Fatal("Failed to determine if prev_sleep_state is available: ", err)
		}

		if valid {
			// Performing prev_sleep_state check.
			expectedPrevSleepState := 5
			if err := powercontrol.ValidatePrevSleepState(ctx, dut, expectedPrevSleepState); err != nil {
				s.Fatal("Failed to validate previous sleep state: ", err)
			}
		}
	}
}
