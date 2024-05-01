// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
	"go.chromium.org/chromiumos/config/go/test/lab/api/btpeerd"
	"go.chromium.org/tast-tests/cros/common/chameleon"
	"go.chromium.org/tast-tests/cros/remote/fileutils"
	"go.chromium.org/tast-tests/cros/remote/log"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	cryptossh "golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	// btpeerChameleondVersionLegacyLogFilePath is the path to the log file with
	// installed chameleond commits for btpeers configured via chameleond bundles,
	// not the newer image-based configuration with btpeerd.
	btpeerChameleondVersionLegacyLogFilePath = "/var/log/chameleon_commits"
	btpeerChameleondLogFilePath              = "/var/log/chameleond"
	btpeerChameleondPort                     = 9992
	btpeerBtpeerdPort                        = 8100

	// btpeerImageBuildInfoFilePath is the path to the build info file present
	// only on btpeers with custom ChromeOS images built with pi-gen-btpeer.
	btpeerImageBuildInfoFilePath = "/etc/chromiumos/raspios_cros_btpeer_build_info.json"
)

// BtpeerClient manages connections and provides clients to btpeers.
type BtpeerClient struct {
	// Device-specific.
	registrationID     int
	hostname           string
	sshConn            *ssh.Conn
	sshOptions         *ssh.Options
	systemLogCollector log.Collector
	hasCustomImage     bool

	// Chameleond-specific.
	chameleondClient        chameleon.Chameleond
	chameleondPortForwarder *ssh.Forwarder
	chameleondLogCollector  log.Collector

	// Btpeerd-specific.
	btpeerdClient         btpeerd.BtpeerManagementServiceClient
	btpeerdPortForwarder  *ssh.Forwarder
	btpeerdGRPCClientConn *grpc.ClientConn
}

