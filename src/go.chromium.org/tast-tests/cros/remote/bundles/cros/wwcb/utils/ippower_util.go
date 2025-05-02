// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
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

// WWCBIPPowerIP is a variable to set IP address of IP Power 9858MT to send HTTP request in WWCB tests.
var WWCBIPPowerIP = testing.RegisterVarString(
	"utils.wwcbIPPowerIp",
	"",
	"A variable to set IP address of IP Power 9858MT to send HTTP request in WWCB tests.",
)

// WWCBIPowerRPM build a labapi.RPM proto object from the optional endpoint
// (<host>[:<port>]) or from the WWCBIPPowerIP variable passed to the test
// this is only to be used for generating when it is not part of the topology
// and for backwards compatibility with old references/the
// `utils.wwcbIPPowerIp` variable.
//
// endpoint: optional endpoint to skip reading variable used for unit tests
func WWCBIPowerRPM(endpoint string) *labapi.RPM {
	if endpoint == "" {
		// Skip reference the tast variable if endpoint is passed in
		// this allows unit tests to avoid panic due to GlobalRegistry for
		// tast being unset.
		if WWCBIPPowerIP.Value() == "" {
			return nil
		}
		endpoint = WWCBIPPowerIP.Value()
	}
	host, portStr, err := net.SplitHostPort(endpoint)
	if err != nil {
		host = endpoint
		portStr = "80"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil
	}
	return &labapi.RPM{
		Present: true,
		PowerUnitHostname: &labapi.IpEndpoint{
			Address: host,
			Port:    int32(port),
		},
		PowerUnitOutlet: "1",
		Type:            labapi.RPMType_RPM_TYPE_IP9850,
	}
}

// SetRPM sets the power for port(specified in the rpm) to the state specified
// by enabled.
//
// ctx: a context to use
// rpm: proto specifying the host/power port to be set
// enabled: true = power on the port, false = power off the port
func SetRPM(ctx context.Context, rpm *labapi.RPM, enabled bool) error {
	if rpm == nil || !rpm.GetPresent() {
		return errors.New("no IP power specified")
	}
	if rpm.Type != labapi.RPMType_RPM_TYPE_IP9850 {
		return errors.Errorf("invalid RPM type %v", rpm)
	}
	address := fmt.Sprintf("%s:%d", rpm.PowerUnitHostname.Address, rpm.PowerUnitHostname.Port)
	port, err := strconv.Atoi(rpm.PowerUnitOutlet)
	if err != nil {
		return errors.Wrapf(err, "RPM has invalid port must be an integer: %v", rpm)
	}
	return SetIPPower(ctx, address, []int{port}, enabled)
}

// SetIPPower is for setting the power of a given port for an IP Power switch
//
// address: address in format of <ip or host>[:port]
// ports: list of ports to be set must be betewee minPortNum and maxPortNum
// enabled: true = power on the ports, false = power off the ports
func SetIPPower(ctx context.Context, address string, ports []int, enabled bool) error {
	myport := ""
	enableval := "0"
	if enabled {
		enableval = "1"
	}
	for _, port := range ports {
		if port < minPortNum || port > maxPortNum {
			return errors.New("the port format error")
		}

		s := fmt.Sprintf("+p6%v=%s", port, enableval)
		myport += s
	}

	url := fmt.Sprintf("http://%s/set.cmd?user=%s+pass=%s+cmd=setpower%s", address, user, password, myport)
	testing.ContextLogf(ctx, "Setting IPPower with request: %s", url)
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

// OpenIppower is for open ip power.
//
// Deprecated only used for selftest
func OpenIppower(ctx context.Context, ports []int) error {
	return SetIPPower(ctx, WWCBIPPowerIP.Value(), ports, true)
}

// CloseIppower is for close ip power.
//
// Deprecated only used for selftest
func CloseIppower(ctx context.Context, ports []int) error {
	return SetIPPower(ctx, WWCBIPPowerIP.Value(), ports, false)
}

// IppowerIP is designed to obtain the IP address of IP Power from a specified network segment.
//
// Deprecated only used for selftest
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
		body, err := io.ReadAll(resp.Body)
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
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Errorf("unable to read the body content from %s", ip)
	}
	if !strings.Contains(string(body), "p61=") {
		return errors.New("the content returned by IP Power does not contain 'p61='")
	}
	return nil
}
