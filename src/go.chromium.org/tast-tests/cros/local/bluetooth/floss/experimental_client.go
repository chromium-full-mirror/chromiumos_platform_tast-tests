// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package floss

import (
	"context"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
)

// ExperimentalClient handles method calls to the floss experimental interface.
type ExperimentalClient struct {
	dbus *dbusutil.DBusObject
}

// NewExperimentalClient creates a new floss ExperimentalClient from the passed D-Bus
// object path.
func NewExperimentalClient(ctx context.Context, path dbus.ObjectPath) (*ExperimentalClient, error) {
	obj, err := NewFlossManagerDBusObject(ctx, flossExperimentalInterface, path)
	if err != nil {
		return nil, err
	}
	return &ExperimentalClient{dbus: obj}, nil
}

// CreateExperimentalClients creates a ExperimentalClient for all floss managers in the system.
func CreateExperimentalClients(ctx context.Context) ([]*ExperimentalClient, error) {
	paths, err := collectExistingFlossManagerServiceObjectPaths(ctx, flossExperimentalInterface)
	if err != nil {
		return nil, err
	}
	experimentalClients := make([]*ExperimentalClient, len(paths))
	for i, path := range paths {
		experimentalClient, err := NewExperimentalClient(ctx, path)
		if err != nil {
			return nil, err
		}
		experimentalClients[i] = experimentalClient
	}
	return experimentalClients, nil
}

// DefaultExperimentalClient returns an initialized ExperimentalClient for the default
// Manager object.
func DefaultExperimentalClient(ctx context.Context) (*ExperimentalClient, error) {
	clients, err := CreateExperimentalClients(ctx)
	if err != nil {
		return nil, err
	}
	if len(clients) != 1 {
		return nil, errors.Errorf("expected exactly 1 floss manager to exist, but found %d", len(clients))
	}
	return clients[0], nil
}

// SetLLPrivacy sets the state of LL privacy.
func (c *ExperimentalClient) SetLLPrivacy(ctx context.Context, enabled bool) error {
	call := c.dbus.Call(ctx, "SetLLPrivacy", enabled)
	if call.Err != nil {
		return errors.Wrap(call.Err, "failed to call SetLLPrivacy")
	}
	return nil
}
