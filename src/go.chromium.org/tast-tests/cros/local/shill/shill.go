// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package shill provides D-Bus wrappers and utilities for shill service.
package shill

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
)

const (
	// dbus constants
	dbusService = "org.chromium.flimflam"
	// JobName is the name of the shill process
	JobName = "shill"
	// ResetShillTimeout specifies the timeout value for a shill restart.
	ResetShillTimeout = 30 * time.Second
)

// ResetShill does a best effort to remove any modifications to the shill
// configuration and resetting it in a known default state.
func ResetShill(ctx context.Context) []error {
	var errs []error
	if err := upstart.StopJob(ctx, JobName); err != nil {
		errs = append(errs, errors.Wrap(err, "failed to stop shill"))
	}
	if err := os.Remove(shillconst.DefaultProfilePath); err != nil && !os.IsNotExist(err) {
		errs = append(errs, errors.Wrap(err, "failed to remove default profile"))
	}
	if err := upstart.RestartJob(ctx, JobName); err != nil {
		// No more can be done if shill doesn't start
		return append(errs, errors.Wrap(err, "failed to restart shill"))
	}
	manager, err := NewManager(ctx)
	if err != nil {
		// No more can be done if a manger interface cannot be created
		return append(errs, errors.Wrap(err, "failed to create new shill manager"))
	}
	if err = manager.PopAllUserProfiles(ctx); err != nil {
		errs = append(errs, errors.Wrap(err, "failed to pop all user profiles"))
	}

	// Wait until a service is connected.
	expectProps := map[string]interface{}{
		shillconst.ServicePropertyIsConnected: true,
	}
	if _, err := manager.WaitForServiceProperties(ctx, expectProps, ResetShillTimeout); err != nil {
		errs = append(errs, errors.Wrap(err, "failed to wait for connected service"))
	}

	return errs
}
