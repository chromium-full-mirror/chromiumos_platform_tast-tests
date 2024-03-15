// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package permissionbroker interacts with the permission broker system daemon.
package permissionbroker

import (
	"context"
	"os"

	"github.com/godbus/dbus/v5"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
)

const (
	jobName       = "permission_broker"
	dbusName      = "org.chromium.PermissionBroker"
	dbusPath      = "/org/chromium/PermissionBroker"
	dbusInterface = "org.chromium.PermissionBroker"

	requestTCPPortAccessMethod = "RequestTcpPortAccess"
	requestUDPPortAccessMethod = "RequestUdpPortAccess"
)

// Client is a wrapper around permission broker DBus API.
type Client struct {
	obj dbus.BusObject
}

// NewClient connects to the permission broker daemon via D-Bus and returns a
// permission broker Client object.
func NewClient(ctx context.Context) (*Client, error) {
	if err := upstart.EnsureJobRunning(ctx, jobName); err != nil {
		return nil, err
	}

	_, obj, err := dbusutil.Connect(ctx, dbusName, dbusPath)
	if err != nil {
		return nil, err
	}
	return &Client{obj}, nil
}

// RequestTCPPortAccess calls permission broker's API to allow TCP port on
// ifname. ifname can be empty to allow port on all interfaces. On success,
// lifelineFD will be returned and the caller should close this fd after the
// port is no longer needed.
func (c *Client) RequestTCPPortAccess(ctx context.Context, port uint16, ifname string) (lifelineFD *os.File, retErr error) {
	return c.callWithLifelineFD(ctx, requestTCPPortAccessMethod, port, ifname)
}

// RequestUDPPortAccess calls permission broker's API to allow UDP port on
// ifname. ifname can be empty to allow port on all interfaces. On success,
// lifelineFD will be returned and the caller should close this fd after the
// port is no longer needed.
func (c *Client) RequestUDPPortAccess(ctx context.Context, port uint16, ifname string) (lifelineFD *os.File, retErr error) {
	return c.callWithLifelineFD(ctx, requestUDPPortAccessMethod, port, ifname)
}

// callWithLifelineFD calls method with args and a lifeline fd. The method
// should return a bool which indicates whether the call succeeds.
func (c *Client) callWithLifelineFD(ctx context.Context, method string, args ...interface{}) (lifelineFD *os.File, retErr error) {
	local, remote, err := os.Pipe()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open pipe for creating %s request arg", method)

	}
	defer func() {
		remote.Close()
		if lifelineFD == nil {
			local.Close()
		}
	}()

	args = append(args, dbus.UnixFD(remote.Fd()))

	result := false
	if err := c.obj.CallWithContext(ctx, dbusInterface+"."+method, 0, args...).Store(&result); err != nil {
		return nil, errors.Wrapf(err, "failed to call %s", method)
	}
	if !result {
		return nil, errors.Errorf("%s returned false", method)
	}

	return local, nil
}