func newBtpeerClient(hostname string, sshOptions *ssh.Options, registrationID int) (*BtpeerClient, error) {
	if err := ssh.ParseTarget(hostname, sshOptions); err != nil {
		return nil, errors.Wrapf(err, "failed to parse ssh target btpeer hostname %q", hostname)
	}
	return &BtpeerClient{
		hostname:       hostname,
		sshOptions:     sshOptions,
		registrationID: registrationID,
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
	testing.ContextLogf(ctx, "Connecting to %s over ssh", c)
	if err := c.connectSSH(ctx); err != nil {
		return errors.Wrapf(err, "failed to connect to %s over ssh", c)
	}
	testing.ContextLogf(ctx, "Successfully connected to %s over ssh", c)

	// Identify image type.
	hasBuildInfoFile, err := fileutils.HostFileExists(ctx, c.sshConn, btpeerImageBuildInfoFilePath)
	if err != nil {
		return errors.Wrapf(err, "failed to check for existence of file %q on %s", btpeerImageBuildInfoFilePath, c)
	}
	c.hasCustomImage = hasBuildInfoFile

	// Log device info and prepare connection step.
	var connectServicesFn func(ctx context.Context) error
	if c.hasCustomImage {
		testing.ContextLogf(ctx, "Btpeer %s has a custom ChromeOS Raspberry Pi image installed, will connect to both btpeerd and chameleond", c)
		// Log build info.
		testing.ContextLogf(ctx, "Fetching image build info from %s", c)
		buildInfoFileContents, err := c.sshConn.CommandContext(ctx, "cat", btpeerImageBuildInfoFilePath).Output()
		if err != nil {
			return errors.Wrapf(err, "failed to read contents of file %q on %s", btpeerImageBuildInfoFilePath, c)
		}
		buildInfo := &labapi.RaspiosCrosBtpeerImageBuildInfo{}
		if err := protojson.Unmarshal(buildInfoFileContents, buildInfo); err != nil {
			return errors.Wrapf(err, "failed to unmarshal contents of file %q to a RaspiosCrosBtpeerImageBuildInfo on %s", btpeerImageBuildInfoFilePath, c)
		}
		buildInfoJSON, err := protojson.Marshal(buildInfo)
		if err != nil {
			return errors.Wrapf(err, "failed to marshall RaspiosCrosBtpeerImageBuildInfo from %q to JSON for %s", btpeerImageBuildInfoFilePath, c)
		}
		testing.ContextLogf(ctx, "Btpeer image build info for %s: RaspiosCrosBtpeerImageBuildInfo%s", c, string(buildInfoJSON))
		connectServicesFn = func(ctx context.Context) error {
			// Connect to btpeerd and chameleond.
			testing.ContextLogf(ctx, "Connecting to btpeerd on %s", c)
			if err := c.connectBtpeerd(ctx); err != nil {
				return errors.Wrapf(err, "failed to connect to btpeerd on %s", c)
			}
			testing.ContextLogf(ctx, "Successfully connected to btpeerd on %s", c)
			testing.ContextLogf(ctx, "Connecting to chameleond on %s", c)
			if err := c.connectChameleond(ctx); err != nil {
				return errors.Wrapf(err, "failed to connect to chameleond on %s", c)
			}
			testing.ContextLogf(ctx, "Successfully connected to chameleond on %s", c)
			return nil
		}
	} else {
		testing.ContextLogf(ctx, "Btpeer %s does not have a custom ChromeOS Raspberry Pi image installed, will connect to chameleond and not btpeerd", c)
		// Log chameleond version from non-custom image.
		testing.ContextLogf(ctx, "Fetching chameleond version information from %s", c)
		if chameleondUpdatedAt, chameleondLastCommit, err := c.fetchChameleondVersion(ctx); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to fetch chameleond version information from %s: %v", c, err)
		} else {
			testing.ContextLogf(ctx, "Chameleond on %s was last updated at %q to commit %q", c, chameleondUpdatedAt, chameleondLastCommit)
		}
		c.disconnectBtpeerd(ctx)
		connectServicesFn = func(ctx context.Context) error {
			// Connect to just chameleond, as btpeerd does not exist on device.
			testing.ContextLogf(ctx, "Connecting to chameleond on %s", c)
			if err := c.connectChameleond(ctx); err != nil {
				return errors.Wrapf(err, "failed to connect to chameleond on %s", c)
			}
			testing.ContextLogf(ctx, "Successfully connected to chameleond on %s", c)
			return nil
		}
	}

	// Connect, retrying after a reboot if it fails.
	testing.ContextLogf(ctx, "Connecting to services on %s", c)
	if err := connectServicesFn(ctx); err != nil {
		testing.ContextLogf(ctx, "WARNING: Failed to connect to services on %s in first attempt: %v", c, err)
		testing.ContextLogf(ctx, "Rebooting %s and reattempting to connect to services", c)
		c.disconnectBtpeerd(ctx)
		c.disconnectChameleond(ctx)
		if err := c.Reboot(ctx); err != nil {
			return errors.Wrapf(err, "failed to reboot %s after first services connection failure", c)
		}
		// Try again with a short poll as ssh may come up before services.
		testing.ContextLogf(ctx, "Connecting to services on %s after successful reboot", c)
		if err := testing.Poll(ctx, connectServicesFn, &testing.PollOptions{
			Interval: 500 * time.Millisecond,
			Timeout:  10 * time.Second,
		}); err != nil {
			return errors.Wrapf(err, "failed to connect to services on %s after successful reboot", c)
		}
	}
	testing.ContextLogf(ctx, "Successfully connected to services on %s", c)

	if c.hasCustomImage {
		// Log device info from btpeerd.
		testing.ContextLogf(ctx, "Retrieving device info from btpeerd on %s", c)
		deviceInfo, err := c.btpeerdClient.DeviceInfo(ctx, &btpeerd.DeviceInfoRequest{})
		if err != nil {
			return errors.Wrapf(err, "failed to get device info from btpeerd on %s", c)
		}
		deviceInfoJSON, err := protojson.Marshal(deviceInfo)
		if err != nil {
			return errors.Wrapf(err, "failed to marshal device info to JSON from btpeerd on %s", c)
		}
		testing.ContextLogf(ctx, "Btpeer device info for %s: DeviceInfoResponse%s", c, string(deviceInfoJSON))
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
		return errors.Wrapf(err, "failed to connect to btpeer%d hostname %q over ssh", c.registrationID, c.hostname)
	}
	c.sshConn = sshConn
	return nil
}

func (c *BtpeerClient) disconnectSSH(ctx context.Context) {
	if c.sshConn != nil {
		if err := c.sshConn.Close(ctx); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to close ssh connection to %s: %v", c, err)
		}
		c.sshConn = nil
	}
}

