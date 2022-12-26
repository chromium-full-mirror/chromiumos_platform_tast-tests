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

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/dut"
	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
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

		out, err := dut.Conn().CommandContext(ctx, "power_supply_info").Output()
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

// VerifyEthernetStatus verifies whether the Ethernet device is connected or not.
func VerifyEthernetStatus(ctx context.Context, dut *dut.DUT, isConnected bool) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		cmd := `ls /sys/class/net | grep eth`
		ethernets, err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Output()
		if err != nil {
			if isConnected {
				return errors.Wrap(err, "failed to retrieve Ethernet devices")
			}
			// Consider Ethernet devices are not found as disconnected.
			return nil
		}

		status := false
		for _, eth := range strings.Split(strings.TrimSpace(string(ethernets)), "\n") {
			cmd := fmt.Sprintf("cat /sys/class/net/%s/operstate", eth)
			out, err := dut.Conn().CommandContext(ctx, "sh", "-c", cmd).Output()
			if err != nil {
				return errors.Wrap(err, "failed to retrieve Ethernet state")
			}

			if "up" == strings.TrimSpace(string(out)) {
				status = true
				break
			}
		}

		if status != isConnected {
			return errors.Errorf("unexpected Ethernet status, got: %t, want: %t", status, isConnected)
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
func VerifyPeripheralsConnection(ctx context.Context, dut *dut.DUT, isConnected bool, expectUSBTypeADeviceNum int) error {
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

	if err := VerifyEthernetStatus(testingCtx, dut, isConnected); err != nil {
		return errors.Wrap(err, "failed to verify connection of Ethernet")
	}

	if err := VerifyTypeADevicesCount(testingCtx, dut, expectUSBTypeADeviceNum); err != nil {
		return errors.Wrap(err, "failed to verify connection of USB Type-A devices")
	}
	return nil
}
