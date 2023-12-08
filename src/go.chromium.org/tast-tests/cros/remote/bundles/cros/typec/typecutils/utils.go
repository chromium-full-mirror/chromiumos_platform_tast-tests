// Copyright 2021 The ChromiumOS Authors
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

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/services/cros/typec"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
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
	partnerPath           = "/sys/class/typec/port0-partner"
	thunderboltDevicePath = "/sys/bus/thunderbolt/devices"
)

// Identifiers used to specify which Thunderbolt generation is being checked.
const (
	TbtGenAny = iota
	TbtGen4   = 4
)

// List of built-in Thunderbolt devices enumerated by the OS.
var builtInTBTDevices = []string{"domain0", "domain1", "0-0", "1-0"}

// CcOffAndWait performs a CC Off command, followed by a sleep to ensure VBus discharges safely before any further modification.
func CcOffAndWait(ctx context.Context, svo *servo.Servo) error {
	if err := svo.SetCC(ctx, servo.Off); err != nil {
		return errors.Wrap(err, "failed to switch off CC")
	}

	if err := testing.Sleep(ctx, tVBusDischargeCcOff); err != nil {
		return errors.Wrap(err, "failed to sleep after CC off")
	}

	return nil
}

// CheckForDPAltMode verifies that a partner was enumerated with the expected DP altmode with the
// selected pin assignment setting(if provided).
func CheckForDPAltMode(ctx context.Context, d *dut.DUT, s *testing.State, pinAssign string) error {
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

		if (pinAssign == "c" && vdoVal&pinCBitMask != 0) ||
			(pinAssign == "d" && vdoVal&pinDBitMask != 0) {
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

// VerifyTXSpeed verfies TX speed of TBT Device.
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

// VerifyRXSpeed verfies TX speed of TBT Device.
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

// builtInTBTDevice returns whether the specified name is a built-in Thunderbolt device or not.
func builtInTBTDevice(name string) bool {
	for _, device := range builtInTBTDevices {
		if name == device {
			return true
		}
	}

	return false
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

// CheckTBTDevice is a helper function which checks for TBT device connection to a DUT.
// |expected| specifies whether we want to check for the presence of a TBT device (true) or the
// absence of one (false).
//
// Caller can specify |gen| to check for a specific Thunderbolt
// generation, or `TbtGenAny` if the check is not necessary.
func CheckTBTDevice(ctx context.Context, d *dut.DUT, expected bool, gen int) error {
	out, err := d.Conn().CommandContext(ctx, "ls", thunderboltDevicePath).Output()
	if err != nil {
		return errors.Wrap(err, "could not run ls command on DUT")
	}

	found := ""
	for _, device := range strings.Split(string(out), "\n") {
		if device == "" {
			continue
		}

		if builtInTBTDevice(device) {
			continue
		}

		// Check for retimers.
		// They are of the form "0-0:1.1" or "0-0:3.1".
		if matched, err := regexp.MatchString(`[\d\-\:]+\.\d`, device); err != nil {
			return errors.Wrap(err, "couldn't execute retimer regexp")
		} else if matched {
			continue
		}

		found = device
		break
	}

	if expected && found == "" {
		return errors.New("no TBT device found")
	} else if !expected && found != "" {
		return errors.Errorf("TBT device found: %s", found)
	}

	if expected && gen != TbtGenAny {
		out, err := d.Conn().CommandContext(ctx, "cat", filepath.Join(thunderboltDevicePath, found, "generation")).Output()
		if err != nil {
			return errors.Wrap(err, "couldn't read Thunderbolt generation")
		}

		if genFound, err := strconv.Atoi(strings.TrimSpace(string(out))); err != nil {
			return errors.Wrap(err, "couldn't parse Thunderbolt generation")
		} else if genFound != gen {
			return errors.Errorf("incorrect Thunderbolt generation; found: %d expected: %d", genFound, gen)
		}
	}

	return nil
}

// LoginChrome is a helper which performs a Chrome Login with the Peripheral Data Access setting disabled.
func LoginChrome(ctx context.Context, d *dut.DUT, s *testing.State, keyFile string) error {
	// Connect to gRPC server.
	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}

	// Send key file to DUT.
	keyPath := filepath.Join("/tmp", keyFile)
	defer d.Conn().CommandContext(ctx, "rm", "-r", keyPath).Output()
	if _, err := linuxssh.PutFiles(
		ctx, d.Conn(), map[string]string{
			s.DataPath(keyFile): keyPath,
		},
		linuxssh.DereferenceSymlinks); err != nil {
		return errors.Wrapf(err, "failed to send data to remote data path %v", keyPath)
	}

	// Log in to Chrome.
	client := typec.NewServiceClient(cl.Conn)
	_, err = client.NewChromeLoginWithPeripheralDataAccess(ctx, &typec.KeyPath{Path: keyPath})
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}

	return nil
}
