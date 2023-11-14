// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/chameleon"
	"go.chromium.org/tast-tests/cros/remote/log"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	cryptossh "golang.org/x/crypto/ssh"
)

const (
	btpeerVersionLogFilePath    = "/var/log/chameleon_commits"
	btpeerChameleondLogFilePath = "/var/log/chameleond"
)

// BtpeerClient manages connections and provides clients to btpeers.
type BtpeerClient struct {
	// Device-specific.
	hostname           string
	sshConn            *ssh.Conn
	sshOptions         *ssh.Options
	systemLogCollector log.Collector

	// Chameleond-specific.
	chameleondClient        chameleon.Chameleond
	chameleondPortForwarder *ssh.Forwarder
	chameleondLogCollector  log.Collector
}

func newBtpeerClient(hostname string, sshOptions *ssh.Options) (*BtpeerClient, error) {
	if err := ssh.ParseTarget(hostname, sshOptions); err != nil {
		return nil, errors.Wrapf(err, "failed to parse ssh target btpeer hostname %q", hostname)
	}
	return &BtpeerClient{
		hostname:   hostname,
		sshOptions: sshOptions,
	}, nil
}

// Connect will connect to the btpeer over ssh, configure a chameleond client,
// and log the chameleond version. These connections and client may be usable
// beyond the expiration of ctx.
func (c *BtpeerClient) Connect(ctx context.Context) error {
	if c.IsConnected() {
		return nil
	}
	// Connect ssh.
	testing.ContextLogf(ctx, "Connecting to btpeer host %q over ssh", c.hostname)
	if err := c.connectSSH(ctx); err != nil {
		return errors.Wrapf(err, "failed to Connect to btpeer host %q over ssh", c.hostname)
	}
	testing.ContextLogf(ctx, "Successfully connected to btpeer host %q over ssh", c.hostname)
	// Connect chameleond.
	testing.ContextLogf(ctx, "Connecting to chameleond on btpeer host %q", c.hostname)
	if err := c.connectChameleond(ctx); err != nil {
		testing.ContextLogf(ctx, "WARNING: Failed to Connect to chameleond on btpeer %q in first attempt: %v", c.hostname, err)
		testing.ContextLogf(ctx, "Rebooting btpeer host %q and retrying chameleond connection", c.hostname)
		if err := c.Reboot(ctx); err != nil {
			return errors.Wrapf(err, "failed to Reboot btpeer %q after first chameleond connection failure", c.hostname)
		}
		// Try chameleond again with a short poll as ssh may come up before
		// chameleond does.
		testing.ContextLogf(ctx, "Connecting to chameleond on btpeer host %q after successful Reboot", c.hostname)
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			return c.connectChameleond(ctx)
		}, &testing.PollOptions{
			Interval: 500 * time.Millisecond,
			Timeout:  10 * time.Second,
		}); err != nil {
			return errors.Wrapf(err, "failed to Connect to chameleond on btpeer host %q after successful Reboot", c.hostname)
		}
	}
	testing.ContextLogf(ctx, "Successfully connected to chameleond on btpeer host %q", c.hostname)
	// Log chameleond version.
	testing.ContextLogf(ctx, "Fetching chameleond version information from btpeer %q", c.hostname)
	if chameleondUpdatedAt, chameleondLastCommit, err := c.fetchChameleondVersion(ctx); err != nil {
		testing.ContextLogf(ctx, "WARNING: Failed to fetch chameleond version information from btpeer %q: %v", c.hostname, err)
	} else {
		testing.ContextLogf(ctx, "Chameleond on btpeer host %q was last updated at %q to commit %q", c.hostname, chameleondUpdatedAt, chameleondLastCommit)
	}
	return nil
}

