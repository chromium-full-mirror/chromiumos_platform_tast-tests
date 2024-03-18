// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"bufio"
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
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
