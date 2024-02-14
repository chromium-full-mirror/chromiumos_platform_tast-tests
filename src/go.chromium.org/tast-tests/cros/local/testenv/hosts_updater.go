// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testenv provides common interfaces to manage the test environments
package testenv

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// HostsUpdater manages to add or remove host entries in /etc/hosts on a DUT for testing, so
// a host can be resolved to an IP address of your target host. It allows to point your test
// at any desired environments such as preprod.
type HostsUpdater struct {
	hostsfile string
	resolver  Resolver
}

type entry struct {
	hostname string
	ip       string
}

// A map of the "from" hosts to the "to" host entries.
type entries map[string]entry

// label is an identifier added to each line of the entries modified by HostsUpdater.
const label = "#testenv-hosts-override="

// Marker files that let the infra to repair or reprovision a DUT when exists (specifically when a change made in tests becomes unrecoverable)
const (
	provisionFailedFilePath = "/mnt/stateful_partition/unencrypted/provision_failed"
	forceProvisionFilePath  = "/mnt/stateful_partition/.force_provision"
)

var provisionMarkers = []string{provisionFailedFilePath, forceProvisionFilePath}

// Resolver is a wrapper interface for the net package. Useful for dependency injection in testing.
type Resolver interface {
	LookupHost(hostname string) ([]string, error)
}

type defaultResolver struct {
	resolved map[string]string // Pairs of (host, IP that has been already known for the host)
}

// LookupHost looks up the given host using the local resolver.
func (r defaultResolver) LookupHost(hostname string) ([]string, error) {
	// If it has already been resolved, use the cached address.
	valid, ok := r.resolved[hostname]
	if ok {
		return []string{valid}, nil
	}

	addrs, err := net.LookupHost(hostname)
	if err != nil {
		return nil, errors.Wrap(err, "failed to look up host")
	}
	// Sort and return only the first valid address for use
	sort.Slice(addrs, func(i, j int) bool {
		return bytes.Compare(net.ParseIP(addrs[i]), net.ParseIP(addrs[j])) < 0
	})
	for _, addr := range addrs {
		if net.ParseIP(addr) != nil {
			valid = addr
			break
		}
	}
	if valid == "" {
		return nil, errors.New("no valid address found")
	}
	r.resolved[hostname] = valid
	return []string{valid}, nil
}

// NewHostsUpdater creates a HostsUpdater. Callers should defer call Cleanup to reset any override entries after use.
// Callers should defer call Cleanup to clean up any override entries after use.
func NewHostsUpdater(ctx context.Context) (*HostsUpdater, error) {
	return newHostsUpdaterInternal(ctx, "/etc/hosts", true, true, defaultResolver{resolved: make(map[string]string)})
}

// newHostsUpdaterInternal is an internal version of NewHostsUpdater() with useful params for testing:
//
//	hostsfile - "/etc/hosts" (default)
//	reset - true (default) to clear any override entries before update
//	failIfNotDUT - true (default) to ensure that it runs on a DUT
//	resolver - defaultResolver (default) or a local IP resolver that implements the Resolver interface
func newHostsUpdaterInternal(ctx context.Context, hostsfile string, reset, failIfNotDUT bool, resolver Resolver) (*HostsUpdater, error) {
	// Safeguard: This should only be run on a DUT, not a host machine.
	if failIfNotDUT && !isDUT() {
		return nil, errors.New("testenv should be run locally only on a DUT")
	}

	h := &HostsUpdater{hostsfile: hostsfile, resolver: resolver}
	if reset {
		if err := h.Cleanup(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to clean up host overrides before update")
		}
	}
	return h, nil
}

// Redirect updates /etc/hosts for redirection for the given map of (original => destination) hosts.
// It allows tests to point to target hosts in the local test environments.
func (h *HostsUpdater) Redirect(ctx context.Context, rules map[string]string) (entries, error) {
	modified, base, err := h.readHosts()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read the hosts for redirection")
	}
	for from, to := range rules {
		if from == to || to == "#" {
			testing.ContextLog(ctx, "Found bypassing redirect for: ", from)
		}
		// Avoid overriding the same host more than once.
		if val, ok := modified[from]; ok && val.hostname == to {
			return nil, errors.Wrapf(err, "already redirected from %v", from)
		}
		addrs, err := h.resolver.LookupHost(to)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to look up the target: %v", from)
		}
		modified[from] = entry{
			hostname: to,
			ip:       addrs[0], // There will be at least one valid IP when LookupHost returns no error.
		}
	}
	if len(modified) == 0 {
		return nil, errors.New("None to redirect")
	}

	testing.ContextLogf(ctx, "HostsUpdater: Redirect %v hosts", len(modified))
	if err := h.writeHosts(modified, base); err != nil {
		return nil, errors.Wrap(err, "failed to redirect hosts")
	}
	// Safeguard: Leave marker files, so the infra could force repair or provision a DUT if cleanup fails.
	if isDUT() {
		for _, f := range provisionMarkers {
			if err := os.WriteFile(f, nil, 0644); err != nil {
				return nil, errors.Wrapf(err, "failed to force provision: %v", f)
			}
		}
	}
	return modified, nil
}

