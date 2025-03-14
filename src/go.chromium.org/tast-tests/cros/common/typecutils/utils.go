// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package typecutils contains constants & helper functions used by the tests in the typec directory.
package typecutils

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/services/cros/usb"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

// Wait for VBUS to discharge in Servo.
// We do this after a CC off, so that a subsequent CC on (either directly or implicitly through src dts changes) doesn't
// confuse certain TCPCs, causing Cr50 UART to be wedged on servo.
// Request = tRequest(24ms) + tSenderResponse(24ms) + tSrcTransition(35ms) + PSTransitionTimer(550ms) = 633ms (rounded up to 650ms)
// Hard Reset = 650ms + tSrcRecover(1s) + tSrcTurnOn(275ms) = 1925ms
// Total time = 1925ms * 1.5 (50% buffer) = 2887ms (rounded up to 3s)
const (
	tVBusDischargeCcOff = 3 * time.Second
)

// Bitmasks for pin assignments C and D in the DisplayPort (DP) alternate mode Vendor-Defined Object (VDO) response.
const (
	pinCBitMask = 0x400
	pinDBitMask = 0x800
)

const (
	// Filepath on the DUT for the servo Type C partner device.
	partnerPath = "/sys/class/typec/port0-partner"
)

// USB Class Codes.
const (
	ClassHid         = 0x3
	ClassMassStorage = 0x8
)

// CcOffAndWait performs a CC Off command, followed by a sleep to ensure VBus discharges safely before any further modification.
func CcOffAndWait(ctx context.Context, svo *servo.Servo) error {
	if err := svo.SetCC(ctx, servo.Off); err != nil {
		return errors.Wrap(err, "failed to switch off CC")
	}

	// GoBigSleepLint: Enusure that VBus discharges safely
	if err := testing.Sleep(ctx, tVBusDischargeCcOff); err != nil {
		return errors.Wrap(err, "failed to sleep after CC off")
	}

	return nil
}

// CheckForDPAltMode verifies that a partner was enumerated with the expected DP altmode with the
// selected pin assignment setting(if provided).
func CheckForDPAltMode(ctx context.Context, d *dut.DUT, pinAssign string) error {
	// Servo is always on port 0.
	out, err := d.Conn().CommandContext(ctx, "ls", partnerPath).Output()
	if err != nil {
		return errors.Wrap(err, "could not run ls command on DUT")
	}

	altModeDevice := regexp.MustCompile(`port0-partner\.\d`)
	for _, device := range bytes.Split(out, []byte("\n")) {
		// We're only interested in the alternate mode devices.
		if !altModeDevice.Match(device) {
			continue
		}

		modePath := filepath.Join(partnerPath, string(device))

		// Check that the alt mode has the DP SVID: 0xff01.
		svidPath := filepath.Join(modePath, "svid")
		if out, err := linuxssh.ReadFile(ctx, d.Conn(), svidPath); err != nil {
			return errors.Wrap(err, "couldn't read alt mode svid")
		} else if !bytes.Contains(out, []byte("ff01")) {
			continue
		}

		// If we aren't interested in the Pin Assignment, return immediately.
		if pinAssign == "" {
			return nil
		}

		// Read the alt mode's VDO to determine the advertised pin assignment.
		vdoPath := filepath.Join(modePath, "vdo")
		out, err := linuxssh.ReadFile(ctx, d.Conn(), vdoPath)
		if err != nil {
			return errors.Wrap(err, "couldn't read alt mode vdo")
		}

		vdoVal, err := strconv.ParseInt(strings.TrimSpace(string(out)), 0, 64)
		if err != nil {
			errors.Wrap(err, "couldn't parse VDO content of alt mode into int")
		}

		if (strings.ToLower(pinAssign) == "c" && vdoVal&pinCBitMask != 0) ||
			(strings.ToLower(pinAssign) == "d" && vdoVal&pinDBitMask != 0) {
			return nil
		}
	}

	return errors.Errorf("didn't find the right DP alternate mode registered for partner for pin assignment %s", pinAssign)
}