// connectSSH will ensure the btpeer device has a live ssh connection. Existing
// connections will be verified. A new ssh connection is established if
// verification fails or if no connection exists.
func (c *BtpeerClient) connectSSH(ctx context.Context) error {
	// Reuse existing connection if alive.
	if c.sshConn != nil {
		if err := c.sshConn.Ping(ctx, 10*time.Second); err != nil {
			// Existing connection is dead.
			c.disconnectSSH(ctx)
		} else {
			// Existing connection is live.
			return nil
		}
	}
	// Establish new ssh connection.
	sshConn, err := ssh.New(ctx, c.sshOptions)
	if err != nil {
		return errors.Wrapf(err, "failed to Connect to btpeer hostname %q over ssh", c.hostname)
	}
	c.sshConn = sshConn
	return nil
}

func (c *BtpeerClient) disconnectSSH(ctx context.Context) {
	if c.sshConn != nil {
		if err := c.sshConn.Close(ctx); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to close ssh connection to btpeer %q: %v", c.hostname, err)
		}
		c.sshConn = nil
	}
}

// Reboot will trigger a Reboot of the btpeer device over ssh then attempt to
// reestablish a new ssh connection to device until it is successful or times
// out.
//
// Anything that relies upon an ssh connection must be reconnected afterwards,
// as the previous ssh connection will have been severed. This may be done by
// calling Connect.
func (c *BtpeerClient) Reboot(ctx context.Context) error {
	if c.sshConn == nil {
		return errors.Errorf("failed to Reboot btpeer %d: no active ssh connection to device", c.hostname)
	}
	// Reboot, ignoring the ssh error that occurs due to severed connection.
	_ = c.sshConn.CommandContext(ctx, "Reboot").Run()
	_ = c.sshConn.Close(ctx)
	c.sshConn = nil
	// Try to reconnect via ssh until successful.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return c.connectSSH(ctx)
	}, &testing.PollOptions{
		Interval: 1 * time.Second,
		Timeout:  1 * time.Minute,
	}); err != nil {
		return errors.Wrapf(err, "failed to reconnect to btpeer hostname %q over ssh after Reboot", c.hostname)
	}
	return nil
}

// connectChameleond will create a new ssh tunnel to the chameleond port on the
// btpeer device and create a new chameleond client connected through that
// tunnel. Requires an active ssh connection to the device. Any existing
// ssh tunnel or chameleond client is closed and replaced.
func (c *BtpeerClient) connectChameleond(ctx context.Context) error {
	c.disconnectChameleond(ctx)
	if c.sshConn == nil {
		return errors.Errorf("failed to Connect to chameleond for btpeer %d: no active ssh connection to device", c.hostname)
	}
	// Create an ssh tunnel to the chameleond port.
	onFwdError := func(err error) {
		testing.ContextLogf(ctx, "ssh forwarding error for btpeer host %q: %v", c.hostname, err)
		c.chameleondClient = nil
	}
	chameleondPortForwarder, err := c.sshConn.ForwardLocalToRemote("tcp", "localhost:0", "localhost:9992", onFwdError)
	if err != nil {
		return errors.Wrapf(err, "failed to port forward chameleond port for btpeer host %q", c.hostname)
	}
	c.chameleondPortForwarder = chameleondPortForwarder
	// Create a new chameleond client which uses forward chameleond port.
	chameleondClient, err := chameleon.NewChameleond(ctx, c.chameleondPortForwarder.ListenAddr().String())
	if err != nil {
		return errors.Wrapf(err, "failed to Connect to chameleond on btpeer host %q through forward chameleond port at %q", c.hostname, chameleondPortForwarder.ListenAddr().String())
	}
	c.chameleondClient = chameleondClient
	return nil
}

// fetchChameleondVersion parses and returns the chameleond version from the
// chameleond version log file on the btpeer device.
func (c *BtpeerClient) fetchChameleondVersion(ctx context.Context) (string, string, error) {
	if c.sshConn == nil {
		return "", "", errors.Errorf("failed to fetch chameleond version from btpeer host %q: no active ssh connection to device", c.hostname)
	}
	// Attempt to fetch the chameleond version (not supported on old versions).
	var chameleondLastCommit, chameleondUpdatedAt string
	btpeerVersionLogFileExists, err := c.remoteFileExists(ctx, c.sshConn, btpeerVersionLogFilePath)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to check for chameleond log file %q on btpeer hostname %q", btpeerChameleondLogFilePath, c.hostname)
	}
	if btpeerVersionLogFileExists {
		lastLogLine, err := c.sshConn.CommandContext(ctx, "tail", "-1", btpeerVersionLogFilePath).Output()
		if err == nil {
			lastLogLineParts := strings.Split(strings.TrimSpace(string(lastLogLine)), " ")
			if len(lastLogLineParts) == 2 {
				chameleondLastCommit = lastLogLineParts[0]
				chameleondUpdatedAt = lastLogLineParts[1]
			}
		}
	}
	if chameleondLastCommit == "" {
		chameleondLastCommit = "unknown"
	}
	if chameleondUpdatedAt == "" {
		chameleondUpdatedAt = "unknown"
	}
	return chameleondLastCommit, chameleondUpdatedAt, nil
}

