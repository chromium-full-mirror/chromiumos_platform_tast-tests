// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package topology contains tools to interact with PASIT topology components.
package topology

import (
	"context"
	"fmt"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
	"go.chromium.org/chromiumos/config/go/test/lab/api/passport"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils/api"
	"go.chromium.org/tast/core/errors"
)

// Connection is a directed connection between two components in the PASIT topology.
// It wraps switches (fixtures) to abstract away the switch "activation" implementations.
type Connection struct {
	connection *labapi.PasitHost_Connection
	childID    string
	manager    api.SwitchService
}

// newConnection creates a connection between to components.
func newConnection(connection *labapi.PasitHost_Connection, reverse bool, manager api.SwitchService) *Connection {
	childID := connection.GetChildId()
	if reverse {
		childID = connection.GetParentId()
	}

	return &Connection{
		connection: connection,
		childID:    childID,
		manager:    manager,
	}
}

func (c *Connection) String() string {
	return fmt.Sprintf("%s -> %s", c.connection.ParentId, c.connection.ChildId)
}

// IsStatic returns true if the connection is a static connection (i.e. it is not a switched connection.)
func (c *Connection) IsStatic() bool {
	// Right now, just check if the connection as an associated manger.
	return c.manager == nil
}

// ConnectionPath is a collection of connections between components.
type ConnectionPath []*Connection

// Activate iterates through all connections in the path and enables them.
func (c ConnectionPath) Activate(ctx context.Context) error {
	// Plug in devices starting at peripheral and working towards DUT.
	for i := len(c) - 1; i >= 0; i-- {
		con := c[i]
		if con.IsStatic() {
			continue
		}

		req := &passport.ConfigureSwitchPortRequest{
			State:    passport.SwitchPortState_SWITCH_PORT_ENABLED,
			SwitchId: con.connection.GetParentId(),
			PortId:   con.connection.GetParentPort(),
		}
		if _, err := con.manager.ConfigureSwitchPort(ctx, req); err != nil {
			return errors.Wrap(err, "failed to enable connection")
		}
	}
	return nil
}

// DisableAll iterates through all connections in the path and disables them.
func (c ConnectionPath) DisableAll(ctx context.Context) error {
	foundSwitch := false
	for _, con := range c {
		if con.IsStatic() {
			continue
		}

		req := &passport.ConfigureSwitchPortRequest{
			State:    passport.SwitchPortState_SWITCH_PORT_DISABLED,
			SwitchId: con.connection.GetParentId(),
		}
		if _, err := con.manager.ConfigureSwitchPort(ctx, req); err != nil {
			return errors.Wrap(err, "failed to enable connection")
		}
		foundSwitch = true
	}
	if !foundSwitch {
		return errors.New("failed to disable path, no switch connections found")
	}
	return nil
}

// DisableLast disables just the final connection in the path.
//
// This can be used to disconnect a monitor but not the dock since paths are always
// defined as starting with the DUT and ending with the leaf device.
func (c ConnectionPath) DisableLast(ctx context.Context) error {
	for i := len(c) - 1; i >= 0; i-- {
		con := c[i]
		if con.IsStatic() {
			continue
		}

		req := &passport.ConfigureSwitchPortRequest{
			State:    passport.SwitchPortState_SWITCH_PORT_DISABLED,
			SwitchId: con.connection.GetParentId(),
		}
		if _, err := con.manager.ConfigureSwitchPort(ctx, req); err != nil {
			return errors.Wrap(err, "failed to enable connection")
		}
		return nil
	}
	return errors.New("failed to disable path, no switch connections found")
}

// FlipLast "flips" the final connection in the path.
//
// This can be used to disconnect a monitor but not the dock since paths are always
// defined as starting with the DUT and ending with the leaf device.
func (c ConnectionPath) FlipLast(ctx context.Context) error {
	for i := len(c) - 1; i >= 0; i-- {
		con := c[i]
		if con.IsStatic() {
			continue
		}

		req := &passport.ConfigureSwitchPortRequest{
			State:    passport.SwitchPortState_SWITCH_PORT_FLIP,
			SwitchId: con.connection.GetParentId(),
		}
		if _, err := con.manager.ConfigureSwitchPort(ctx, req); err != nil {
			return errors.Wrap(err, "failed to flip connection")
		}
		return nil
	}
	return errors.New("failed to flip path, no switch connections found")
}