// IsDeviceEnumerated validates device enumeration in DUT.
// device holds the device name of connected TBT/USB4 device.
// port holds the TBT/USB4 port ID in DUT.
func IsDeviceEnumerated(ctx context.Context, dut *dut.DUT, device, port string) (bool, error) {
	deviceNameFile := fmt.Sprintf("/sys/bus/thunderbolt/devices/%s/device_name", port)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := linuxssh.ReadFile(ctx, dut.Conn(), deviceNameFile)
		if err != nil {
			return errors.Wrapf(err, "failed to read %q file", deviceNameFile)
		}

		if strings.TrimSpace(string(out)) != device {
			return errors.New("device enumeration failed")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 1 * time.Second}); err != nil {
		return false, err
	}
	return true, nil
}

// CheckUSBPdMuxinfo verifies whether USB4=1 or not.
func CheckUSBPdMuxinfo(ctx context.Context, dut *dut.DUT, deviceStr string) error {
	out, err := dut.Conn().CommandContext(ctx, "ectool", "usbpdmuxinfo").Output()
	if err != nil {
		return errors.Wrap(err, "failed to execute ectool usbpdmuxinfo command")
	}
	if !strings.Contains(string(out), deviceStr) {
		return errors.Wrapf(err, "failed to find %s in usbpdmuxinfo", deviceStr)
	}
	return nil
}

// CableConnectedPortNumber on success will returns Active/Passive cable connected port number.
func CableConnectedPortNumber(ctx context.Context, dut *dut.DUT, connector string) (string, error) {
	out, err := dut.Conn().CommandContext(ctx, "ectool", "usbpdmuxinfo").Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to execute ectool usbpdmuxinfo command")
	}
	portRe := regexp.MustCompile(fmt.Sprintf(`Port.([0-9]):.*(%s=1)`, connector))
	portNum := portRe.FindStringSubmatch(string(out))
	if len(portNum) == 0 {
		return "", errors.New("failed to get port number from usbpdmuxinfo")
	}
	return portNum[1], nil
}

// VerifyTXSpeed verifies TX speed of TBT Device.
func VerifyTXSpeed(ctx context.Context, dut *dut.DUT, device, port string) (bool, error) {
	deviceNameFile := fmt.Sprintf("/sys/bus/thunderbolt/devices/%s/tx_speed", port)
	out, err := linuxssh.ReadFile(ctx, dut.Conn(), deviceNameFile)
	if err != nil {
		return false, errors.Wrapf(err, "failed to read %q file", deviceNameFile)
	}

	if strings.TrimSpace(string(out)) != device {
		return false, errors.New("failed to verify TX speed")
	}

	return true, nil
}

// VerifyRXSpeed verifies TX speed of TBT Device.
func VerifyRXSpeed(ctx context.Context, dut *dut.DUT, device, port string) (bool, error) {
	deviceNameFile := fmt.Sprintf("/sys/bus/thunderbolt/devices/%s/rx_speed", port)
	out, err := linuxssh.ReadFile(ctx, dut.Conn(), deviceNameFile)
	if err != nil {
		return false, errors.Wrapf(err, "failed to read %q file", deviceNameFile)
	}

	if strings.TrimSpace(string(out)) != device {
		return false, errors.New("failed to verify RX speed")
	}

	return true, nil
}

// FindConnectedDp returns a list of all connected DP connectors.
func FindConnectedDp(ctx context.Context, d *dut.DUT) ([]string, error) {
	var ret []string

	out, err := d.Conn().CommandContext(ctx, "modetest", "-c").Output()
	if err != nil {
		return ret, errors.Wrap(err, "could not run modetest command on DUT")
	}

	for _, line := range strings.Split(string(out), "\n") {
		// Look for patterns in the following lines:
		//
		// 273     0       disconnected    DP-3            0x0             0       272
		// 254     253     connected       DP-1            0x0             24      253
		// We need to match for both "connected" *and* "disconnected" since one is a subset of the other.
		connectorRe := regexp.MustCompile(`.+\s+(disconnected|connected)\s+(DP-\d)`)
		connector := connectorRe.FindStringSubmatch(line)
		if len(connector) != 3 {
			continue
		}

		if connector[1] == "connected" {
			ret = append(ret, connector[2])
		}
	}

	return ret, nil
}

