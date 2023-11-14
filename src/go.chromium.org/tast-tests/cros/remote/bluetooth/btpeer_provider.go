// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"golang.org/x/sync/errgroup"
)

const maxPossibleBtpeersInWificell = 4

var btpeerProviderSingleton *BtpeerProvider

// BtpeerProvider manages btpeers used by fixtures. Only one instance should
// exist for all fixtures, which should be retrieved with GetBtpeerProvider.
//
// Btpeers must first be registered by either
// RegisterBtpeerHostsByWificellDutHostname or RegisterBtpeerHosts and then
// ConnectAndReset may be used to get a BtpeerClient for the desired amount of
// btpeers.
type BtpeerProvider struct {
	registeredBtpeers []*BtpeerClient
}

// GetBtpeerProvider returns the singleton instance of BtpeerProvider.
func GetBtpeerProvider() *BtpeerProvider {
	if btpeerProviderSingleton == nil {
		btpeerProviderSingleton = &BtpeerProvider{}
	}
	return btpeerProviderSingleton
}

// RegisterBtpeerHostsByWificellDutHostname registers btpeer hosts found to
// exist as companions of the provided dut by their expected hostnames if they
// are not already registered.
//
// Registration validates that the hostname is resolvable, initializes a new
// BtpeerClient, connects to the btpeer, and saves the client for later use.
//
// Registered btpeers may be retrieved with ConnectAndReset.
func (m *BtpeerProvider) RegisterBtpeerHostsByWificellDutHostname(ctx context.Context, sshOptions *ssh.Options, dutHostname string) error {
	// Get the base dut hostname.
	dutHostnameWithoutPort := dutHostname
	if hostname, _, err := net.SplitHostPort(dutHostname); err == nil {
		dutHostnameWithoutPort = hostname
	}
	if dutHostnameWithoutPort == "localhost" || net.ParseIP(dutHostnameWithoutPort) != nil {
		return errors.Errorf("cannot resolve companion hostname from localhost or IP dut hostname %q", dutHostname)
	}

	// Register btpeers that have valid hostnames only, checking all possible companion hostnames.
	var newBtpeerHostnames []string
	testing.ContextLogf(ctx, "Checking for existence for up to %d possible btpeers for dut %q", maxPossibleBtpeersInWificell, dutHostnameWithoutPort)
	for i := 1; i <= maxPossibleBtpeersInWificell; i++ {
		btpeerSuffix := fmt.Sprintf("-btpeer%d", i)
		btpeerHostname, err := utils.CompanionDeviceHostname(dutHostnameWithoutPort, btpeerSuffix)
		if err != nil {
			return errors.Wrapf(err, "failed to build companion device hostname for suffix %q", btpeerSuffix)
		}
		if m.isRegisteredBtpeerHost(btpeerHostname) {
			testing.ContextLogf(ctx, "Skipping registration of already-registered btpeer host %q", btpeerHostname)
		}
		if _, err := net.LookupIP(btpeerHostname); err != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to resolve IP for possible btpeer host %q: %v", btpeerHostname, err)
			testing.ContextLogf(ctx, "Assuming btpeer host %q does not exist", btpeerHostname)
		} else {
			testing.ContextLogf(ctx, "Found valid btpeer host %q", btpeerHostname)
			newBtpeerHostnames = append(newBtpeerHostnames, btpeerHostname)
		}
	}
	testing.ContextLogf(ctx, "Found %d new btpeer hosts to register for dut %q", len(newBtpeerHostnames), dutHostnameWithoutPort)
	for _, host := range newBtpeerHostnames {
		testing.ContextLogf(ctx, "Registering new btpeer host %q", host)
		if err := m.registerBtpeerHost(ctx, sshOptions, host); err != nil {
			return errors.Wrapf(err, "failed to register btpeer host %q", host)
		}
		testing.ContextLogf(ctx, "Successfully registered new btpeer host %q", host)
	}
	return nil
}

