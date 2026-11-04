// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package arcwifi provides utils for ARC WiFi tests.
package arcwifi

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// AddNetwork invokes the adb command "cmd wifi add-network" with the given
// arguments inside ARC, retrying if the ARC internal handler times out and
// outputs "Save failed" (see b/356289753).
func AddNetwork(ctx context.Context, a *arc.ARC, args ...string) error {
	cmdArgs := append([]string{"wifi", "add-network"}, args...)
	return testing.Poll(ctx, func(ctx context.Context) error {
		out, err := a.Command(ctx, "cmd", cmdArgs...).Output()
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to execute cmd wifi add-network"))
		}
		if strings.Contains(string(out), "Save failed") {
			return errors.Errorf("cmd wifi add-network failed with output: %s", strings.TrimSpace(string(out)))
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  15 * time.Second,
		Interval: time.Second,
	})
}

// ForgetNetwork invokes the adb command to forget the WiFi network with ssid
// inside ARC.
func ForgetNetwork(ctx context.Context, a *arc.ARC, ssid string) error {
	out, err := a.Command(ctx, "cmd", "wifi", "list-networks").Output(testexec.DumpLogOnError)
	if err != nil {
		return err
	}

	for _, network := range strings.Split(string(out), "\n") {
		network = strings.TrimSpace(network)
		// Skip empty lines or non-network status lines (e.g., "No networks").
		if !strings.HasPrefix(network, "networkid=") {
			continue
		}
		netInfos := strings.Split(network, " ")

		if len(netInfos) < 2 {
			return errors.New("list-networks result is not valid: " + network)
		}

		netInfoSplit := strings.Split(string(netInfos[1]), "\"")
		if len(netInfoSplit) < 2 {
			return errors.New("list-networks result is not valid: " + network)
		}

		// The output is in the format of:
		// networkid=<netID> SSID="<SSID>" BSSID=<BSSID> guid=<guid> security=<security>
		// currentSSID is the SSID of current network. If currentSSID matches newly added
		// network, get networkId of this network and forget network through shell cmd.
		currentSSID := netInfoSplit[1]

		if currentSSID == ssid {
			netID := strings.Split(netInfos[0], "=")[1]
			return a.Command(ctx, "cmd", "wifi", "forget-network", netID).Run(testexec.DumpLogOnError)
		}
	}
	return nil
}