// UnmountRemovableMedia is a helper to unmount removable media before they are disconnected.
func UnmountRemovableMedia(ctx context.Context, d *dut.DUT) error {
	base := "/media/removable"

	paths, err := d.Conn().CommandContext(ctx, "ls", base).Output()
	if err != nil {
		return errors.Wrap(err, "could not list "+base)
	}

	// Unmount each path in /media/removable.
	for _, path := range strings.Split(string(paths), "\n") {
		d.Conn().CommandContext(ctx, "umount", filepath.Join(base, strings.TrimSpace(string(path)))).Run()
	}

	mount, err := d.Conn().CommandContext(ctx, "mount").Output()
	if err != nil {
		return errors.Wrap(err, "could not check mounted file systems")
	}

	// Confirm that no mounted files systems are at /media/removable.
	if strings.Contains(string(mount), base) {
		return errors.New("mounted removable media found")
	}
	return nil
}

// ListBlockDevices creates a list of block devices on the DUT matching "/dev/sdX".
func ListBlockDevices(ctx context.Context, d *dut.DUT) ([]string, error) {
	var ret []string

	paths, err := d.Conn().CommandContext(ctx, "ls", "/dev").Output()
	if err != nil {
		return ret, errors.Wrap(err, "failed to list devices")
	}

	blockdevRe := regexp.MustCompile(`sd[a-z]$`)
	for _, path := range strings.Split(string(paths), "\n") {
		if !blockdevRe.Match([]byte(path)) {
			continue
		}

		ret = append(ret, path)
	}

	return ret, nil
}

// Usb3GetExternalStorageList returns a list of currently connected external USB storage devices
// based on the removable property and interface classes. The returned value is an array of
// strings containing each devices address (example: "3-2.1.3").
func Usb3GetExternalStorageList(ctx context.Context, cl usb.SysfsServiceClient) ([]string, error) {
	var externalStorageList []string

	deviceMap, err := cl.GetDevices(ctx, &empty.Empty{})
	if err != nil {
		return externalStorageList, errors.Wrap(err, "unable to get USB device map")
	}

	for addr, device := range deviceMap.Devices {
		if device.Removable == usb.RemovableAttribute_REMOVABLE_ATTRIBUTE_FIXED || device.Speed <= 480 {
			continue
		}

		for _, interf := range device.Interfaces {
			if interf.InterfaceClass == ClassMassStorage {
				externalStorageList = append(externalStorageList, addr)
				break
			}
		}
	}

	return externalStorageList, nil
}

// Usb2GetHidDeviceList returns a list of currently connected external USB HID devices. The returned
// value is an array of strings containing each devices address (example: "3-2.1.3").
func Usb2GetHidDeviceList(ctx context.Context, cl usb.SysfsServiceClient) ([]string, error) {
	var hidDeviceList []string

	deviceMap, err := cl.GetDevices(ctx, &empty.Empty{})
	if err != nil {
		return hidDeviceList, errors.Wrap(err, "unable to get USB device map")
	}

	for addr, device := range deviceMap.Devices {
		if device.Removable == usb.RemovableAttribute_REMOVABLE_ATTRIBUTE_FIXED || device.Speed > 480 {
			continue
		}

		for _, interf := range device.Interfaces {
			if interf.InterfaceClass == ClassHid {
				hidDeviceList = append(hidDeviceList, addr)
				break
			}
		}
	}

	return hidDeviceList, nil
}

// CheckPowerRole verifies that the power role on a specified port is the expected one.
func CheckPowerRole(ctx context.Context, dut *dut.DUT, powerRole string, portId uint8) error {
	powerRole = strings.ToUpper(powerRole)

	out, err := dut.Conn().CommandContext(ctx, "ectool", "typecstatus", fmt.Sprintf("%d", portId)).Output()
	if err != nil {
		return errors.Wrap(err, "failed to execute ectool typecstatus command")
	}

	if !strings.Contains(string(out), fmt.Sprintf("Role:%s", powerRole)) {
		return errors.Wrapf(err, "power role was not %s", powerRole)
	}
	return nil
}