// RegisterBtpeerHosts registers the provided hostnames as btpeers if they are
// not already registered.
//
// Can be provided as ssh tunnels to btpeers (e.g. localhost:2201->my-btpeer:22)
// or normal hostnames. Note that normal hostnames will only be resolvable from
// within the lab.
//
// Registration validates that the hostname is either a localhost tunnel or is
// resolvable, initializes a new BtpeerClient, connects to the btpeer, and saves
// the client for later use.
//
// Registered btpeers may be retrieved with ConnectAndReset.
func (m *BtpeerProvider) RegisterBtpeerHosts(ctx context.Context, sshOptions *ssh.Options, btpeerHosts ...string) error {
	for _, host := range btpeerHosts {
		if m.isRegisteredBtpeerHost(host) {
			testing.ContextLogf(ctx, "Skipping registration of already-registered btpeer host %q", host)
			continue
		}
		testing.ContextLogf(ctx, "Registering new btpeer host %q", host)
		if err := m.registerBtpeerHost(ctx, sshOptions, host); err != nil {
			return errors.Wrapf(err, "failed to register btpeer host %q", host)
		}
		testing.ContextLogf(ctx, "Successfully registered new btpeer host %q", host)
	}
	return nil
}

// registerBtpeerHost validates that the hostname is either a localhost tunnel
// or is resolvable, initializes a new BtpeerClient, connects to the btpeer,
// and saves the client for later use.
func (m *BtpeerProvider) registerBtpeerHost(ctx context.Context, sshOptions *ssh.Options, btpeerHost string) error {
	// Remove port from hostname.
	hostnameHadPort := false
	hostnameWithoutPort := btpeerHost
	if hostname, _, err := net.SplitHostPort(hostnameWithoutPort); err == nil {
		hostnameHadPort = true
		hostnameWithoutPort = hostname
	}
	// Allow only localhost tunnels or resolvable hostnames.
	if hostnameWithoutPort == "localhost" || hostnameWithoutPort == "127.0.0.1" {
		if !hostnameHadPort {
			return errors.New("btpeer hostname identified as localhost, but is missing forwarding port")
		}
	} else if _, err := net.LookupIP(hostnameWithoutPort); err != nil {
		return errors.Wrapf(err, "failed to resolve IP for btpeer hostname %q", hostnameWithoutPort)
	}
	// Prepare a client.
	btpeer, err := newBtpeerClient(btpeerHost, sshOptions)
	if err != nil {
		return errors.Wrapf(err, "failed to initialize new BtpeerClient for btpeer host %q", btpeerHost)
	}
	// Connect to btpeer.
	if err := btpeer.Connect(ctx); err != nil {
		return errors.Wrapf(err, "failed to connect to btpeer host %q during registration", btpeerHost)
	}
	// Register for later use.
	m.registeredBtpeers = append(m.registeredBtpeers, btpeer)
	return nil
}

func (m *BtpeerProvider) isRegisteredBtpeerHost(btpeerHostname string) bool {
	for _, btpeer := range m.registeredBtpeers {
		if btpeer.hostname == btpeerHostname {
			return true
		}
	}
	return false
}

// RegisteredBtpeers will return the total number of registered btpeers.
func (m *BtpeerProvider) RegisteredBtpeers() int {
	return len(m.registeredBtpeers)
}

