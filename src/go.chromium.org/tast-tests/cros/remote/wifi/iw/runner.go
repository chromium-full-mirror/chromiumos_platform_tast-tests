// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package iw contains utility functions to wrap around the iw program.
package iw

import (
	"go.chromium.org/tast-tests/cros/common/wifi/iw"
	"go.chromium.org/tast-tests/cros/remote/network/cmd"
	"go.chromium.org/tast/core/ssh"
)

// Runner is an alias for common iw Runner but only for remote execution.
type Runner = iw.Runner

// NewRemoteRunner creates a iw runner for remote execution.
func NewRemoteRunner(host *ssh.Conn) *Runner {
	return iw.NewRunner(&cmd.RemoteCmdRunner{Host: host})
}
