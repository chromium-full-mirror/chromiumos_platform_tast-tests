// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package firewall wraps basic iptables call to control
// filtering of incoming/outgoing traffic.
package firewall

import (
	"go.chromium.org/tast-tests/cros/common/network/firewall"
	"go.chromium.org/tast-tests/cros/remote/network/cmd"
	"go.chromium.org/tast/core/ssh"
)

// Runner is an alias for common firewall Runner but only for remote execution.
type Runner = firewall.Runner

// NewRemoteRunner creates a firewall runner for remote execution.
func NewRemoteRunner(host *ssh.Conn) *Runner {
	return firewall.NewRunner(&cmd.RemoteCmdRunner{Host: host})
}
