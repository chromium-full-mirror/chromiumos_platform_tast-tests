// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cellular provides functions for testing Cellular connectivity.
package cellular

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/modemmanager"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

// CallboxServer  .
type CallboxServer struct {
	apnName   string
	DNS1      string
	DutIP     string
	Interface string
	IsIPv6    bool
}
type dnsAddress struct {
	url string
	ip  string
}

const serverPort = 9920

var (
	callboxUrls = []string{"server-callbox.cros"}
)

// NewCallboxServer creates a CallboxServer object.
func NewCallboxServer(ctx context.Context) (*CallboxServer, error) {
	ctx, st := timing.Start(ctx, "CallboxServer.NewCallboxServer")
	defer st.End()

	modem, err := modemmanager.NewModem(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Modem")
	}
	bearer, err := modem.GetFirstConnectedDataBearer(ctx, mmconst.BearerAPNTypeDefault)
	if err != nil {
		return nil, errors.Wrap(err, "error getting Connect APN properties")
	}

	interfaceName := bearer.Interface()
	if interfaceName == "" {
		return nil, errors.New("interface has no name")
	}
	apn, err := bearer.GetAPN()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get APN")
	}
	var dutIP string
	var dns1 string
	isIPv6 := false
	ip := bearer.IP4Config().Address
	if ip != "" {
		dutIP = ip
		dns1 = bearer.IP4Config().DNS1
	} else {
		ipCommand := []string{"ip", "-6", "addr", "show", "dev", interfaceName, "scope", "global", "primary"}
		stdout, stderr, err := testexec.CommandContext(ctx, ipCommand[0], ipCommand[1:]...).SeparatedOutput()
		if err != nil {
			if string(stderr) != "" {
				testing.ContextLog(ctx, "command stderr: ", string(stderr))
			}
			return nil, errors.Wrap(err, "failed to get ipv6 address")
		}
		r := regexp.MustCompile(`([a-f0-9:]+:+[a-f0-9]+)\/`)
		m := r.FindStringSubmatch(string(stdout))
		if len(m) != r.NumSubexp()+1 {
			return nil, errors.New("could not parse ip addr result")
		}
		dutIP = m[1]
		dns1 = bearer.IP6Config().DNS1
		isIPv6 = true
	}

	server := CallboxServer{apnName: apn, DNS1: dns1, DutIP: dutIP, IsIPv6: isIPv6, Interface: interfaceName}
	return &server, nil
}

// ResetEntitlementValueForThisDevice configures the callbox to return |value| when a request with imsi+ip is received, where ip is the current IP of the DUT.
func (srv *CallboxServer) ResetEntitlementValueForThisDevice(ctx context.Context, imsi string) error {
	params := map[string]interface{}{"imsi": imsi}
	return srv.sendServerCommand(ctx, "ResetEntitlementValueForDevice", params)
}

// SetupEntitlementReturnCodeForThisDevice configures the callbox to return |value| when a request with imsi+ip is received, where ip is the current IP of the DUT.
func (srv *CallboxServer) SetupEntitlementReturnCodeForThisDevice(ctx context.Context, imsi string, value int32) error {
	params := map[string]interface{}{"imsi": imsi, "code": value}
	return srv.sendServerCommand(ctx, "SetupEntitlementReturnCodeForDevice", params)
}

// IgnoreNextEntitlementCheckForThisDevice configures the ignore the next entielment check from the DUT.
func (srv *CallboxServer) IgnoreNextEntitlementCheckForThisDevice(ctx context.Context) error {
	return srv.sendServerCommand(ctx, "IgnoreNextEntitlementCheckForDevice", nil)
}

func (srv *CallboxServer) sendServerCommand(ctx context.Context, srvCommand string, params map[string]interface{}) error {
	message, err := json.Marshal(map[string]interface{}{"command": srvCommand, "params": params})
	if err != nil {
		return errors.Wrap(err, "failed to marshal command")
	}
	ipType := "ipv4"
	if srv.IsIPv6 {
		ipType = "ipv6"
	}
	curlCommand := []string{"sudo", "-u", "shill", "curl", "--connect-timeout", "5", "--max-time", "10",
		fmt.Sprintf("http://server-callbox.cros:%d/server_command", serverPort),
		"--interface", srv.Interface, "--dns-interface", srv.Interface, "--dns-servers", srv.DNS1, fmt.Sprintf("--dns-%s-addr", ipType), srv.DutIP,
		"--request", "POST", "--header", "Content-Type:application/json",
		"--data-raw", string(message)}

	stdout, stderr, err := testexec.CommandContext(ctx, curlCommand[0], curlCommand[1:]...).SeparatedOutput()
	if err != nil {
		if string(stderr) != "" {
			testing.ContextLog(ctx, "command stderr: ", string(stderr))
		}
		return errors.Wrap(err, "failed to send request")
	}

	if string(stdout) != "" {
		testing.ContextLog(ctx, "command stdout: ", stdout)
	}
	return nil

}
