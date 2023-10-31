// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package vm provides set of util functions for base-vm.
package vm

import (
	"path/filepath"

	"github.com/shirou/gopsutil/v3/process"
	"go.chromium.org/tast-tests/cros/local/procutil"
	"go.chromium.org/tast/core/errors"
)

// SocketPath look-ups a crosvm socket path to communicate with ARCVM
func SocketPath() (string, error) {
	socketPath := ""
	var pathMismatchErr error
	_, err := procutil.FindAll(procutil.And(procutil.ByExe("/usr/bin/crosvm"), func(p *process.Process) bool {
		cmdline, err := p.CmdlineSlice()
		if err != nil {
			return false
		}
		for i, arg := range cmdline {
			// Although there are multiple ways to specify the
			// control socket, ARCVM currently always uses `--socket
			// PATH`. We could try to be more resiliant against
			// future changes, but parsing the cmdline like this
			// isn't a stable API to begin with. So just hardcode
			// things to handle the current behavior, and the test
			// can be updated if ARCVM changes.
			if arg == "--socket" && i+1 < len(cmdline) {
				socketArg := cmdline[i+1]
				if filepath.Base(socketArg) == "arcvm.sock" {
					if socketPath != "" && socketPath != socketArg {
						pathMismatchErr = errors.New("too many ARCVM sockets")
					} else {
						socketPath = socketArg
					}
					return true
				}
			}
		}
		return false
	}))
	if err != nil {
		return "", errors.Wrap(err, "failed to find ARCVM process")
	}
	return socketPath, pathMismatchErr
}