// Reboot will trigger a reboot of the btpeer device over ssh then attempt to
// reestablish a new ssh connection to device until it is successful or times
// out.
//
// Anything that relies upon an ssh connection must be reconnected afterwards,
// as the previous ssh connection will have been severed. This may be done by
// calling Connect.
func (c *BtpeerClient) Reboot(ctx context.Context) error {
	if c.sshConn == nil {
		return errors.Errorf("failed to reboot %s: no active ssh connection to device", c)
	}
	// Reboot, ignoring the ssh error that occurs due to severed connection.
	_ = c.sshConn.CommandContext(ctx, "reboot").Run()
	_ = c.sshConn.Close(ctx)
	c.sshConn = nil
	// Try to reconnect via ssh until successful.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return c.connectSSH(ctx)
	}, &testing.PollOptions{
		Interval: 1 * time.Second,
		Timeout:  1 * time.Minute,
	}); err != nil {
		return errors.Wrapf(err, "failed to reconnect to %s over ssh after reboot", c)
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
		return errors.Errorf("failed to connect to chameleond for %s: no active ssh connection to device", c)
	}
	// Create an ssh tunnel to the chameleond port.
	onFwdError := func(err error) {
		testing.ContextLogf(ctx, "ERROR: ssh chameleond port forwarding error for %s: %v", c, err)
		c.chameleondClient = nil
	}
	remoteAddr := fmt.Sprintf("localhost:%d", btpeerChameleondPort)
	chameleondPortForwarder, err := c.sshConn.ForwardLocalToRemote("tcp", "localhost:0", remoteAddr, onFwdError)
	if err != nil {
		return errors.Wrapf(err, "failed to port forward chameleond port for %s", c)
	}
	c.chameleondPortForwarder = chameleondPortForwarder
	// Create a new chameleond client which uses forward chameleond port.
	chameleondClient, err := chameleon.NewChameleond(ctx, c.chameleondPortForwarder.ListenAddr().String())
	if err != nil {
		return errors.Wrapf(err, "failed to connect to chameleond on %s through forward chameleond port at %q", c, chameleondPortForwarder.ListenAddr().String())
	}
	c.chameleondClient = chameleondClient
	return nil
}

// fetchChameleondVersion parses and returns the chameleond version from the
// chameleond version log file on the btpeer device.
func (c *BtpeerClient) fetchChameleondVersion(ctx context.Context) (string, string, error) {
	if c.sshConn == nil {
		return "", "", errors.Errorf("failed to fetch chameleond version from %s: no active ssh connection to device", c)
	}
	// Attempt to fetch the chameleond version (not supported on old versions).
	var chameleondLastCommit, chameleondUpdatedAt string
	btpeerVersionLogFileExists, err := c.remoteFileExists(ctx, c.sshConn, btpeerChameleondVersionLegacyLogFilePath)
	if err != nil {
		return "", "", errors.Wrapf(err, "failed to check for chameleond log file %q on %s", btpeerChameleondLogFilePath, c)
	}
	if btpeerVersionLogFileExists {
		lastLogLine, err := c.sshConn.CommandContext(ctx, "tail", "-1", btpeerChameleondVersionLegacyLogFilePath).Output()
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
			testing.ContextLogf(ctx, "WARNING: Failed to shut down forwarded chameleond port tunnel for %s: %v", c, err)
		}
		c.chameleondPortForwarder = nil
	}
}

// connectBtpeerd will create a new ssh tunnel to the btpeerd port on the
// btpeer device and create a new chameleond client connected through that
// tunnel. Requires an active ssh connection to the device. Any existing
// ssh tunnel or chameleond client is closed and replaced.
func (c *BtpeerClient) connectBtpeerd(ctx context.Context) error {
	c.disconnectBtpeerd(ctx)
	if c.sshConn == nil {
		return errors.Errorf("failed to connect to btpeerd for %s: no active ssh connection to device", c)
	}
	// Create an ssh tunnel to the chameleond port.
	onFwdError := func(err error) {
		testing.ContextLogf(ctx, "ERROR: ssh btpeerd port forwarding error for %s: %v", c, err)
		c.disconnectBtpeerd(ctx)
	}
	remoteAddr := fmt.Sprintf("localhost:%d", btpeerBtpeerdPort)
	portForwarder, err := c.sshConn.ForwardLocalToRemote("tcp", "localhost:0", remoteAddr, onFwdError)
	if err != nil {
		return errors.Wrapf(err, "failed to port forward btpeerd port for %s", c)
	}
	c.btpeerdPortForwarder = portForwarder
	// Create a new btpeerd client which uses forwarded port.
	grpcConn, err := grpc.Dial(c.btpeerdPortForwarder.ListenAddr().String(), grpc.WithInsecure())
	if err != nil {
		return errors.Wrapf(err, "failed to connect to btpeerd gRPC server on %s through forwarded btpeerd port at %q", c, portForwarder.ListenAddr().String())
	}
	c.btpeerdGRPCClientConn = grpcConn
	c.btpeerdClient = btpeerd.NewBtpeerManagementServiceClient(c.btpeerdGRPCClientConn)
	return nil
}

