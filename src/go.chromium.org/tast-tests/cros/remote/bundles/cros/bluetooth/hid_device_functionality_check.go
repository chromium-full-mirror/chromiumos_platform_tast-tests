// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/action"
	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           HIDDeviceFunctionalityCheck,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Verify that the user is able to use Bluetooth HID keyboard/mouse for input",
		Contacts: []string{
			"edgar.change@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		Attr:         []string{"group:bluetooth"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(1)},
		ServiceDeps:  []string{"tast.cros.bluetooth.BluetoothService", "tast.cros.bluetooth.BluetoothUIService"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name:      "floss_disabled__keyboard",
				Fixture:   "chromeLoggedInWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val:       cbt.DeviceTypeKeyboard,
			}, {
				Name:      "floss_disabled__mouse",
				Fixture:   "chromeLoggedInWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val:       cbt.DeviceTypeMouse,
			}, {
				Name:      "floss_disabled__le_keyboard",
				Fixture:   "chromeLoggedInWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val:       cbt.DeviceTypeLEKeyboard,
			}, {
				Name:      "floss_disabled__le_mouse",
				Fixture:   "chromeLoggedInWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val:       cbt.DeviceTypeLEMouse,
			}, {
				Name:              "floss_enabled__keyboard",
				Fixture:           "chromeLoggedInWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val:               cbt.DeviceTypeKeyboard,
			}, {
				Name:              "floss_enabled__mouse",
				Fixture:           "chromeLoggedInWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val:               cbt.DeviceTypeMouse,
			}, {
				Name:              "floss_enabled__le_keyboard",
				Fixture:           "chromeLoggedInWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val:               cbt.DeviceTypeLEKeyboard,
			}, {
				Name:              "floss_enabled__le_mouse",
				Fixture:           "chromeLoggedInWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val:               cbt.DeviceTypeLEMouse,
			},
		},
	})
}

// HIDDeviceFunctionalityCheck verifies that the user is able to use Bluetooth HID for input.
func HIDDeviceFunctionalityCheck(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)
	deviceType := s.Param().(cbt.DeviceType)

	device, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0], &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: deviceType,
	})
	if err != nil {
		s.Fatal("Failed to create an Emulated BTPeer Device: ", err)
	}

	if _, err := fv.BluetoothUIService.PairDeviceWithQuickSettings(ctx, &bts.PairDeviceWithQuickSettingsRequest{
		AdvertisedName: device.AdvertisedName(),
	}); err != nil {
		s.Fatalf("Failed to pair the Bluetooth device %q with quick settings: %v", device.String(), err)
	}

	resp, err := fv.BluetoothService.StackType(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to fetch bluetooth stack set from fixture: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	rpcClient, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to create a new rpc client: ", err)
	}
	defer rpcClient.Close(cleanupCtx)

	// Terminate the goroutine before the test ends.
	monitorCtx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// TODO(b/334806604): Remove this block once the cause of the issue is identified.
	// It's observed that BT peer does not listed as an input device after it's paired and connected to ChromeOS,
	// this block is for monitoring if the BT peer ever gets disconnected during the entire test, to further identify the cause.
	go func(ctx context.Context, rpcClient *rpc.Client) {
		// Avoid using the service client initialized by the fixture, as it is shared with other tests and may cause unexpected leaks.
		bluetoothService := bts.NewBluetoothServiceClient(rpcClient.Conn)
		if _, err := bluetoothService.SetBluetoothStack(ctx, &bts.SetBluetoothStackRequest{
			StackType: resp.GetStackType(),
		}); err != nil {
			s.Log("Failed to initialize the new bluetooth service: ", err)
			return
		}

		deadline, ok := ctx.Deadline()
		if !ok {
			s.Log("Failed to obtain test context deadline")
			return
		}
		if _, err := bluetoothService.WaitForConnectState(ctx, &bts.WaitForConnectStateRequest{
			DeviceAddress:        device.LocalBluetoothAddress(),
			ExpectedConnectState: false,
			Timeout:              durationpb.New(deadline.Sub(time.Now())),
		}); err != nil {
			if errors.As(err, &context.Canceled) {
				return
			}
			s.Log("Failed to monitor Bluetooth device connection status: ", err)
			return
		}
		s.Log("Bluetooth device disconnected unexpectedly during the test")
	}(monitorCtx, rpcClient)

	// This is the map of [input event ID] to [send HID report action],
	// indicates that by sending a HID report should be able to observe an expected input event.
	var inputEventIDToSendReportActionMap map[string]action.Action

	switch deviceType {
	case cbt.DeviceTypeMouse, cbt.DeviceTypeLEMouse:
		const movement = 100
		mouse := device.RPCMouse()
		inputEventIDToSendReportActionMap = map[string]action.Action{
			"BTN_LEFT":  mouse.LeftClick,
			"BTN_RIGHT": mouse.RightClick,
			"REL_Y":     func(ctx context.Context) error { return mouse.Move(ctx, 0 /* deltaX */, movement /* deltaY */) },
			"REL_X":     func(ctx context.Context) error { return mouse.Move(ctx, movement /* deltaX */, 0 /* deltaY */) },
			"REL_WHEEL": func(ctx context.Context) error { return mouse.Scroll(ctx, movement /* steps */) },
		}
	case cbt.DeviceTypeKeyboard, cbt.DeviceTypeLEKeyboard:
		sendKeyEvent := func(key byte) action.Action {
			// TODO(b/289871801): Using KeyboardSendString Chameleond RPC method once the issue is resolved.
			return func(ctx context.Context) error { return device.RPCKeyboard().KeyboardSendKeyEvent(ctx, key) }
		}

		inputEventIDToSendReportActionMap = map[string]action.Action{
			"KEY_C": sendKeyEvent('c'),
			"KEY_H": sendKeyEvent('h'),
			"KEY_R": sendKeyEvent('r'),
			"KEY_O": sendKeyEvent('o'),
			"KEY_M": sendKeyEvent('m'),
			"KEY_E": sendKeyEvent('e'),
		}
	}

	for inputEventID, sendReportAction := range inputEventIDToSendReportActionMap {
		if err := sendHIDReportAndVerify(ctx, s.DUT(), sendReportAction, device.AdvertisedName(), inputEventID, s.OutDir()); err != nil {
			s.Fatalf("Failed to verify Bluetooth HID %q %q functionality: %v", device.AdvertisedName(), inputEventID, err)
		}
	}
}

