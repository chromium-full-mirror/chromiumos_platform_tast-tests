// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

import (
	"context"
	"encoding/json"
	"strings"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Env is a thin layer that provides user-facing interfaces that are to be commonly used to start and close the test environment.
// This can manage the lifecycle of the underlying servers (eg, the middle-layer DNS or proxy server)
// and configure them with various options for HTTP/S redirection or network verification.
type Env interface {
	// Start is called to start the servers for the configurations passed in to the New* methods.
	// Once started, it can't be restarted with a new configuration for now but closing and starting will do the trick when needed to reload the configuration.
	Start(context.Context) error
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

	// DNS configs
	shouldRedirect bool
	hostsUpdater   *HostsUpdater

	cleanups []func(context.Context) error // will be called in a reverse order during Close
}

// NewBase creates a new base test environment and configures it with the given options.
func NewBase(ctx context.Context, name string, opts ...Option) (*BaseEnv, error) {
	b := &BaseEnv{
		Name:        name,
		vars:        make(map[string]interface{}),
		hostMap:     make(map[string]string),
		redirectMap: make(map[string]string),
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

	// Configure redirection for the given hosts using the updater
	if b.shouldRedirect {
		hostsUpdater, err := NewHostsUpdater(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to init /etc/hosts updater")
		}
		b.cleanups = append(b.cleanups, hostsUpdater.Cleanup)
		b.hostsUpdater = hostsUpdater
	}
	return b, nil
}

// Start starts the necessary servers with the configurations to set up the test environment.
func (b *BaseEnv) Start(ctx context.Context) error {
	if b.shouldRedirect && b.hostsUpdater != nil {
		if _, err := b.hostsUpdater.Redirect(ctx, b.redirectMap); err != nil {
			return errors.Wrap(err, "failed to override hosts in /etc/hosts")
		}
	}
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
