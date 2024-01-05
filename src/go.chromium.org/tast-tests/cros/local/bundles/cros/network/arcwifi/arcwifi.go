// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package arcwifi provides utils for ARC WiFi tests.
package arcwifi

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
)

// ForgetNetwork invokes the adb command to forget the WiFi network with ssid
// inside ARC.
func ForgetNetwork(ctx context.Context, a *arc.ARC, ssid string) error {
	out, err := a.Command(ctx, "cmd", "wifi", "list-networks").Output(testexec.DumpLogOnError)
	if err != nil {
		return err
	}

	for _, network := range strings.Split(string(out), "\n") {
		// Get network information by shell cmd, if len(network) is 0, means len(netInfos) is also 0,
		// information line is empty, skip.
		if len(network) == 0 {
			continue
		}
		netInfos := strings.Split(network, " ")

		// The output is in the format of:
		// networkid=<netID> SSID="<SSID>" BSSID=<BSSID> guid=<guid> security=<security>
		// currentSSID is the SSID of current network. If currentSSID matches newly added
		// network, get networkId of this network and forget network through shell cmd.
		currentSSID := strings.Split(string(netInfos[1]), "\"")[1]

		if currentSSID == ssid {
			netID := strings.Split(netInfos[0], "=")[1]
			return a.Command(ctx, "cmd", "wifi", "forget-network", netID).Run(testexec.DumpLogOnError)
		}
	}
	return nil
}
