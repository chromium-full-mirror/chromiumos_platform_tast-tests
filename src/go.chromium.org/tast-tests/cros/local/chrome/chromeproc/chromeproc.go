// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package chromeproc provides utilities to find Chrome processes.
package chromeproc

import (
	"github.com/shirou/gopsutil/v3/process"

	"go.chromium.org/tast-tests/cros/local/chrome/ash/ashproc"
	"go.chromium.org/tast-tests/cros/local/chrome/internal/chromeproc"
)

// GetRootPID returns the PID of the root Chrome process.
// This corresponds to the browser process.
func GetRootPID() (int, error) {
	p, err := chromeproc.Root(ashproc.ExecPath)
	if err != nil {
		return -1, err
	}
	return int(p.Pid), nil
}

// GetPluginProcesses returns Chrome plugin processes.
func GetPluginProcesses() ([]*process.Process, error) {
	return chromeproc.PluginProcesses(ashproc.ExecPath)
}

// GetRendererProcesses returns Chrome renderer processes.
func GetRendererProcesses() ([]*process.Process, error) {
	return chromeproc.RendererProcesses(ashproc.ExecPath)
}

// GetGPUProcesses returns Chrome gpu-process processes.
func GetGPUProcesses() ([]*process.Process, error) {
	return chromeproc.GPUProcesses(ashproc.ExecPath)
}

// GetBrokerProcesses returns Chrome broker processes.
func GetBrokerProcesses() ([]*process.Process, error) {
	return chromeproc.BrokerProcesses(ashproc.ExecPath)
}

// GetUtilityProcesses returns Chrome utility processes.
func GetUtilityProcesses() ([]*process.Process, error) {
	return chromeproc.UtilityProcesses(ashproc.ExecPath)
}
