// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package graphics contains graphics-related utility functions for local tests.
package graphics

import (
	"context"
	"net"

	"chromiumos/tast/common/chameleon"
	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

var (
	chameleonHost = testing.RegisterVarString(
		"graphics.chameleon_host",
		"",
		"Hostname for Chameleon (optional/not currently used)")

	chameleonIP = testing.RegisterVarString(
		"graphics.chameleon_ip",
		"",
		"Local IP address of Chameleon (required)")

	chameleonSSHPort = testing.RegisterVarString(
		"graphics.chameleon_ssh_port",
		"22",
		"SSH port for Chameleon (optional/not currently used)")

	chameleonPort = testing.RegisterVarString(
		"graphics.chameleon_port",
		"9992",
		"Port for chameleond on Chameleon (optional/used)")
)

// ChameleonGetURL retrieves the Chameleon's IP and port to create a URL.
func ChameleonGetURL() (string, error) {
	if net.ParseIP(chameleonIP.Value()) == nil {
		return "", errors.Errorf("failed to get chameleon ip. The Chameleon ip: %s", chameleonIP.Value())
	}

	chamIP := chameleonIP.Value()
	chamPort := chameleonPort.Value()
	chamURL := net.JoinHostPort(chamIP, chamPort)
	return chamURL, nil
}

// ChameleonShouldUsePort determines whether a port is physically plugged for usage.
func ChameleonShouldUsePort(ctx context.Context, cham chameleon.Chameleond, port chameleon.PortID) (bool, error) {
	err := cham.Plug(ctx, port)
	if err != nil {
		return false, errors.Errorf("failed to plug the port %d : %s", port, err)
	}

	isPhysPlug, err := cham.IsPhysicalPlugged(ctx, port)
	if err != nil {
		return false, errors.Errorf("failed to check if port %d is physically plugged: %s", port, err)
	}

	err = cham.Unplug(ctx, port)
	if err != nil {
		return false, errors.Errorf("failed to unplug the port %d : %s", port, err)
	}
	return isPhysPlug, nil
}
