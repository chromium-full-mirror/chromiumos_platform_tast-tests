// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
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

// IppowerIP is designed to obtain the IP address of IP Power from a specified network segment.
func IppowerIP(dutIP string) (string, error) {
	subnet := strings.Join(strings.Split(dutIP, ".")[:3], ".")
	client := http.Client{
		Timeout: 600 * time.Millisecond,
	}

	for i := 1; i < 255; i++ {
		url := fmt.Sprintf("http://%s.%d/set.cmd?user=%s+pass=%s+cmd=getpower", subnet, i, user, password)
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			resp.Body.Close()
			continue
		}
		if strings.Contains(string(body), "p61=") {
			resp.Body.Close()
			return fmt.Sprintf("%s.%d", subnet, i), nil
		}
		resp.Body.Close()
	}
	return "", errors.Errorf("unable to obtain IP Power IP from the %s network segment", subnet)
}

// CheckIppowerStatus is for check if IP Power is online.
func CheckIppowerStatus(ip string) error {
	client := http.Client{
		Timeout: 600 * time.Millisecond,
	}
	url := fmt.Sprintf("http://%s/set.cmd?user=%s+pass=%s+cmd=getpower", ip, user, password)
	resp, err := client.Get(url)
	if err != nil {
		return errors.Errorf("unable to send a GET request to %s", ip)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Errorf("unable to read the body content from %s", ip)
	}
	if !strings.Contains(string(body), "p61=") {
		return errors.New("the content returned by IP Power does not contain 'p61='")
	}
	return nil
}
