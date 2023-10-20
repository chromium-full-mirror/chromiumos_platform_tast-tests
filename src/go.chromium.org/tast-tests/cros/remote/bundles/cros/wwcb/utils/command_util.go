// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/exec"
	"go.chromium.org/tast/core/testing"
)

const (
	pollTimeout  = 30 * time.Second
	pollInterval = 200 * time.Millisecond
)

// VerifyPowerStatus verifies battery is charging or discharging.
func VerifyPowerStatus(ctx context.Context, dut *dut.DUT, isBatteryCharging bool) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		out, err := dut.Conn().CommandContext(ctx, "sudo", "cat", "/sys/class/power_supply/BAT0/status").Output()
		if err != nil {
			return errors.Wrap(err, "retrieve power supply info from DUT")
		}

		var chargingState bool
		if strings.TrimSpace(string(out)) != "Discharging" {
			chargingState = true
		} else {
			chargingState = false
		}
		if chargingState != isBatteryCharging {
			return errors.Errorf("unexpected power state, got: %t, want: %t", chargingState, isBatteryCharging)
		}
		return nil
	}, &testing.PollOptions{Timeout: pollTimeout, Interval: pollInterval})
}

// VerifyUSBAudioConnection verifies whether the USB audio are connected or not.
func VerifyUSBAudioConnection(ctx context.Context, dut *dut.DUT, isConnected bool) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		cmd := fmt.Sprint("cras_test_client | awk '$8==\"USB\" {print $5}'")
		out, err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Output()
		if err != nil {
			return errors.Wrap(err, "retrieve audio info with USB type from DUT")
		}

		status := false
		if strings.Contains(string(out), "yes") {
			status = true
		}

		if status != isConnected {
			return errors.Errorf("unexpected USB audio status, got: %t, want: %t", status, isConnected)
		}

		return nil
	}, &testing.PollOptions{Timeout: pollTimeout, Interval: pollInterval})
}

// VerifyDisplayCount verifies the number of dislpays is as expected.
func VerifyDisplayCount(ctx context.Context, dut *dut.DUT, want int) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		cmd := fmt.Sprintf("ls /sys/class/drm | grep card0-")
		out, err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Output()
		if err != nil {
			return errors.Wrap(err, "list display from DUT")
		}

		displayCount := 0
		for _, item := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			cmd := fmt.Sprintf("cat /sys/class/drm/%s/status", item)
			out, err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Output()
			if err != nil {
				return errors.Wrap(err, "retrieve display status from DUT")
			}

			if strings.TrimSpace(string(out)) == "connected" {
				displayCount++
			}
		}

		if displayCount != want {
			return errors.Errorf("unexpected number of displays, got: %d, want: %d", displayCount, want)
		}
		return nil
	}, &testing.PollOptions{Timeout: pollTimeout, Interval: pollInterval})
}

// GetUSBDevice retrieves USB devices info from lsusb command.
func GetUSBDevice(ctx context.Context, dut *dut.DUT) ([]string, error) {
	lsusbInfo, err := dut.Conn().CommandContext(ctx, "lsusb").Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "get lsusb info")
	}
	return strings.Split(strings.TrimSpace(string(lsusbInfo)), "\n"), err
}

// VerifyUSBDevicesCount verifies whether number of USB devices is as expected or not.
func VerifyUSBDevicesCount(ctx context.Context, dut *dut.DUT, expectUSBDevices []string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		devices, err := GetUSBDevice(ctx, dut)
		if err != nil {
			return errors.Wrap(err, "get USB devices")
		}
		if len(devices) != len(expectUSBDevices) {
			return errors.Errorf("unexpected number of USB devices, got: %v, want: %v", strings.Join(devices, "\n"), strings.Join(expectUSBDevices, "\n"))
		}
		return nil
	}, &testing.PollOptions{Timeout: pollTimeout, Interval: pollInterval})
}

// VerifyPeripheralsConnection verifies whether the peripherals are connected or not.
// It checks the following peripherals: power, external display, USB audio, Ethernet, USB devices.
func VerifyPeripheralsConnection(ctx context.Context, dut *dut.DUT, isConnected bool, expectUSBDevices []string) error {
	testing.ContextLog(ctx, "Starting verifying peripherals")

	testingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := VerifyPowerStatus(testingCtx, dut, isConnected); err != nil {
		return errors.Wrap(err, "verify connection of power")
	}

	var displayCount int
	if isConnected {
		displayCount = 2
	} else {
		displayCount = 1
	}
	if err := VerifyDisplayCount(testingCtx, dut, displayCount); err != nil {
		return errors.Wrap(err, "verify connection of external display")
	}

	if err := VerifyUSBAudioConnection(testingCtx, dut, isConnected); err != nil {
		return errors.Wrap(err, "verify connection of USB audio")
	}

	if err := VerifyEthernetState(testingCtx, dut, isConnected); err != nil {
		return errors.Wrap(err, "verify connection of Ethernet")
	}

	if err := VerifyUSBDevicesCount(testingCtx, dut, expectUSBDevices); err != nil {
		return errors.Wrap(err, "verify connection of USB devices")
	}
	return nil
}

