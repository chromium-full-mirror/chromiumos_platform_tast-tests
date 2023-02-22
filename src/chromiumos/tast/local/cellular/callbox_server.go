// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cellular provides functions for testing Cellular connectivity.
package cellular

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"chromiumos/tast/common/mmconst"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/local/modemmanager"
	"chromiumos/tast/testing"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/timing"
)

// CallboxServer  .
type CallboxServer struct {
	apnName   string
	DutIP     string
	Interface string
	// IpAddresses are the IP address of the callbox and server
	IPAddresses []string
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
// The function returns a closure to undo the configuration changes made during setup.
func NewCallboxServer(ctx context.Context) (*CallboxServer, func(), error) {
	ctx, st := timing.Start(ctx, "CallboxServer.NewCallboxServer")
	defer st.End()

	modem, err := modemmanager.NewModem(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create Modem")
	}
	bearer, err := modem.GetFirstConnectedDataBearer(ctx, mmconst.BearerAPNTypeDefault)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error getting Connect APN properties")
	}

	interfaceName := bearer.Interface()
	if interfaceName == "" {
		return nil, nil, errors.New("interface has no name")
	}

	apn, err := bearer.GetAPN()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get APN")
	}
	var gateway []string
	var dutIP string
	ip := bearer.IP4Config().Address
	if ip != "" {
		dutIP = ip
		if strings.LastIndex(ip, ".") > 0 {
			gateway = append(gateway, ip[0:strings.LastIndex(ip, ".")+1]+"1")
		}
	} else {
		dutIP = bearer.IP6Config().Address
	}
	ip = bearer.IP6Config().Address
	if ip != "" {
		// TODO(b/276758876): hardcode the IPv6 gateway until we implement the DNS server on the callbox.
		gateway = append(gateway, "2001:468:3000:1::0")
	}

	if len(gateway) == 0 {
		return nil, nil, errors.New("the bearer has no valid addresses")
	}
	server := CallboxServer{apnName: apn, DutIP: dutIP, IPAddresses: gateway, Interface: interfaceName}
	cleanupFunction, err := ConfigureCallboxHostNamesOnDut(ctx, server.IPAddresses)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error configuring the callbox URLs")
	}

	return &server, cleanupFunction, nil
}

// CleanUpCallboxUrlsFromEtcHosts removes all the custom urls for the callbox from /etc/hosts
func CleanUpCallboxUrlsFromEtcHosts(ctx context.Context) error {
	input, err := ioutil.ReadFile("/etc/hosts")
	if err != nil {
		return errors.Wrap(err, "failed to read /etc/hosts")
	}

	lines := strings.Split(string(input), "\n")
	// ensure each url is removed.
	var newLines []string
	for _, line := range lines {
		for _, url := range callboxUrls {
			if !strings.Contains(line, url) {
				newLines = append(newLines, line)
			}
		}
	}
	output := strings.Join(newLines, "\n")
	err = ioutil.WriteFile("/etc/hosts", []byte(output), 0644)
	if err != nil {
		return errors.Wrap(err, "failed to write /etc/hosts")
	}
	return nil
}

// ConfigureCallboxHostNamesOnDut will add the IP address of the callbox into /etc/hosts and map it to URLs used in tests so
// the tests can communicate with the callbox transparently.
// The function returns a closure to undo the changes to /etc/hosts.
func ConfigureCallboxHostNamesOnDut(ctx context.Context, gatewayIPs []string) (func(), error) {
	input, err := ioutil.ReadFile("/etc/hosts")
	if err != nil {
		return nil, errors.Wrap(err, "failed to read /etc/hosts")
	}

	lines := strings.Split(string(input), "\n")
	for _, url := range callboxUrls {
		for _, gatewayIP := range gatewayIPs {
			host := fmt.Sprintf("%s %s", gatewayIP, url)
			lines = append([]string{host}, lines...)
		}
	}
	output := strings.Join(lines, "\n")
	err = ioutil.WriteFile("/etc/hosts", []byte(output), 0644)
	if err != nil {
		return nil, errors.Wrap(err, "failed to write /etc/hosts")
	}
	return func() { CleanUpCallboxUrlsFromEtcHosts(ctx) }, nil
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

	var curlCommand []string
	curlCommand = []string{"sudo", "-u", "shill", "curl", "--connect-timeout", "5", "--max-time", "10",
		fmt.Sprintf("http://server-callbox.cros:%d/server_command", serverPort),
		"--interface", srv.Interface, "--request", "POST", "--header", "Content-Type:application/json",
		"--data-raw", string(message)}

	stdout, stderr, err := testexec.CommandContext(ctx, curlCommand[0], curlCommand[1:]...).SeparatedOutput()
	if err != nil {
		if string(stderr) != "" {
			testing.ContextLog(ctx, "command stderr: ", stderr)
		}
		return errors.Wrap(err, "failed to send request")
	}

	if string(stdout) != "" {
		testing.ContextLog(ctx, "command stdout: ", stdout)
	}
	return nil

}