// ConnectAndReset will connect to and reset the amount of desired btpeers and
// return access clients for each connected btpeer. Existing connections to
// btpeers will be reused, but the returned btpeers will have been reset.
//
// If there are more registered btpeers available than desired, excess connected
// btpeers will be reset and disconnected.
//
// StartLogCollection must be called for each btpeer with a valid context
// separately in order to begin collecting logs from them.
//
// A non-nil error will be thrown if more btpeers are requested than previously
// registered with RegisterByDutHostname and/or RegisterByBTPeerHostname.
func (m *BtpeerProvider) ConnectAndReset(ctx context.Context, btpeerCount int) ([]*BtpeerClient, error) {
	if btpeerCount <= 0 {
		return nil, errors.Errorf("invalid btpeerCount %d: must be greater than zero", btpeerCount)
	}
	if len(m.registeredBtpeers) < btpeerCount {
		return nil, errors.Errorf("not enough registered btpeers (%d) to meet requested amount (%d)", len(m.registeredBtpeers), btpeerCount)
	}
	var selectedBtpeers []*BtpeerClient
	var btpeersToReset []*BtpeerClient
	var btpeersToDisconnect []*BtpeerClient
	for i := 0; i < len(m.registeredBtpeers); i++ {
		btpeer := m.registeredBtpeers[i]
		if i < btpeerCount {
			if !btpeer.IsConnected() {
				testing.ContextLogf(ctx, "Connecting to btpeer %q", btpeer.hostname)
				if err := btpeer.Connect(ctx); err != nil {
					return nil, errors.Errorf("failed to connect to btpeer %q", btpeer.hostname)
				}
				testing.ContextLogf(ctx, "Successfully connected to btpeer %q", btpeer.hostname)
			} else {
				testing.ContextLogf(ctx, "Reusing existing connection to btpeer %q", btpeer.hostname)
			}
			btpeersToReset = append(btpeersToReset, btpeer)
			selectedBtpeers = append(selectedBtpeers, btpeer)
		} else if btpeer.IsConnected() {
			testing.ContextLogf(ctx, "Found unwanted connected btpeer %q", btpeer.hostname)
			btpeersToReset = append(btpeersToReset, btpeer)
			btpeersToDisconnect = append(btpeersToDisconnect, btpeer)
		}
	}
	if err := m.Reset(ctx, btpeersToReset...); err != nil {
		return nil, errors.Wrapf(err, "failed to reset all %d btpeers", len(btpeersToReset))
	}
	for _, btpeer := range btpeersToDisconnect {
		testing.ContextLogf(ctx, "Disconnecting from unwanted connected btpeer %q", btpeer.hostname)
		btpeer.Disconnect(ctx)
	}
	return selectedBtpeers, nil
}

// DisconnectAll will disconnect all registered btpeers, ignoring their
// connection status.
func (m *BtpeerProvider) DisconnectAll(ctx context.Context) {
	for _, btpeer := range m.registeredBtpeers {
		btpeer.Disconnect(ctx)
	}
}

// connectedBtpeers collects all registered btpeers that are connected.
func (m *BtpeerProvider) connectedBtpeers() []*BtpeerClient {
	var btpeers []*BtpeerClient
	for _, btpeer := range m.registeredBtpeers {
		if btpeer.IsConnected() {
			btpeers = append(btpeers, btpeer)
		}
	}
	return btpeers
}

// Reset calls BtpeerClient.Reset for each btpeer to return them to their normal
// state and clear any changes a test may have made to them.
//
// Each btpeer is reset in parallel to save time. If any reset fails, the first
// error is returned and any pending resets are cancelled.
//
// Can be called from within a test.
func (m *BtpeerProvider) Reset(ctx context.Context, btpeers ...*BtpeerClient) error {
	if len(btpeers) == 0 {
		return nil
	}
	resetCtx, cancelResetCtx := context.WithTimeout(ctx, 1*time.Minute)
	defer cancelResetCtx()
	resetGroup, resetCtx := errgroup.WithContext(resetCtx)
	testing.ContextLogf(ctx, "Resetting %d btpeers concurrently", len(btpeers))
	for _, btpeer := range btpeers {
		// Note: loop var values are copied to inner vars for use in func literal.
		btpeer := btpeer
		resetGroup.Go(func() error {
			return btpeer.Reset(resetCtx)
		})
	}
	if err := resetGroup.Wait(); err != nil {
		return errors.Wrapf(err, "failed to reset all %d btpeers", len(btpeers))
	}
	testing.ContextLogf(ctx, "Successfully reset all %d btpeers", len(btpeers))
	return nil
}