// sendHIDReportAndVerify sends the HID report by triggering the Bluetooth HID event and
// verifies its functionality on the chromeOS by the evtest command-line tool.
func sendHIDReportAndVerify(ctx context.Context, dut *dut.DUT, sendReport action.Action, deviceName, expectedEvent, outDir string) error {
	devicePath, err := fetchDevicePath(ctx, dut, deviceName, outDir)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch the Bluetooth device %q device event path", deviceName)
	}

	cmd := dut.Conn().CommandContext(ctx, "evtest", devicePath)

	var outBuffer bytes.Buffer
	cmd.Stdout = &outBuffer

	if err = cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to start evtest command")
	}
	defer cmd.Wait()
	defer cmd.Abort()

	// Wait for the command to be started, or the report could be sent before it's started.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Any captured outputs indicates the command has started.
		if outBuffer.Len() != 0 {
			outBuffer.Reset()
			return nil
		}
		return errors.New("evtest not started")
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: time.Second}); err != nil {
		return err
	}

	if err := sendReport(ctx); err != nil {
		return errors.Wrap(err, "failed to invoke a Bluetooth HID event")
	}

	// Regular expression for finding the expected input event.
	//
	// This is an example of the log of a triggered event:
	// 	Event: time 1686880566.986645, type 1 (EV_KEY), code 272 (BTN_LEFT), value 1
	reg := regexp.MustCompile(fmt.Sprintf(`Event: time \d+\.\d+, type \d+ \(.*\), code \d+ \(%s\), value \d+`, expectedEvent))

	// The BlueZ service on the chameleon device could take up to a minute to be ready
	// before sending out a Bluetooth/BLE HID report.
	// (refer to Python class "chameleond.utils.raspi_bluez_service.BluezService")
	waitForReportTimeout := time.Minute

	// Retry a few times, just in case of wireless communication latency or performance issue.
	return testing.Poll(ctx, func(ctx context.Context) error {
		if reg.MatchString(outBuffer.String()) {
			return nil
		}
		return errors.New("failed to find the expected HID event log in the output of the evtest command-line tool")
	}, &testing.PollOptions{Timeout: waitForReportTimeout, Interval: time.Second})
}

// fetchDevicePath returns device path that can be used by evtest command-line tool.
func fetchDevicePath(ctx context.Context, dut *dut.DUT, deviceName, outDir string) (_ string, retErr error) {
	// Variables for finding the device ID by match information of registered input devices.
	//
	// This is an example of the information of a input device:
	// 	N: Name="KEYBD_REF Keyboard"<line-break>
	// 	P: Phys=3c:9c:0f:2d:7d:86<line-break>
	// 	S: Sysfs=/devices/virtual/misc/uhid/0005:1D6B:0246.001F/input/input54<line-break>
	// 	U: Uniq=e4:5f:01:ee:4d:ef<line-break>
	// 	H: Handlers=sysrq leds event19 <line-break>
	// 	B: PROP=0 <line-break>
	// 	B: EV=13 <line-break>
	// 	B: KEY=80002000000 387ad8011001 e000000000000 0 <line-break>
	// 	B: MSC=10 <line-break>
	var (
		lineBreak = `(\r\n|\r|\n)`

		name     = fmt.Sprintf(`N: Name="%s.*"`, deviceName) + lineBreak
		phys     = `P: Phys=.*` + lineBreak
		sysfs    = `S: Sysfs=.*` + lineBreak
		uniq     = `U: Uniq=.*` + lineBreak
		handlers = `H: Handlers=.*event(\d+).*` + lineBreak

		reg = regexp.MustCompile(name + phys + sysfs + uniq + handlers)
	)

	var deviceID string

	var inputDevices []byte

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	defer func(ctx context.Context) {
		if retErr != nil {
			path := filepath.Join(outDir, fmt.Sprintf("input_devices_%v", time.Now().Unix()))
			if err := os.WriteFile(path, inputDevices, 0644); err != nil {
				testing.ContextLog(ctx, "Failed to dump the device file content: ", err)
			}
		}
	}(cleanupCtx)

	// A Bluetooth device could take a while to be completely registered as an input device, especially for the low end devices.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		data, err := linuxssh.ReadFile(ctx, dut.Conn(), "/proc/bus/input/devices")
		if err != nil {
			return errors.Wrap(err, "failed to acquire the full info of all input devices")
		}
		inputDevices = data

		ss := reg.FindStringSubmatch(string(data))
		// Expecting 7 sub-matches which are the entire match, 5 line-breaks and the device-ID.
		if ss == nil || len(ss) != 7 {
			return errors.New("failed to find the input device id")
		}
		deviceID = ss[5]

		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
		return "", errors.Wrap(err, "failed to parse the file describing input devices")
	}

	return fmt.Sprintf("/dev/input/event%s", deviceID), nil
}
