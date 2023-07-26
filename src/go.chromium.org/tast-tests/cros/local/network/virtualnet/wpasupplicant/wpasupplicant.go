// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wpasupplicant provides the utils to run the wpa_supplicant server
// inside a virtualnet.Env.
package wpasupplicant

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/env"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	cmdPath  = "/usr/sbin/wpa_supplicant"
	confPath = "/tmp/wpa_supplicant.conf"
	logPath  = "/tmp/wpa_supplicant.log"
	ctrlPath = "/run/wpa_supplicant_sta"
)

type wpasupplicant struct {
	iface      string
	ssid       string
	passphrase string
	env        *env.Env
	cmd        *testexec.Cmd
}

// New creates a new wpasupplicant object. The returned object can be passed to
// Env.StartServer(), its lifetime will be managed by the Env object.
func New(iface, ssid, passphrase string) *wpasupplicant {
	return &wpasupplicant{
		iface:      iface,
		ssid:       ssid,
		passphrase: passphrase,
	}
}

// Start starts the wpa_supplicant process, and wait until the WiFi is connected.
func (d *wpasupplicant) Start(ctx context.Context, env *env.Env) error {
	d.env = env

	confTemplate := `
network={
	ssid="%s"
	psk="%s"
}`
	conf := fmt.Sprintf(confTemplate, d.ssid, d.passphrase)

	if err := os.WriteFile(d.env.ChrootPath(confPath), []byte(conf), 0644); err != nil {
		return errors.Wrap(err, "failed to write wpa_supplicant config into file")
	}

	// Start the command.
	cmd := []string{
		cmdPath,
		"-i", d.iface,
		"-c", d.env.ChrootPath(confPath),
		"-O", d.env.ChrootPath(ctrlPath),
	}
	d.cmd = d.env.CreateCommandWithoutChroot(ctx, cmd...)
	output, err := d.cmd.Cmd.StdoutPipe()
	if err != nil {
		return errors.Wrap(err, "failed to get stdout of wpa_supplicant")
	}
	if err := d.cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to start wpa_supplicant daemon")
	}

	// Wait for connected.
	logFile, err := os.Create(d.env.ChrootPath(logPath))
	if err != nil {
		return errors.Wrap(err, "failed to open log path: "+d.env.ChrootPath(logPath))
	}
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		logFile.WriteString(scanner.Text() + "\n")
		if strings.Contains(scanner.Text(), "CTRL-EVENT-CONNECTED") {
			break
		}
	}

	testing.ContextLog(ctx, "WiFi is connected")
	return nil
}

// Stop stops the wpa_supplicant process.
func (d *wpasupplicant) Stop(ctx context.Context) error {
	if d.cmd == nil || d.cmd.Process == nil {
		return nil
	}
	if err := d.cmd.Kill(); err != nil {
		return errors.Wrap(err, "failed to kill wpa_supplicant processs")
	}
	d.cmd.Wait()
	d.cmd = nil
	return nil
}

// WriteLogs writes logs into |f|.
func (d *wpasupplicant) WriteLogs(ctx context.Context, f *os.File) error {
	return d.env.ReadAndWriteLogIfExists(d.env.ChrootPath(logPath), f)
}