// Cleanup clears up all the host redirects.
func (h *HostsUpdater) Cleanup(ctx context.Context) (retErr error) {
	// Safeguard: Delete the marker files when the cleanup succeeded.
	defer func(ctx context.Context) {
		if !isDUT() || retErr != nil {
			return
		}
		for _, f := range provisionMarkers {
			if _, err := os.Stat(f); err != nil {
				return
			}
			if err := os.Remove(f); err != nil {
				retErr = errors.Wrapf(err, "failed to remove the marker: %v", f)
				return
			}
		}
		testing.ContextLog(ctx, "HostsUpdater: Removed provision marker files")
	}(ctx)

	modified, base, err := h.readHosts()
	if err != nil {
		return errors.Wrap(err, "failed to read the host file to cleanup")
	}
	if len(modified) == 0 {
		return nil
	}
	testing.ContextLogf(ctx, "HostsUpdater: Close removing %v modifled entries", len(modified))
	// Write only the base lines for reset.
	err = h.writeHosts(entries{}, base)
	if err != nil {
		return errors.Wrap(err, "failed to write the host file for cleanup")
	}
	return nil
}

// readHosts reads /etc/hosts line by line, then returns the entries that are
// overridden and the base lines that exist before.
func (h *HostsUpdater) readHosts() (entries, []string, error) {
	var base []string
	modified := entries{}

	f, err := os.Open(h.hostsfile)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to read the host file: %v", h.hostsfile)
	}
	defer f.Close()

	// Read the override entries and the rest, and save them separately.
	// Example of the host override:
	// 10.0.0.1 foo.bar.baz #testenv-hosts-override=preprod-foo.bar.baz
	s := bufio.NewScanner(f)
	for s.Scan() {
		l := s.Text()
		parts := strings.SplitN(l, label, 2)
		if len(parts) == 2 {
			fields := strings.Fields(parts[0])
			host := strings.TrimSpace(fields[1])
			// If the same host is overridden more than once,
			// the first match wins in /etc/hosts and the others will be ignored.
			if _, ok := modified[host]; !ok {
				modified[host] = entry{
					hostname: strings.TrimSpace(parts[1]),
					ip:       strings.TrimSpace(fields[0]),
				}
			}
		} else {
			base = append(base, l)
		}
	}
	if err := s.Err(); err != nil {
		return nil, nil, errors.Wrap(err, "failed to scan the host file")
	}
	return modified, base, nil
}

func (h *HostsUpdater) writeHosts(modified entries, base []string) error {
	// Write to a temp file, then copy it to the target host file to atomically replace it.
	f, err := os.CreateTemp("", "hosts.testenv")
	if err != nil {
		return errors.Wrap(err, "failed to create a temp file")
	}
	tempfile := f.Name()
	if err := h.writeHostsFile(f, modified, base); err != nil {
		f.Close()
		return err
	}
	f.Close()
	return replaceFile(tempfile, h.hostsfile)
}

func (h *HostsUpdater) writeHostsFile(f *os.File, modified entries, base []string) error {
	w := bufio.NewWriter(f)
	defer w.Flush()

	// First write the base lines that remain unchanged.
	for i, l := range base {
		if i < len(base)-1 { // Avoid an empty line at the end
			l += "\n"
		}
		if _, err := w.WriteString(l); err != nil {
			return errors.Wrap(err, "failed to write the base lines")
		}
	}
	// Then write the host overrides.
	for host, override := range modified {
		if _, err := fmt.Fprintf(w, "\n%v %v %v%v", override.ip, host, label, override.hostname); err != nil {
			return errors.Wrap(err, "failed to write the host overrides")
		}
	}
	return nil
}