func (c *BtpeerClient) disconnectChameleond(ctx context.Context) {
	// Close clients.
	c.chameleondClient = nil
	// Close ssh tunnel.
	if c.chameleondPortForwarder != nil {
		if err := c.chameleondPortForwarder.Close(); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to shut down forwarded chameleond port tunnel for btpeer %q: %v", c.hostname, err)
		}
	}
}

// StartLogCollection starts the collection of logs from the btpeer. The log
// collection will continue until StopLogCollection or Disconnect is called or
// until the ctx expires.
func (c *BtpeerClient) StartLogCollection(ctx context.Context) error {
	c.StopLogCollection(ctx)
	if c.sshConn == nil {
		return errors.Errorf("failed to start log collection on btpeer host %q: no active ssh connection to device", c.hostname)
	}
	// Start collecting system logs.
	systemLogCollector, err := log.StartJournalctlCollector(ctx, c.sshConn, "--output", "short-full")
	if err != nil {
		return errors.Wrapf(err, "failed to start collecting system logs on btpeer host %q", c.hostname)
	}
	c.systemLogCollector = systemLogCollector
	// Start collecting chameleond logs.
	hasChameleondLogFile, err := c.remoteFileExists(ctx, c.sshConn, btpeerChameleondLogFilePath)
	if err != nil {
		return errors.Wrapf(err, "failed to check for chameleond log file %q on btpeer host %q", btpeerChameleondLogFilePath, c.hostname)
	}
	if hasChameleondLogFile {
		chameleondLogCollector, err := log.StartTailCollector(ctx, c.sshConn, btpeerChameleondLogFilePath, true)
		if err != nil {
			return errors.Wrapf(err, "failed to start collecting chameleond logs on btpeer host %q", c.hostname)
		}
		c.chameleondLogCollector = chameleondLogCollector
	}
	return nil
}

// StopLogCollection stops any ongoing log collection started by StartLogCollection.
func (c *BtpeerClient) StopLogCollection(ctx context.Context) {
	if c.systemLogCollector != nil {
		if err := c.systemLogCollector.Close(); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to close system log collector for btpeer %q: %v", c.hostname, err)
		}
		c.systemLogCollector = nil
	}
	if c.chameleondLogCollector != nil {
		if err := c.chameleondLogCollector.Close(); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to close chameleond log collector for btpeer %q: %v", c.hostname, err)
		}
		c.chameleondLogCollector = nil
	}
}

// Disconnect will close log collectors, clients, and ssh connections to the
// btpeer device. Connect must be called in order to use the btpeer again.
func (c *BtpeerClient) Disconnect(ctx context.Context) {
	c.StopLogCollection(ctx)
	c.disconnectChameleond(ctx)
	c.disconnectSSH(ctx)
}

// Hostname returns the hostname of the btpeer device.
func (c *BtpeerClient) Hostname() string {
	return c.hostname
}

// ChameleondClient returns a chameleond client for this btpeer device.
//
// Note: All chameleond method calls are routed through an ssh tunnel to the
// chameleond service running on the btpeer device.
func (c *BtpeerClient) ChameleondClient() chameleon.Chameleond {
	if c.chameleondClient == nil {
		// Panic rather than throw an error to allow for ease of use, as this is not
		// meant to happen during expected usage in tests.
		panic(fmt.Sprintf("no open chameleond client available for btpeer %q", c.hostname))
	}
	return c.chameleondClient
}

