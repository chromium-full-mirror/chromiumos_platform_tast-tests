// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package testenv provides common interfaces to manage the test environments
package testenv

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"

	"go.chromium.org/tast/core/errors"
)

// HostsUpdater manages to add or remove host entries in /etc/hosts on a DUT for testing, so
// a host can be resolved to an IP address of your target host. It allows to point your test
// at any desired environments such as preprod.
type HostsUpdater struct {
	hostsfile string
}

type entry struct {
	hostname string
	ip       string
}

// A map of the "from" hosts to the "to" host entries.
type entries map[string]entry

// label is an identifier added to each line of the entries modified by HostsUpdater.
const label = "#testenv-hosts-override="

// NewHostsUpdater creates a HostsUpdater.
// Callers should defer call a returned function to reset any override entries after use.
func NewHostsUpdater() (*HostsUpdater, func() error, error) {
	return newHostsUpdaterInternal("/etc/hosts", true)
}

// newHostsUpdaterInternal is an internal version of NewHostsUpdater() with useful params for testing:
//
//	hostsfile - /etc/hosts (default) or a mock file for tests
//	reset - true (default) to clear any override entries before
func newHostsUpdaterInternal(hostsfile string, reset bool) (*HostsUpdater, func() error, error) {
	h := &HostsUpdater{hostsfile: hostsfile}
	if !reset {
		return h, h.Reset, nil
	}
	if err := h.Reset(); err != nil {
		return nil, nil, errors.Wrap(err, "failed to clean up host overrides before update")
	}
	return h, h.Reset, nil
}

// Override updates /etc/hosts to add a route rule for the given pairs of
// "from" and "to" hosts. It allows tests to point to target hosts in the local
// test environments.
func (h *HostsUpdater) Override(rules ...fromTo) (entries, error) {
	modified, base, err := h.readHosts()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read the hosts for override")
	}

	for _, r := range rules {
		from := string(r.From)
		to := string(r.To)
		// Avoid overriding the same host more than once.
		if val, ok := modified[from]; ok && val.hostname == to {
			return nil, errors.Wrapf(err, "host already overridden from: %v, to: %v", from, to)
		}
		addrs, err := net.LookupHost(to)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to look up host: %v", to)
		}
		// TODO(b/301311992): Refactor the LookupHost in a follow up to check the size of addrs.
		modified[from] = entry{
			hostname: to,
			ip:       addrs[0],
		}
	}

	if err := h.writeHosts(modified, base); err != nil {
		return nil, errors.Wrapf(err, "failed to override host: %v", rules)
	}
	return modified, nil
}

// Reset clears all the host overrides.
func (h *HostsUpdater) Reset() error {
	modified, base, err := h.readHosts()
	if err != nil {
		return errors.Wrap(err, "failed to read the host file to reset")
	}
	if len(modified) == 0 {
		return nil
	}
	// Write only the base lines for reset.
	err = h.writeHosts(entries{}, base)
	if err != nil {
		return errors.Wrap(err, "failed to write the host file for reset")
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
	// TODO(b/301311992): Replace a direct write to /etc/hosts with an atomic copy of a temp file.
	f, err := os.Open(h.hostsfile)
	if err != nil {
		return errors.Wrapf(err, "failed to read the host file: %v", h.hostsfile)
	}
	defer f.Close()

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
