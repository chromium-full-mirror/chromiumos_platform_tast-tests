// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	pollTimeout  = 30 * time.Second
	pollInterval = 200 * time.Millisecond
)

// VerifyPowerStatus verifies battery is charging or discharging.
func VerifyPowerStatus(ctx context.Context, dut *dut.DUT, isBatteryCharging bool) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		regex := `state:(\s+\w+\s?\w+)`
		expMatch := regexp.MustCompile(regex)

		out, err := dut.Conn().CommandContext(ctx, "sudo", "power_supply_info").Output()
		if err != nil {
			return errors.Wrap(err, "failed to retrieve power supply info from DUT")
		}

		matches := expMatch.FindStringSubmatch(string(out))
		if len(matches) < 2 {
			return errors.Errorf("failed to match regex: %q in: %q", expMatch, string(out))
		}

		var chargingState bool
		if strings.TrimSpace(matches[1]) != "Discharging" {
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
			return errors.Wrap(err, "failed to retrieve audio info with USB type from DUT")
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
			return errors.Wrap(err, "failed to list display from DUT")
		}

		displayCount := 0
		for _, item := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			cmd := fmt.Sprintf("cat /sys/class/drm/%s/status", item)
			out, err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Output()
			if err != nil {
				return errors.Wrap(err, "failed to retrieve display status from DUT")
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
		return nil, errors.Wrap(err, "failed to get lsusb info")
	}
	return strings.Split(strings.TrimSpace(string(lsusbInfo)), "\n"), err
}

// VerifyTypeADevicesCount verifies number of USB devices is as expected.
func VerifyTypeADevicesCount(ctx context.Context, dut *dut.DUT, expectUSBTypeADeviceNum int) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		current, err := GetUSBDevice(ctx, dut)
		if err != nil {
			return err
		}
		if len(current) != expectUSBTypeADeviceNum {
			return errors.Errorf("unexpected number of USB devices, got: %d, want: %d", len(current), expectUSBTypeADeviceNum)
		}
		return nil
	}, &testing.PollOptions{Timeout: pollTimeout, Interval: pollInterval})
}

// VerifyPeripheralsConnection verifies whether the peripherals are connected or not.
// It checks the following peripherals: power, external display, USB audio, Ethernet, USB Type-A devices.
func VerifyPeripheralsConnection(ctx context.Context, dut *dut.DUT, isConnected bool, dockingEth string, expectUSBTypeADeviceNum int) error {
	testing.ContextLog(ctx, "Starting verifying peripherals")

	testingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := VerifyPowerStatus(testingCtx, dut, isConnected); err != nil {
		return errors.Wrap(err, "failed to verify connection of power")
	}

	var displayCount int
	if isConnected {
		displayCount = 2
	} else {
		displayCount = 1
	}
	if err := VerifyDisplayCount(testingCtx, dut, displayCount); err != nil {
		return errors.Wrap(err, "failed to verify connection of external display")
	}

	if err := VerifyUSBAudioConnection(testingCtx, dut, isConnected); err != nil {
		return errors.Wrap(err, "failed to verify connection of USB audio")
	}

	if isConnected {
		if err := FindInterface(ctx, dut, dockingEth); err != nil {
			return errors.Wrap(err, "failed to find docking station Ethernet")
		}
	} else {
		if err := FindInterface(ctx, dut, dockingEth); err == nil {
			return errors.New("Expect the Ethernet interface in the Dock is not connected; however it is still found")
		}
	}

	if err := VerifyTypeADevicesCount(testingCtx, dut, expectUSBTypeADeviceNum); err != nil {
		return errors.Wrap(err, "failed to verify connection of USB Type-A devices")
	}
	return nil
}

// ListEthernets returns ethernet interface name array.
func ListEthernets(ctx context.Context, dut *dut.DUT) ([]string, error) {
	cmd := fmt.Sprint(`ifconfig -s`)
	out, err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Output()
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute ifconfig command")
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
			return errors.Wrap(err, "failed to find interfaces")
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
		return nil, errors.Wrap(err, "failed to execute lsof command")
	}
	return strings.Fields(string(out)), err
}

// BuiltinUsbCamerasFromV4L2Test returns a list of builtin usb camera paths.
func BuiltinUsbCamerasFromV4L2Test(ctx context.Context, dut *dut.DUT) ([]string, error) {
	cmd := dut.Conn().CommandContext(ctx, "media_v4l2_test", "--list_builtin_usbcam")
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run media_v4l2_test")
	}
	return strings.Fields(string(out)), nil
}

// USBCamerasFromV4L2Test returns a list of usb camera paths.
func USBCamerasFromV4L2Test(ctx context.Context, dut *dut.DUT) ([]string, error) {
	cmd := dut.Conn().CommandContext(ctx, "media_v4l2_test", "--list_usbcam")
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run media_v4l2_test")
	}
	return strings.Fields(string(out)), nil
}

// VideoDevices returns a list of video devices.
func VideoDevices(ctx context.Context, dut *dut.DUT) ([]string, error) {
	cmd := dut.Conn().CommandContext(ctx, "ls", "/dev/video*")
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run ls command")
	}
	return strings.Fields(string(out)), nil
}

// DevicesFromV4L2 returns a list of all video for linux devices.
func DevicesFromV4L2(ctx context.Context, dut *dut.DUT) ([]string, error) {
	cmd := dut.Conn().CommandContext(ctx, "v4l2-ctl", "--list-devices")
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run v4l2 command")
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