// IsConnected returns true if there is an existing ssh connection and usable
// chameleond client.
func (c *BtpeerClient) IsConnected() bool {
	return c.sshConn != nil && c.chameleondClient != nil
}

// Reset resets the btpeer to return it to its normal state and clear any
// changes any test may have made to them.
func (c *BtpeerClient) Reset(ctx context.Context) error {
	if !c.IsConnected() {
		return errors.Errorf("failed to reset btpeer %q: not connected to device", c.hostname)
	}
	testing.ContextLogf(ctx, "Resetting btpeer %q", c.hostname)
	// Reset the base chameleond service state.
	if err := c.chameleondClient.Reset(ctx); err != nil {
		return errors.Wrapf(err, "failed to reset chameleond on btpeer %q", c.hostname)
	}
	// Reset the bluetooth service state, through the keyboard device interface
	// since this method is not exposed at a higher level.
	if err := c.chameleondClient.BluetoothKeyboardDevice().ResetStack(ctx, ""); err != nil {
		return errors.Wrapf(err, "failed to reset bluetooth stack on btpeer %q", c.hostname)
	}
	testing.ContextLogf(ctx, "Successfully reset btpeer %q", c.hostname)
	return nil
}

// DumpLogs dumps log collector buffers to log files with the given dump context.
// The log files are saved in the current directory context (e.g. fixture log
// dir or test log dir, depending on when this is run) and in subdirectories
// for the btpeer host and log type.
//
// This can be called repeatedly to dump the log buffers after different periods
// to make it easier to see logs that happen during different contexts. The
// dumpContext should be a name that reflects the current log period (e.g.
// "SetUp", "Reset", "TearDown", "AfterSomeOtherNotableEvent", etc.)
//
// Generated log files will look like this:
// <context_dir>/
//
//	    btpeer_logs/
//	        <btpeer_hostname>/
//		           chameleond/
//		               <dump_timestamp>_<dumpContext1>.log
//		               <dump_timestamp>_<dumpContext2>.log
//		               ...
//		           system/
//		               <dump_timestamp>_<dumpContext1>.log
//		               <dump_timestamp>_<dumpContext2>.log
//		               ...
func (c *BtpeerClient) DumpLogs(ctx context.Context, dumpContext string) error {
	baseLogDir := filepath.Join("btpeer_logs", c.hostname)
	systemLogDir := filepath.Join(baseLogDir, "system")
	chameleondLogDir := filepath.Join(baseLogDir, "chameleond")
	if c.systemLogCollector != nil {
		testing.ContextLogf(ctx, "Dumping collected system logs from btpeer %q for %q", c.hostname, dumpContext)
		if err := log.DumpCollectedLogsToFile(ctx, c.systemLogCollector, systemLogDir, dumpContext); err != nil {
			return errors.Wrapf(err, "failed to dump collected btpeer system logs from btpeer %q", c.hostname)
		}
	}
	if c.chameleondLogCollector != nil {
		testing.ContextLogf(ctx, "Dumping collected chameleond logs from btpeer %q for %q", c.hostname, dumpContext)
		if err := log.DumpCollectedLogsToFile(ctx, c.chameleondLogCollector, chameleondLogDir, dumpContext); err != nil {
			return errors.Wrapf(err, "failed to dump collected btpeer chameleond logs from btpeer %q", c.hostname)
		}
	}
	return nil
}

// remoteFileExists runs the `test -f <path>` command using the provided ssh
// connection to verify file existence. Returns true if the test passes and
// false if the test fails. A non-nil error is returned if the command fails
// to run as expected.
func (c *BtpeerClient) remoteFileExists(ctx context.Context, sshConn *ssh.Conn, path string) (bool, error) {
	if err := sshConn.CommandContext(ctx, "test", "-f", path).Run(); err != nil {
		exitErr, ok := err.(*cryptossh.ExitError)
		if !ok || exitErr.ExitStatus() != 1 {
			return false, errors.Wrapf(err, "failed to run 'test -f %q' on remote host", path)
		}
		return false, nil
	}
	return true, nil
}