// VerifyNetworkState verifies the state of the given network interface is as expected or not.
func VerifyNetworkState(ctx context.Context, dut *dut.DUT, network string, expectedState bool) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		out, err := dut.Conn().CommandContext(ctx, "sudo", "cat", "/sys/class/net/"+network+"/operstate").Output()
		if err != nil {
			return errors.Wrap(err, "retrieve ethernet operstate")
		}

		var currentState bool
		if strings.TrimSpace(string(out)) == "up" {
			currentState = true
		} else {
			currentState = false
		}

		if currentState != expectedState {
			return errors.Errorf("unexpected ethernet state, got: %t, want: %t", currentState, expectedState)
		}

		return nil
	}, &testing.PollOptions{Timeout: pollTimeout, Interval: pollInterval})
}

// VerifyEthernetState verifies whether the ethernet states are as expected or not.
// After the DUT restarts, the order of the ethernet interfaces may change,
// so it's inappropriate to check the assigning network.
// Change to list ethernet states to determine the state of docking station.
func VerifyEthernetState(ctx context.Context, dut *dut.DUT, isDockEthConnected bool) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		networks, err := ListNetworks(ctx, dut)
		if err != nil {
			return errors.Wrap(err, "list networks interface")
		}

		ethStates := make(map[string]string)
		for _, network := range networks {
			if strings.Contains(network, "eth") {
				out, err := dut.Conn().CommandContext(ctx, "sudo", "cat", "/sys/class/net/"+network+"/operstate").Output()
				if err != nil {
					return errors.Wrap(err, "retrieve ethernet operstate")
				}
				ethStates[network] = strings.TrimSpace(string(out))
			}
		}

		// One is from servo, the other is from docking station.
		if len(ethStates) != 2 {
			return errors.Errorf("unexpect number of Ethernet detected; got %d, want 2", len(ethStates))
		}

		allUP := true
		for _, state := range ethStates {
			if state != "up" {
				allUP = false
				break
			}
		}

		if allUP != isDockEthConnected {
			return errors.Errorf("unexpected ethernet states while the dock eth connection is %t; got %v,", isDockEthConnected, ethStates)
		}
		return nil
	}, &testing.PollOptions{Timeout: pollTimeout, Interval: pollInterval})
}

// ListNetworks returns the list of network interface.
func ListNetworks(ctx context.Context, dut *dut.DUT) ([]string, error) {
	out, err := dut.Conn().CommandContext(ctx, "ls", "/sys/class/net").Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "execute ls command")
	}
	return strings.Fields(string(out)), err
}

// ListEthernets returns ethernet interface name array.
func ListEthernets(ctx context.Context, dut *dut.DUT) ([]string, error) {
	cmd := fmt.Sprint(`ifconfig -s`)
	out, err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Output()
	if err != nil {
		return nil, errors.Wrap(err, "execute ifconfig command")
	}

	var ethernets []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		elements := strings.Split(line, " ")
		ethernets = append(ethernets, elements[0])
	}
	return ethernets, nil
}

// FindDockEthernet returns docking ethernet interface name.
func FindDockEthernet(ctx context.Context, dut *dut.DUT, defaultEth []string) (string, error) {
	var diff []string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		current, err := ListEthernets(ctx, dut)
		if err != nil {
			return err
		}

		diff = FindDifference(current, defaultEth)
		if len(diff) != 1 {
			return errors.Errorf("unexpected number of Ethernet detected; got %d, want 1", len(diff))
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 200 * time.Millisecond}); err != nil {
		return "", err
	}
	return diff[0], nil
}

// FindDifference finds the elements in one array but no in the other.
// i.e., Set difference of two arrays: A - B.
func FindDifference(a, b []string) []string {
	m := make(map[string]bool)

	for _, item := range b {
		m[item] = true
	}

	var diff []string
	for _, item := range a {
		if _, ok := m[item]; !ok {
			diff = append(diff, item)
		}
	}
	return diff
}

// FindInterface finds the certain interface name from ifconfig.
func FindInterface(ctx context.Context, dut *dut.DUT, ifName string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		cmd := fmt.Sprint(`ifconfig -s`)
		out, err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Output()
		if err != nil {
			return errors.Wrap(err, "find interfaces")
		}

		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			elements := strings.Split(line, " ")
			if len(elements) > 0 {
				if elements[0] == ifName {
					return nil
				}
			}
		}

		return errors.Errorf("Unable to find the %s interface", ifName)
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 200 * time.Microsecond})
}

