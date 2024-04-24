// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	// user is the default login user name of the IP Power 9858MT.
	user = "admin"

	// password is the default login password of the IP Power 9858MT.
	password = "12345678"

	// minPortNum is the minimum number of 4 ports of IP Power 9858MT.
	minPortNum = 1

	// minPortNum is the maximum number of 4 ports of IP Power 9858MT.
	maxPortNum = 4
)

// wwcbIPPowerIP is a variable to set IP address of IP Power 9858MT to send HTTP request in WWCB tests.
var wwcbIPPowerIP = testing.RegisterVarString(
	"utils.wwcbIPPowerIp",
	"192.168.1.168",
	"A variable to set IP address of IP Power 9858MT to send HTTP request in WWCB tests.",
)

// OpenIppower is for open ip power.
func OpenIppower(ctx context.Context, ports []int) error {
	myport := ""
	for _, port := range ports {
		if port < minPortNum || port > maxPortNum {
			return errors.New("the port format error")
		}

		s := fmt.Sprintf("+p6%v=1", port)
		myport += s
	}

	url := fmt.Sprintf("http://%s/set.cmd?user=%s+pass=%s+cmd=setpower%s", wwcbIPPowerIP.Value(), user, password, myport)
	testing.ContextLogf(ctx, "request: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return errors.Wrap(err, "failed to send request")
	}
	defer resp.Body.Close()
	// GoBigSleepLint: To prevent the simultaneous triggering of power and device connections,
	// which can lead to device loss.
	testing.Sleep(ctx, 5*time.Second)
	return nil
}

// CloseIppower is for close ip power.
func CloseIppower(ctx context.Context, ports []int) error {
	myport := ""
	for _, port := range ports {
		if port < minPortNum || port > maxPortNum {
			return errors.New("the port format error")
		}
		s := fmt.Sprintf("+p6%v=0", port)
		myport += s
	}

	url := fmt.Sprintf("http://%s/set.cmd?user=%s+pass=%s+cmd=setpower%s", wwcbIPPowerIP.Value(), user, password, myport)
	testing.ContextLogf(ctx, "request: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return errors.Wrap(err, "failed to send request")
	}
	defer resp.Body.Close()
	return nil
}
