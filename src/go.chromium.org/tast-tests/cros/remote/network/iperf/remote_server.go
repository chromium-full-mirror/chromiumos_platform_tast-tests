// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package iperf

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/network/cmd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// Server represents an Iperf server host.
type Server interface {
	// Start launches a new Iperf server instance.
	Start(ctx context.Context, config *Config) error
	// Stop terminates the Iperf server instance.
	Stop(ctx context.Context) error
	// FetchResult fetches the most recent result from the Iperf server after a run has been completed.
	FetchResult(ctx context.Context, config *Config) (*Result, error)
}

// RemoteServer represents a remote host to launch an iperf server on.
type RemoteServer struct {
	useMiniJail  bool
	conn         *ssh.Conn
	iperfPath    string
	minijailPath string
	fw           *firewallHelper
	stdout       *bytes.Buffer
}

// NewRemoteServer creates an SSHServerHost from an existing ssh connection.
func NewRemoteServer(ctx context.Context, conn *ssh.Conn) (*RemoteServer, error) {
	iperfPath, err := cmd.FindCmdPath(ctx, conn, "iperf")
	if err != nil {
		return nil, errors.Wrap(err, "failed to find iperf on host")
	}

	// minijail isn't available on openwrt, but is required on gale
	// If minijail0 is on ${PATH} on any host, use it. If not,
	// use the original invocation.
	useMiniJail := true
	minijailPath, err := cmd.FindCmdPath(ctx, conn, "minijail0")
	if err != nil {
		useMiniJail = false
		testing.ContextLog(ctx, "minijail0 not present on host, proceeding without minijail")
	} else {
		testing.ContextLog(ctx, "minijail0 found on host, proceeding to use minijail")
	}

	return &RemoteServer{
		useMiniJail:  useMiniJail,
		conn:         conn,
		iperfPath:    iperfPath,
		minijailPath: minijailPath,
		fw:           newFirewallHelper(conn),
	}, nil
}

// Start launches a new Iperf server instance on the remote machine.
func (c *RemoteServer) Start(ctx context.Context, config *Config) error {
	args := getServerArguments(config)
	iperfCommand := fmt.Sprintf("%s %s %s", c.minijailPath, c.iperfPath, strings.Join(args, " "))
	testing.ContextLog(ctx, "Starting iperf server")
	testing.ContextLogf(ctx, "iperf server invocation: %s", iperfCommand)

	if err := c.fw.open(ctx, config.ClientIP); err != nil {
		return errors.Wrap(err, "failed to configure server firewall")
	}

	var cmd *ssh.Cmd
	if c.useMiniJail {
		args = append([]string{c.iperfPath}, args...)
		cmd = c.conn.CommandContext(ctx, c.minijailPath, args...)
	} else {
		cmd = c.conn.CommandContext(ctx, c.iperfPath, args...)
	}

	c.stdout = new(bytes.Buffer)
	cmd.Stdout = c.stdout

	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to start Iperf server")
	}

	// Try to verify that server has been started and that port is in use (b/313675621).
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		cmd := fmt.Sprintf("netstat -l | grep :%d", config.Port)
		if out, err := c.conn.CommandContext(ctx, "sh", "-c", cmd).Output(); err != nil {
			return errors.Wrap(err, "failed to find port")
		} else if string(out) == "" {
			return errors.Errorf("port %d is not in use", config.Port)
		}
		return nil
	}, &testing.PollOptions{
		Timeout: 3 * time.Second,
	}); err != nil {
		return errors.Wrap(err, "failed to verify that iperf server is started")
	}

	go func() {
		if err := cmd.Wait(testexec.DumpLogOnError); err != nil {
			testing.ContextLogf(ctx, "Iperf server stopped unexpectedly %v: %q", err, stderr.String())
		}
	}()

	return nil
}

// Close releases any additional resources held open by the server.
func (c *RemoteServer) Close(ctx context.Context) {
	if err := c.Stop(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to stop Iperf, err: ", err)
	}
}

// Stop terminates any Iperf servers running on the remote machine.
func (c *RemoteServer) Stop(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, commandTimeoutMargin)
	defer cancel()

	var allErrors error
	if err := c.conn.CommandContext(ctx, "killall", "-q", "-9", c.iperfPath).Run(); err != nil && err.Error() != "Process exited with status 1" {
		allErrors = errors.Wrapf(allErrors, "failed to stop iperf on server host: %v", err) // NOLINT
	}

	if err := c.fw.close(ctx); err != nil {
		allErrors = errors.Wrapf(allErrors, "failed to close firewall on server host: %v", err) //NOLINT
	}

	return allErrors
}

// FetchResult fetches the most recent Iperf results from the remote machine.
func (c *RemoteServer) FetchResult(ctx context.Context, config *Config) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeoutMargin)
	defer cancel()

	if c.stdout == nil {
		return nil, errors.New("failed to fetch results, no server sessions found")
	}

	result, err := newResultFromOutput(ctx, c.stdout.String(), config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse server output")
	}

	return result, nil
}

func getServerArguments(config *Config) []string {
	res := []string{
		"-s",
		"-y", "c",
		"-B", config.ServerIP,
		"-p", strconv.Itoa(config.Port),
	}

	if config.ServerWindowSize > 0 {
		res = append(res, "-w", strconv.Itoa(int(config.ServerWindowSize)))
	}

	if config.Protocol == ProtocolUDP {
		res = append(res, "-u")
	}

	return res
}