// ListProcessInfo lists on its standard output file information about files opened by processes.
func ListProcessInfo(ctx context.Context, dut *dut.DUT, file string) ([]string, error) {
	out, err := dut.Conn().CommandContext(ctx, "lsof", file).Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "execute lsof command")
	}
	return strings.Fields(string(out)), nil
}

// BuiltinUsbCamerasFromV4L2Test returns a list of builtin usb camera paths.
func BuiltinUsbCamerasFromV4L2Test(ctx context.Context, dut *dut.DUT) ([]string, error) {
	cmd := dut.Conn().CommandContext(ctx, "media_v4l2_test", "--list_builtin_usbcam")
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "run media_v4l2_test")
	}
	return strings.Fields(string(out)), nil
}

// USBCamerasFromV4L2Test returns a list of usb camera paths.
func USBCamerasFromV4L2Test(ctx context.Context, dut *dut.DUT) ([]string, error) {
	cmd := dut.Conn().CommandContext(ctx, "media_v4l2_test", "--list_usbcam")
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "run media_v4l2_test")
	}
	return strings.Fields(string(out)), nil
}

// VideoDevices returns a list of video devices.
func VideoDevices(ctx context.Context, dut *dut.DUT) ([]string, error) {
	cmd := dut.Conn().CommandContext(ctx, "ls", "/dev/video*")
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "run ls command")
	}
	return strings.Fields(string(out)), nil
}

// DevicesFromV4L2 returns a list of all video for linux devices.
func DevicesFromV4L2(ctx context.Context, dut *dut.DUT) ([]string, error) {
	cmd := dut.Conn().CommandContext(ctx, "v4l2-ctl", "--list-devices")
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "run v4l2 command")
	}
	return strings.Fields(string(out)), nil
}

// Contains checks if an element of type string exists in a slice of strings.
func Contains(list []string, s string) bool {
	for _, line := range list {
		if line == s {
			return true
		}
	}
	return false
}

// FormatStorageToFAT formats the specified storage to FAT format.
func FormatStorageToFAT(ctx context.Context, mountPoint string, dut *dut.DUT, fs *dutfs.Client) error {
	// Get the device node.
	cmd := fmt.Sprintf("df | grep '%s' | awk '{print $1}'", mountPoint)
	deviceNode, err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrapf(err, "get device node of %s", mountPoint)
	}
	testing.ContextLogf(ctx, "mount point: %s", mountPoint)

	// Umount device.
	cmd = fmt.Sprintf("sudo umount %s", deviceNode)
	if err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "umount %s", string(deviceNode))
	}

	// Format device.
	cmd = fmt.Sprintf("sudo mkfs.vfat %s", deviceNode)
	if err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "format %s", string(deviceNode))
	}

	// Mount device.
	if dirExists, err := fs.Exists(ctx, mountPoint); err != nil {
		return errors.Wrap(err, "check mount point exists")
	} else if dirExists == false {
		if err := fs.MkDir(ctx, mountPoint, os.FileMode(0750)); err != nil {
			return errors.Wrap(err, "create mount point")
		}
	}

	cmd = fmt.Sprintf("sudo mount %s \"%s\"", strings.TrimSpace(string(deviceNode)), mountPoint)
	if err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "mount %s", string(deviceNode))
	}
	return nil
}

// CompareTwoFiles compares two files line by line.
func CompareTwoFiles(ctx context.Context, dut *dut.DUT, fileA, fileB string) error {
	diffCmd := fmt.Sprintf("diff '%s' '%s'", fileA, fileB)
	if err := dut.Conn().CommandContext(ctx, "sh", "-c", diffCmd).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "execute diff command")
	}
	return nil
}

// GetMountPoints gets the mount point information.
func GetMountPoints(ctx context.Context, dut *dut.DUT) ([]string, error) {
	lsblkOutput, err := dut.Conn().CommandContext(ctx, "sh", "-c", "lsblk -l -o mountpoint | grep removable").Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "received an incorrect result when using lsblk in the command")
	}
	return strings.Split(strings.TrimSpace(string(lsblkOutput)), "\n"), nil
}

// GetDeviceWritableStatus gets the read-write protection status of the device.
func GetDeviceWritableStatus(ctx context.Context, dut *dut.DUT, mountPoint string) (bool, error) {
	const writeProtectionFlag = "1\n"
	output, err := dut.Conn().CommandContext(ctx, "sh", "-c", fmt.Sprintf("df | grep '%s' | awk '{print $1}' | head -n 1", mountPoint)).Output(testexec.DumpLogOnError)
	if err != nil {
		return false, errors.Wrap(err, "can't get device node")
	}
	deviceNode := strings.TrimSpace(string(output))
	output, err = dut.Conn().CommandContext(ctx, "sh", "-c", fmt.Sprintf("sudo blockdev --getro %s", deviceNode)).Output(testexec.DumpLogOnError)
	if err != nil {
		return false, errors.Wrap(err, "can't get usb read-write mode")
	}
	if string(output) != writeProtectionFlag {
		return true, nil
	}
	return false, nil
}

