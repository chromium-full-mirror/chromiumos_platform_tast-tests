// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

// DeviceScanner returns the evtest scanner for the touch pad and touch screen device
// by running the command on the DUT.
func DeviceScanner(ctx context.Context, h *firmware.Helper, devPath string) (*ssh.Cmd, *bufio.Scanner, error) {
	// Declare a bufio.Scanner for detecting touchpad and touch screen.
	cmd := h.DUT.Conn().CommandContext(ctx, "evtest", devPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create stdout pipe")
	}

	if err := cmd.Start(); err != nil {
		return nil, nil, errors.Wrap(err, "failed to start scanner")
	}

	scanner := bufio.NewScanner(stdout)
	return cmd, scanner, nil
}

// EvtestMonitor is used to check whether events sent to the devices are picked up by the evtest.
func EvtestMonitor(ctx context.Context, scanner *bufio.Scanner) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	evtestRe := regexp.MustCompile(`Event.*time.*code\s(\d*)\s\(BTN_TOUCH\)`)
	text := make(chan string)
	go func() {
		for scanner.Scan() {
			text <- scanner.Text()
		}
		defer close(text)
	}()
	for {
		select {
		case <-timeoutCtx.Done():
			return errors.New("failed to detect events within expected time")
		case out := <-text:
			if match := evtestRe.FindStringSubmatch(out); match != nil {
				return nil
			}
		}
	}
}

// FindRegisteredDevice finds registered input devices by reading a file under the `proc` virtual filesystem,
// returns the id of the input device with specified name |deviceName|.
// If the expected device name is not found in the file, the whole file content will be dumped into the |outDir| directory.
func FindRegisteredDevice(ctx context.Context, dut *dut.DUT, deviceName, outDir string) (_ int, retErr error) {
	// inputDevicesVirtualFile is the file that provides information about the input devices currently recognized by the kernel.
	const inputDevicesVirtualFile = "/proc/bus/input/devices"

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

	var deviceIDStr string

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
		data, err := linuxssh.ReadFile(ctx, dut.Conn(), inputDevicesVirtualFile)
		if err != nil {
			return errors.Wrap(err, "failed to acquire the full info of all input devices")
		}
		inputDevices = data

		ss := reg.FindStringSubmatch(string(data))
		// Expecting 7 sub-matches which are the entire match, 5 line-breaks and the device-ID.
		if ss == nil || len(ss) != 7 {
			return errors.New("failed to find the input device id")
		}
		deviceIDStr = ss[5]

		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
		return -1, errors.Wrap(err, "failed to parse the file describing input devices")
	}

	deviceID, err := strconv.Atoi(deviceIDStr)
	if err != nil {
		return -1, err
	}

	return deviceID, nil
}
