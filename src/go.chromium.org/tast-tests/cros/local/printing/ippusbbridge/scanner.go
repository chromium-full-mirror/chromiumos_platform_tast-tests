// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ippusbbridge

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/printing/usbprinter"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// findScanner runs lsusb in order to find the Bus and Device number for the USB
// device with the VID and PID given in devInfo.
func findScanner(ctx context.Context, devInfo usbprinter.DevInfo) (bus, device string, err error) {
	b, err := testexec.CommandContext(ctx, "lsusb", "-d", fmt.Sprintf("%s:%s", devInfo.VID, devInfo.PID)).Output(testexec.DumpLogOnError)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to run lsusb")
	}

	out := string(b)
	colonIndex := strings.Index(out, ":")
	if colonIndex == -1 {
		return "", "", errors.Wrap(err, "failed to find ':' in lsusb output")
	}

	tokens := strings.Split(out[:colonIndex], " ")
	if len(tokens) != 4 || tokens[0] != "Bus" || tokens[2] != "Device" {
		return "", "", errors.Errorf("failed to parse output as Bus [bus-id] Device [device-id]: %s", out)
	}

	return tokens[1], tokens[3], nil
}

// ScannerInfo represents a scanner connection mediated by ippusb_bridge.  ConnectionString can be used to open
// this scanner with lorgnette, and Cleanup should be called to shut down ippusb_bridge when the caller no longer
// needs it.
type ScannerInfo struct {
	ConnectionString string
	Cleanup          func(context.Context)
}

// PrepareScannerConnection sets up ippusb_bridge to connect to the device identified by devInfo.  If network is true, its sets up
// ippusb_bridge to listen on a network port; otherwise it will listen on a local socket in /run/ippusb.
func PrepareScannerConnection(ctx context.Context, devInfo usbprinter.DevInfo, network bool) (*ScannerInfo, error) {
	result := &ScannerInfo{
		Cleanup: func(ctx context.Context) {
			Kill(ctx, devInfo)
		},
	}
	if !network {
		// In the USB case, ippusb_bridge is already started by upstart when the USB device is plugged in.  Just wait
		// for the socket to be ready before returning the connection string.
		if err := WaitForSocket(ctx, devInfo); err != nil {
			return nil, err
		}
		result.ConnectionString = fmt.Sprintf("ippusb:escl:TestScanner:%s_%s/eSCL", devInfo.VID, devInfo.PID)
		return result, nil
	}

	// To simulate a network scanner, we start up ippusb_bridge manually and have it listen on localhost:60000.
	// Normally udev automatically starts ippusb_bridge when a USB printer is plugged in, so we have to manually
	// stop the job first.
	bus, device, err := findScanner(ctx, devInfo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find scanner bus device")
	}

	testing.ContextLog(ctx, "Stopping ippusb-bridge job")
	cmd := testexec.CommandContext(ctx, "initctl", "stop", "ippusb-bridge", fmt.Sprintf("BUS=%03s", bus), fmt.Sprintf("DEV=%03s", device))
	if err := cmd.Run(); err != nil {
		return nil, errors.Wrapf(err, "failed to stop ippusb-bridge instance for BUS=%03s DEV=%03s", bus, device)
	}

	testing.ContextLog(ctx, "Setting up ipp-usb connection")
	ippusbBridge := testexec.CommandContext(ctx, "ippusb_bridge", "--bus-device", fmt.Sprintf("%s:%s", bus, device))
	if err := ippusbBridge.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to connect to printer with ippusb_bridge")
	}

	// Defined in src/platform2/ippusb_bridge/src/main.rs
	const port = 60000

	// Wait for ippusb_bridge to start up.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		d := &net.Dialer{}
		_, err := d.DialContext(ctx, "tcp", fmt.Sprintf("localhost:%d", port))
		return err
	}, &testing.PollOptions{
		Timeout:  10 * time.Second,
		Interval: 1 * time.Second,
	}); err != nil {
		return nil, errors.Wrap(err, "failed to wait for ippusb_bridge to start")
	}

	// Even after ippusb_bridge is listening, it may take a few seconds to finish
	// probing USB devices before it responds.  In order to avoid lorgnette timing out,
	// do an initial query to make sure traffic is passing through the tunnel.
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/eSCL/ScannerCapabilities", port))
	if err != nil {
		return nil, errors.Wrap(err, "failed to send query to ippusb_bridge")
	}
	_, err = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response from ippusb_bridge")
	}

	result.ConnectionString = fmt.Sprintf("airscan:escl:TestScanner:http://localhost:%d/eSCL", port)
	return result, nil
}