func (c *BtpeerClient) disconnectBtpeerd(ctx context.Context) {
	// Disconnect from gRPC server.
	if c.btpeerdGRPCClientConn != nil {
		if err := c.btpeerdGRPCClientConn.Close(); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to close connection to btpeerd gRPC server for %s: %v", c, err)
		}
		c.btpeerdGRPCClientConn = nil
	}
	c.btpeerdClient = nil
	// Close ssh tunnel.
	if c.btpeerdPortForwarder != nil {
		if err := c.btpeerdPortForwarder.Close(); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to shut down forwarded btpeerd port tunnel for %s: %v", c, err)
		}
		c.btpeerdPortForwarder = nil
	}
}

// StartLogCollection starts the collection of logs from the btpeer. The log
// collection will continue until StopLogCollection or Disconnect is called or
// until the ctx expires.
func (c *BtpeerClient) StartLogCollection(ctx context.Context) error {
	c.StopLogCollection(ctx)
	if c.sshConn == nil {
		return errors.Errorf("failed to start log collection on %s: no active ssh connection to device", c)
	}
	// Start collecting system logs (includes btpeerd logs).
	systemLogCollector, err := log.StartJournalctlCollector(ctx, c.sshConn, "--output", "short-full")
	if err != nil {
		return errors.Wrapf(err, "failed to start collecting system logs on %s", c)
	}
	c.systemLogCollector = systemLogCollector
	// Start collecting chameleond logs.
	hasChameleondLogFile, err := c.remoteFileExists(ctx, c.sshConn, btpeerChameleondLogFilePath)
	if err != nil {
		return errors.Wrapf(err, "failed to check for chameleond log file %q on %s", btpeerChameleondLogFilePath, c)
	}
	if hasChameleondLogFile {
		chameleondLogCollector, err := log.StartTailCollector(ctx, c.sshConn, btpeerChameleondLogFilePath, true)
		if err != nil {
			return errors.Wrapf(err, "failed to start collecting chameleond logs on %s", c)
		}
		c.chameleondLogCollector = chameleondLogCollector
	}
	return nil
}

// StopLogCollection stops any ongoing log collection started by StartLogCollection.
func (c *BtpeerClient) StopLogCollection(ctx context.Context) {
	if c.systemLogCollector != nil {
		if err := c.systemLogCollector.Close(); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to close system log collector for %s: %v", c, err)
		}
		c.systemLogCollector = nil
	}
	if c.chameleondLogCollector != nil {
		if err := c.chameleondLogCollector.Close(); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to close chameleond log collector for %s: %v", c, err)
		}
		c.chameleondLogCollector = nil
	}
}

// Disconnect will close log collectors, clients, and ssh connections to the
// btpeer device. Connect must be called in order to use the btpeer again.
func (c *BtpeerClient) Disconnect(ctx context.Context) {
	c.StopLogCollection(ctx)
	c.disconnectBtpeerd(ctx)
	c.disconnectChameleond(ctx)
	c.disconnectSSH(ctx)
}

// Hostname returns the hostname of the btpeer device.
func (c *BtpeerClient) Hostname() string {
	return c.hostname
}

// RegistrationID returns the ID set when the BtpeerClient was created. This
// should be the btpeer number in the testbed (e.g. 1 for btpeer1), or the order
// in which the btpeer hostnames were provided to Tast.
func (c *BtpeerClient) RegistrationID() int {
	return c.registrationID
}

