// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

import (
	"context"
	"encoding/json"
	"strings"

	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/testenv/middns"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Env is a thin layer that provides user-facing interfaces that are to be commonly used to start and close the test environment.
// This can manage the lifecycle of the underlying servers (eg, the middle-layer DNS or proxy server)
// and configure them with various options for HTTP/S redirection or network verification.
type Env interface {
	// Close is called to stop the servers and release the resources used when the test environment is no longer needed.
	// It's a caller's responsibility to call Close after use.
	Close(context.Context) error
}

// BaseEnv is a base struct of Env that is partly implemented that will be useful for a concrete Env to use for their own implementation of Env.
type BaseEnv struct {
	Env
	Name string

	// Env vars that are defined in the data file that should be passed in when BaseEnv is initialized.
	vars map[string]interface{}

	// Runtime configs set by Options
	hostMap     map[string]string // A map of (host aliases => host names), eg {"example-prod": "www.example.com", "example-preprod": "preprod.example.com"}
	redirectMap map[string]string // A map of (original hosts => destination hosts), eg {"example-prod": "example-preprod"}

	// Redirection configs
	shouldRedirect         bool
	portalDetectionEnabled bool
	midDNS                 *middns.DNSServer

	cleanups []func(context.Context) error // will be called in a reverse order during Close
}

// NewBase creates a new base test environment and starts setting the requested environment.
// Env options can be passed in for various scenarios.
func NewBase(ctx context.Context, name string, opts ...Option) (*BaseEnv, error) {
	b := &BaseEnv{
		Name:                   name,
		vars:                   make(map[string]interface{}),
		hostMap:                make(map[string]string),
		redirectMap:            make(map[string]string),
		portalDetectionEnabled: true,
	}

	// Parse options and configure the BaseEnv with them
	for _, opt := range opts {
		if err := opt(b); err != nil {
			return nil, err
		}
	}
	// Parse the host alias map
	if val, ok := b.vars[hostMapVar]; ok {
		hostMap, err := parseHostMap(ctx, val.(string))
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse HostMap env var")
		}
		b.hostMap = hostMap
	}

	// Resolve host alias to host name to pass in to the DNS for redirection
	if val, ok := b.vars[redirectMapVar]; ok {
		if len(b.hostMap) == 0 {
			return nil, errors.Errorf("var %v needs the host (%v) to be defined first", redirectMapVar, hostMapVar)
		}
		resolved := map[string]string{}
		for from, to := range val.(map[string]string) {
			resolvedFrom, ok := b.hostMap[from]
			if !ok {
				return nil, errors.Errorf("host alias not found: %v (undefined in yaml?)", from)
			}
			resolvedTo := "#" // "#" means bypassing redirect
			if to != "#" {
				// If not bypassed, read the destination host from the map
				resolvedTo, ok = b.hostMap[to]
				if !ok {
					return nil, errors.Errorf("host alias not found: %v (undefined in yaml?)", to)
				}
			}
			resolved[resolvedFrom] = resolvedTo
		}
		b.redirectMap = resolved
		b.shouldRedirect = len(b.redirectMap) > 0
	}

	// TODO(b/342085937): Remove the line below to turn back on DNS based redirection after troubleshooting login failure by an invalid DNS IP entry.
	b.shouldRedirect = false

	// Configure redirection for the given hosts using either a DNS server or /etc/hosts updater.
	if b.shouldRedirect {
		midDNS := middns.NewDNSServer()
		b.cleanups = append(b.cleanups, midDNS.Close)
		b.midDNS = midDNS
	}

	// Start a new environment.
	if err := b.start(ctx); err != nil {
		b.Close(ctx)
		return nil, errors.Wrap(err, "failed to start new environment")
	}
	return b, nil
}

// start starts the necessary servers with the configurations to set up the test environment.
func (b *BaseEnv) start(ctx context.Context) error {
	if b.shouldRedirect {
		// Start host redirection.
		if !b.portalDetectionEnabled {
			// Disable portal detection while the DNS server runs to avoid conflicts on DNS queries, resulting in lost connection.
			manager, err := shill.NewManager(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to create shill manager")
			}
			if err := manager.DisablePortalDetection(ctx); err != nil {
				return errors.Wrap(err, "failed to disable portal detection")
			}
			testing.ContextLog(ctx, "testenv: portal detection disabled before mid DNS starts")

			// Re-enable portal detection on cleanup.
			b.cleanups = append(b.cleanups, manager.EnablePortalDetection)
		}
		if err := b.midDNS.Start(ctx, b.redirectMap); err != nil {
			return errors.Wrap(err, "failed to redirect hosts using mid DNS server")
		}
	}
	testing.ContextLog(ctx, "testenv: started for external dependencies")
	return nil
}

// Close stops the running servers and cleans up any resources used to set up this environment.
// It should be safe to call Close more than once.
func (b *BaseEnv) Close(ctx context.Context) error {
	defer func() {
		// Run cleanups in a reverse order, so LIFO (last in first out) for deferred cleanup in the queue.
		for i := len(b.cleanups) - 1; i >= 0; i-- {
			if err := b.cleanups[i](ctx); err != nil {
				testing.ContextLog(ctx, "logging cleanup failure: ", err)
			}
		}
		b.cleanups = []func(context.Context) error{}
	}()
	testing.ContextLog(ctx, "testenv: closed")
	return nil
}

// parseHostMap reads a given JSON to get a map of host aliases to names.
// Host alias must be suffixed with the valid env name (eg, "-preprod", "-prod")
func parseHostMap(ctx context.Context, hostsJSON string) (map[string]string, error) {
	if hostsJSON == "" {
		return nil, errors.New("hostsJSON can't be empty")
	}
	// Lowercase the JSON string for easy comparison
	hostsJSON = strings.ToLower(hostsJSON)

	ret := make(map[string]string)
	if err := json.Unmarshal([]byte(hostsJSON), &ret); err != nil {
		return nil, errors.Wrap(err, "failed to decode hosts in json")
	}

	// Check the validity.
	for alias, hostname := range ret {
		if !isValidAlias(alias) {
			return nil, errors.Errorf("invalid alias: %v", alias)
		}
		if !isValidHostname(hostname) {
			return nil, errors.Errorf("invalid hostname associated with alias: %v", alias)
		}
	}
	return ret, nil
}
