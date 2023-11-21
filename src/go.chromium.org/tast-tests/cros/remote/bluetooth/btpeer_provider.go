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
	nextRegistrationID          int
	registeredBtpeers           []*BtpeerClient
	hostsThatFailedRegistration []string
}

// GetBtpeerProvider returns the singleton instance of BtpeerProvider.
func GetBtpeerProvider() *BtpeerProvider {
	if btpeerProviderSingleton == nil {
		btpeerProviderSingleton = &BtpeerProvider{
			nextRegistrationID: 1,
		}
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
//
// Registration for all hosts will only be attempted once for this
// BtpeerProvider, even if it fails. Further calls to register the same host
// will be skipped. Success/failure of host registration is logged to context.
func (m *BtpeerProvider) RegisterBtpeerHostsByWificellDutHostname(ctx context.Context, sshOptions *ssh.Options, dutHostname string) error {
	dutHostnameWithoutPort := dutHostname
	if hostname, _, err := net.SplitHostPort(dutHostname); err == nil {
		dutHostnameWithoutPort = hostname
	}
	if dutHostnameWithoutPort == "localhost" || net.ParseIP(dutHostnameWithoutPort) != nil {
		return errors.Errorf("cannot resolve companion hostname from localhost or IP dut hostname %q", dutHostname)
	}
	testing.ContextLogf(ctx, "Registering up to %d possible btpeers for dut %q", maxPossibleBtpeersInWificell, dutHostnameWithoutPort)
	for i := 1; i <= maxPossibleBtpeersInWificell; i++ {
		btpeerSuffix := fmt.Sprintf("-btpeer%d", i)
		btpeerHost, err := utils.CompanionDeviceHostname(dutHostnameWithoutPort, btpeerSuffix)
		if err != nil {
			return errors.Wrapf(err, "failed to build companion device hostname for suffix %q", btpeerSuffix)
		}
		m.registerBtpeerHost(ctx, sshOptions, btpeerHost)
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
//
// Registration for all hosts will only be attempted once for this
// BtpeerProvider, even if it fails. Further calls to register the same host
// will be skipped. Success/failure of host registration is logged to context.
func (m *BtpeerProvider) RegisterBtpeerHosts(ctx context.Context, sshOptions *ssh.Options, btpeerHosts ...string) {
	for _, btpeerHost := range btpeerHosts {
		m.registerBtpeerHost(ctx, sshOptions, btpeerHost)
	}
}

// registerBtpeerHost validates that the hostname is either a localhost tunnel
// or is resolvable, initializes a new BtpeerClient, connects to the btpeer,
// and saves the client for later use.
//
// Registration for all hosts will only be attempted once for this
// BtpeerProvider, even if it fails. Further calls to register the same host
// will be skipped. Success/failure of host registration is logged to context.
func (m *BtpeerProvider) registerBtpeerHost(ctx context.Context, sshOptions *ssh.Options, btpeerHost string) {
	// Skip if registration already attempted for host.
	for _, btpeer := range m.registeredBtpeers {
		if btpeer.hostname == btpeerHost {
			testing.ContextLogf(ctx, "Skipping registration of btpeer host %q as it is already successfully registered", btpeerHost)
			return
		}
	}
	for _, failedBtpeer := range m.hostsThatFailedRegistration {
		if failedBtpeer == btpeerHost {
			testing.ContextLogf(ctx, "Skipping registration of btpeer host %q as it already failed registration once", btpeerHost)
			return
		}
	}
	testing.ContextLogf(ctx, "Registering new btpeer host %q", btpeerHost)
	// Register at the end based on success.
	registrationID := m.nextRegistrationID
	m.nextRegistrationID++
	var registrationError error
	var btpeer *BtpeerClient
	defer (func() {
		if registrationError != nil {
			testing.ContextLogf(ctx, "WARNING: Failed to register btpeer host %q: %v", btpeerHost, registrationError)
			m.hostsThatFailedRegistration = append(m.hostsThatFailedRegistration)
		} else {
			// Register for later use.
			m.registeredBtpeers = append(m.registeredBtpeers, btpeer)
			testing.ContextLogf(ctx, "Successfully registered new btpeer host %q as btpeer %s", btpeerHost, btpeer)
		}
	})()
	// Allow only localhost tunnels or resolvable hostnames.
	hostnameHadPort := false
	hostnameWithoutPort := btpeerHost
	if hostname, _, err := net.SplitHostPort(hostnameWithoutPort); err == nil {
		hostnameHadPort = true
		hostnameWithoutPort = hostname
	}
	if hostnameWithoutPort == "localhost" || hostnameWithoutPort == "127.0.0.1" {
		if !hostnameHadPort {
			registrationError = errors.New("btpeer hostname identified as localhost, but is missing forwarding port")
			return
		}
	} else if _, err := net.LookupIP(hostnameWithoutPort); err != nil {
		registrationError = errors.Wrapf(err, "failed to resolve IP for btpeer hostname %q", hostnameWithoutPort)
		return
	}
	// Initialize btpeer client connection.
	// Note: Every btpeer should be connected to at least once, even if they are
	// not needed, so that they are all reset during ConnectAndReset at least once.
	btpeer, err := newBtpeerClient(btpeerHost, sshOptions, registrationID)
	if err != nil {
		registrationError = errors.Wrap(err, "failed to initialize new BtpeerClient")
		return
	}
	if err := btpeer.Connect(ctx); err != nil {
		registrationError = errors.Wrapf(err, "failed to connect to btpeer %s", btpeer)
		return
	}
}

// RegisteredBtpeers will return the total number of successfully registered
// btpeers.
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
// A non-nil error will be thrown if more btpeers are requested than
// successfully registered with RegisterBtpeerHosts and/or
// RegisterBtpeerHostsByWificellDutHostname.
func (m *BtpeerProvider) ConnectAndReset(ctx context.Context, btpeerCount int) ([]*BtpeerClient, error) {
	if btpeerCount <= 0 {
		return nil, errors.Errorf("invalid btpeerCount %d: must be greater than zero", btpeerCount)
	}
	if len(m.registeredBtpeers) < btpeerCount {
		return nil, errors.Errorf("not enough successfully registered btpeers (%d) to meet requested amount (%d)", len(m.registeredBtpeers), btpeerCount)
	}
	var selectedBtpeers []*BtpeerClient
	var btpeersToReset []*BtpeerClient
	var btpeersToDisconnect []*BtpeerClient
	for i := 0; i < len(m.registeredBtpeers); i++ {
		btpeer := m.registeredBtpeers[i]
		if i < btpeerCount {
			if !btpeer.IsConnected() {
				testing.ContextLogf(ctx, "Connecting to %s", btpeer)
				if err := btpeer.Connect(ctx); err != nil {
					return nil, errors.Errorf("failed to connect to %s", btpeer)
				}
				testing.ContextLogf(ctx, "Successfully connected to %s", btpeer)
			} else {
				testing.ContextLogf(ctx, "Reusing existing connection to %s", btpeer)
			}
			btpeersToReset = append(btpeersToReset, btpeer)
			selectedBtpeers = append(selectedBtpeers, btpeer)
		} else if btpeer.IsConnected() {
			testing.ContextLogf(ctx, "Found unwanted connected btpeer %s", btpeer)
			btpeersToReset = append(btpeersToReset, btpeer)
			btpeersToDisconnect = append(btpeersToDisconnect, btpeer)
		}
	}
	if err := m.Reset(ctx, btpeersToReset...); err != nil {
		return nil, errors.Wrapf(err, "failed to reset all %d btpeers", len(btpeersToReset))
	}
	for _, btpeer := range btpeersToDisconnect {
		testing.ContextLogf(ctx, "Disconnecting from unwanted connected btpeer %s", btpeer)
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
