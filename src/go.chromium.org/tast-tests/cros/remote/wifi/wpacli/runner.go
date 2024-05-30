// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wpacli contains utility functions to wrap around the wpacli program.
package wpacli

import (
	"go.chromium.org/tast-tests/cros/common/wifi/wpacli"
	"go.chromium.org/tast-tests/cros/remote/network/cmd"
	"go.chromium.org/tast/core/ssh"
)

// Runner is an alias for common wpacli Runner but only for remote execution.
type Runner = wpacli.Runner

// WPAMonitor is an alias for common wpacli WPAMonitor but only for remote execution.
type WPAMonitor = wpacli.WPAMonitor

// NewRemoteRunner creates a wpacli runner for remote execution.
func NewRemoteRunner(host *ssh.Conn) *Runner {
	return wpacli.NewRunner(&cmd.RemoteCmdRunner{Host: host})
}

// NewRemoteRunnerOnIface creates a wpacli runner for remote execution for the particular interface.
func NewRemoteRunnerOnIface(host *ssh.Conn, iface string) *Runner {
	return wpacli.NewRunnerOnIface(&cmd.RemoteCmdRunner{Host: host}, iface)
}

// NewRemoteWPAMonitorOnIface creates a wpacli monitor for remote execution for the particular interface.
func NewRemoteWPAMonitorOnIface(host *ssh.Conn, iface string) *WPAMonitor {
	return wpacli.NewWPAMonitorOnIface(&cmd.RemoteCmdRunner{Host: host}, iface)
}
