// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package iperf

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/network/cmd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// Client represents an Iperf client host.
type Client interface {
	// Start launches the Iperf client and returns the result.
	Start(ctx context.Context, config *Config) (*Result, error)
	Stop(ctx context.Context) error
}

// RemoteClient represents a remote host to launch an Iperf client on.
type RemoteClient struct {
	conn      *ssh.Conn
	iperfPath string
	fw        *firewallHelper
	config    *Config
}

// NewRemoteClient creates a RemoteClient from an existing ssh connection.
func NewRemoteClient(ctx context.Context, conn *ssh.Conn) (*RemoteClient, error) {
	return &RemoteClient{
		conn: conn,
		fw:   newFirewallHelper(conn),
	}, nil
}

// Start launches the Iperf client on the remote host and returns the result.
func (c *RemoteClient) Start(ctx context.Context, config *Config) (*Result, error) {
	args := getClientArguments(config)
	c.config = config

	iperfPath, err := cmd.FindCmdPath(ctx, c.conn, string(config.Version))
	if err != nil {
		return nil, errors.Wrap(err, "failed to find iperf on host")
	}
	c.iperfPath = iperfPath
	iperfCommand := fmt.Sprintf("%s %s", iperfPath, strings.Join(args, " "))
	testing.ContextLogf(ctx, "Running iperf client for %d seconds", config.TestTime/time.Second)
	testing.ContextLogf(ctx, "iperf client invocation: %q", iperfCommand)

	clientCtx, cancel := context.WithTimeout(ctx, config.TestTime+commandTimeoutMargin)
	defer cancel()

	if err := c.fw.open(ctx, config.ServerIP); err != nil {
		return nil, errors.Wrap(err, "failed to configure client firewall")
	}

	output, err := c.conn.CommandContext(clientCtx, iperfPath, args...).CombinedOutput()
	if err != nil {
		return nil, errors.Wrap(err, "failed to run Iperf client")
	}
	defer c.Stop(ctx)

	result, err := newResultFromOutput(ctx, string(output), config)
	if err != nil {
		testing.ContextLog(ctx, "Failed to parse output: ", string(output))
		return nil, errors.Wrap(err, "failed to parse Iperf output")
	}

	return result, nil
}

// Close releases any additional resources held open by the client.
func (c *RemoteClient) Close(ctx context.Context) {
	c.Stop(ctx)
}

// Stop terminates any Iperf clients running on the remote machine.
func (c *RemoteClient) Stop(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, commandTimeoutMargin)
	defer cancel()

	var allErrors error
	if c.config == nil || c.config.Version == Version2 {
		if err := c.conn.CommandContext(ctx, "killall", "-q", "-9", c.iperfPath).Run(); err != nil && err.Error() != "Process exited with status 1" {
			allErrors = errors.Join(allErrors, errors.Wrap(err, "failed to stop iperf on client host"))
		}
	}

	if err := c.fw.close(ctx); err != nil {
		allErrors = errors.Join(allErrors, errors.Wrap(err, "failed to close firewall on server host"))
	}

	return allErrors
}

func getClientArguments(config *Config) []string {
	res := []string{
		"-c", config.ServerIP,
		"-B", config.ClientIP,
		"-b", strconv.FormatFloat(float64(config.MaxBandwidth), 'f', -1, 64),
		"-p", strconv.Itoa(config.Port),
		"-P", strconv.Itoa(config.PortCount),
		"-t", strconv.Itoa(int(config.TestTime / time.Second)),
	}

	if config.ClientWindowSize > 0 {
		res = append(res, "-w", strconv.Itoa(int(config.ClientWindowSize)))
	}

	if config.Protocol == ProtocolUDP {
		res = append(res, "-u")
	}

	if config.DatagramLength > 0 {
		res = append(res, "-l", strconv.Itoa(int(config.DatagramLength)))
	}

	switch config.Version {
	case Version2:
		res = append(res, "-y", "c")
		if config.Bidirectional {
			res = append(res, "-d")
		}
	case Version3:
		if config.Bidirectional {
			res = append(res, "--bidir")
		}
	}

	return res
}
