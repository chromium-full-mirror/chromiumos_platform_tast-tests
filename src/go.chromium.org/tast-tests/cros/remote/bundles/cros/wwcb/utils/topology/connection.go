// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package topology contains tools to interact with PASIT topology components.
package topology

import (
	"context"
	"fmt"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
	"go.chromium.org/tast/core/errors"
)

// ConnectionManager is an interface that handles enabling/disabling connections between components
// for devices that support it.
type ConnectionManager interface {
	// Configures the enable state of the connection.
	EnabledState(context.Context, bool) error
}

// Connection is a directed connection between two components in the PASIT topology.
// It wraps switches (fixtures) to abstract away the switch "activation" implementations.
type Connection struct {
	connection *labapi.PasitHost_Connection
	childID    string
	manager    ConnectionManager
}

// newConnection creates a connection between to components.
func newConnection(connection *labapi.PasitHost_Connection, reverse bool, manager ConnectionManager) *Connection {
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
	for _, con := range c {
		if con.IsStatic() {
			continue
		}

		if err := con.manager.EnabledState(ctx, true); err != nil {
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

		if err := con.manager.EnabledState(ctx, true); err != nil {
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
		if err := con.manager.EnabledState(ctx, false); err != nil {
			return errors.Wrap(err, "failed to enable connection")
		}
		return nil
	}
	return errors.New("failed to disable path, no switch connections found")
}