// String returns a loggable description of the btpeer this client is for,
// including its Hostname and RegistrationID.
func (c *BtpeerClient) String() string {
	return fmt.Sprintf("btpeer%d[%s]", c.registrationID, c.hostname)
}

// ChameleondClient returns a chameleond client for this btpeer device.
//
// Note: All chameleond method calls are routed through an ssh tunnel to the
// chameleond service running on the btpeer device.
func (c *BtpeerClient) ChameleondClient() chameleon.Chameleond {
	if c.chameleondClient == nil {
		// Panic rather than throw an error to allow for ease of use, as this is not
		// meant to happen during expected usage in tests.
		panic(fmt.Sprintf("no open chameleond client available for %s", c))
	}
	return c.chameleondClient
}

// BtpeerdClient returns a btpeerd client for this btpeer device.
//
// Note: All btpeerd method calls are routed through an ssh tunnel to the
// btpeerd service running on the btpeer device.
//
// Once btpeerd has been rolled out to all lab btpeers, this should be simplified
// to panic if not set like ChameleondClient.
func (c *BtpeerClient) BtpeerdClient() (btpeerd.BtpeerManagementServiceClient, error) {
	if c.btpeerdClient == nil {
		return nil, errors.Errorf("no open btpeerd client available for %s", c)
	}
	return c.btpeerdClient, nil
}

// IsConnected returns true if there is an existing ssh connection, usable
// chameleond client, and either the device does not have btpeerd or there is a
// usable btpeerd client.
func (c *BtpeerClient) IsConnected() bool {
	return c.sshConn != nil && c.chameleondClient != nil &&
		(!c.hasCustomImage || c.btpeerdClient != nil)
}

// Reset resets the btpeer to return it to its normal state and clear any
// changes any test may have made to them.
func (c *BtpeerClient) Reset(ctx context.Context) error {
	if !c.IsConnected() {
		return errors.Errorf("failed to reset %s: not connected to device", c)
	}
	testing.ContextLogf(ctx, "Resetting %s", c)
	// Reset the base chameleond service state.
	if err := c.chameleondClient.Reset(ctx); err != nil {
		return errors.Wrapf(err, "failed to reset chameleond on %q", c)
	}
	// Reset the bluetooth service state, through the keyboard device interface
	// since this method is not exposed at a higher level.
	if err := c.chameleondClient.BluetoothKeyboardDevice().ResetStack(ctx, ""); err != nil {
		return errors.Wrapf(err, "failed to reset bluetooth stack on %s", c)
	}
	testing.ContextLogf(ctx, "Successfully reset %s ", c)
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
//
// <context_dir>/
//
//	    btpeer_logs/
//	        btpeer<registrationID>_<hostname>/
//		           chameleond/
//		               <dump_timestamp>_<dumpContext1>.log
//		               <dump_timestamp>_<dumpContext2>.log
//		               ...
//		           system/
//		               <dump_timestamp>_<dumpContext1>.log
//		               <dump_timestamp>_<dumpContext2>.log
//		               ...
func (c *BtpeerClient) DumpLogs(ctx context.Context, dumpContext string) error {
	btpeerDirName := fmt.Sprintf("btpeer%d_%s", c.registrationID, c.hostname)
	btpeerDirName = regexp.MustCompile(`[\W_]+`).ReplaceAllString(btpeerDirName, "_")
	baseLogDir := filepath.Join("btpeer_logs", btpeerDirName)
	systemLogDir := filepath.Join(baseLogDir, "system")
	chameleondLogDir := filepath.Join(baseLogDir, "chameleond")
	if c.systemLogCollector != nil {
		testing.ContextLogf(ctx, "Dumping collected system logs from %s for %q", c, dumpContext)
		if err := log.DumpCollectedLogsToFile(ctx, c.systemLogCollector, systemLogDir, dumpContext); err != nil {
			return errors.Wrapf(err, "failed to dump collected system logs from %s", c)
		}
	}
	if c.chameleondLogCollector != nil {
		testing.ContextLogf(ctx, "Dumping collected chameleond logs from %s for %q", c, dumpContext)
		if err := log.DumpCollectedLogsToFile(ctx, c.chameleondLogCollector, chameleondLogDir, dumpContext); err != nil {
			return errors.Wrapf(err, "failed to dump collected chameleond logs from %s", c)
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