// CopyFileToExternalStorage copies the file to external storage and ensures write protection is functioning properly.
func CopyFileToExternalStorage(ctx context.Context, dut *dut.DUT, mountPoint, remoteTextPath string) error {
	mountPointStatus, err := GetDeviceWritableStatus(ctx, dut, mountPoint)
	if err != nil {
		return errors.Wrap(err, "can't get the read-write status of the device")
	}
	usbTextPath := filepath.Join(mountPoint, "sample.txt")
	testing.ContextLogf(ctx, "Copy file to %s", usbTextPath)
	// Copy file to USB.
	copyCmd := fmt.Sprintf("cp '%s' '%s'", remoteTextPath, usbTextPath)
	err = dut.Conn().CommandContext(ctx, "sh", "-c", copyCmd).Run(testexec.DumpLogOnError)
	if mountPointStatus == true && err != nil {
		return errors.Wrap(err, "an error occurred while copying the file to the USB")
	} else if mountPointStatus == false && err == nil {
		return errors.Wrap(err, "should not successfully copy the file to the device")
	}
	return nil
}

// VerifyUSBDeviceConnectionChangeCount Verifies the count of USB devices after a connection change.
func VerifyUSBDeviceConnectionChangeCount(ctx context.Context, dut *dut.DUT, before, expectedCount int) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		after, err := GetUSBDevice(ctx, dut)
		if err != nil {
			return errors.Wrap(err, "get USB devices")
		}
		if (len(after) - before) != expectedCount {
			return errors.Wrapf(err, "unexpected change in the number of usb devices detected; expect: %d, actual: %d (from %d to %d)", expectedCount, (len(after) - before), before, len(after))
		}
		return nil
	}, &testing.PollOptions{Timeout: pollTimeout, Interval: pollInterval})
}

// VerifyExternalStorageMounted verifies the external storage is mounted by checking the output of mount command contains "/media/removable".
func VerifyExternalStorageMounted(ctx context.Context, dut *dut.DUT) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		out, err := dut.Conn().CommandContext(ctx, "mount").Output(exec.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to execute mount")
		}
		if !strings.Contains(string(out), "/media/removable") {
			return errors.New("failed to check the output of mount command contains /media/removable")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 1 * time.Second})
}

// IsDUTInTabletMode returns true if the DUT is in tablet mode, false otherwise.
func IsDUTInTabletMode(ctx context.Context, dut *dut.DUT) (bool, error) {
	// Verify powerd received the tablet mode change notification.
	testing.ContextLog(ctx, "Get the tabletmode value from powerd")
	// Expected output from the `dbus-send` command is:
	//    boolean true
	// OR
	//    boolean false
	out, err := dut.Conn().CommandContext(ctx, "dbus-send", "--system", "--print-reply=literal", "--type=method_call", "--dest=org.chromium.PowerManager", "/org/chromium/PowerManager", "org.chromium.PowerManager.GetTabletMode").Output()
	if err != nil {
		return false, errors.Wrap(err, "retrieve dbus-send output")
	}
	words := strings.Fields(string(out))
	if len(words) != 2 {
		return false, errors.Errorf("Received unexpected output from dbus-send:%s", string(out))
	}
	testing.ContextLog(ctx, "powerd tabletmode: ", words[1])
	return words[1] == "true", nil
}

// EnableDUTInTabletMode returns reset command.
func EnableDUTInTabletMode(ctx context.Context, dut *dut.DUT, pxy *servo.Proxy, isEnableTablet bool) (string, error) {
	tablet, err := IsDUTInTabletMode(ctx, dut)
	if err != nil {
		return "", errors.Wrap(err, "check DUT in tablet mode")
	}
	resetCommand := ""
	// Enable table mode.
	if isEnableTablet && !tablet {
		testing.ContextLog(ctx, "Enable tablet mode")
		if err := pxy.Servo().RunECCommand(ctx, "tabletmode on"); err != nil {
			return "", errors.Wrap(err, "enable tablet mode")
		}
		resetCommand = "tabletmode off"
	} else if !isEnableTablet && tablet {
		testing.ContextLog(ctx, "Disable tablet mode")
		if err := pxy.Servo().RunECCommand(ctx, "tabletmode off"); err != nil {
			return "", errors.Wrap(err, "disable tablet mode")
		}
		resetCommand = "tabletmode on"
	}
	return resetCommand, nil
}
