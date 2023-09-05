// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	cui "go.chromium.org/tast-tests/cros/local/crostini/ui"
	"go.chromium.org/tast-tests/cros/local/terminalapp"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
)

// InstallCrostini installs crostini for the current user.
func InstallCrostini(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, iOptions *cui.InstallationOptions) (*vm.Container, *terminalapp.TerminalApp, error) {
	// Install Crostini.
	if _, err := cui.InstallCrostini(ctx, tconn, cr, iOptions); err != nil {
		return nil, nil, errors.Wrapf(err, "failed to install Crostini for user %s", iOptions.UserName)
	}

	// TerminalApp always automatically launches after the installation.
	// Close it before proceeding to ensure a clean env.
	if err := apps.Close(ctx, tconn, apps.Terminal.ID); err != nil {
		return nil, nil, errors.Wrap(err, "failed to close Terminal app after installing Linux")
	}

	terminalapp, err := terminalapp.Launch(ctx, tconn)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to launch terminal after installing Crostini for user %s", iOptions.UserName)
	}

	// Get the container.
	cont, err := vm.DefaultContainer(ctx, iOptions.UserName)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to connect to the container after installing Crostini")
	}
	return cont, terminalapp, nil
}
