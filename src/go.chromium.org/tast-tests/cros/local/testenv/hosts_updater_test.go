// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.chromium.org/tast/core/errors"
)

var (
	mockHostsBase = []byte(`
# etc/hosts: mock file
# IPv4 and IPv6 localhost aliases
127.0.0.1       localhost
::1             localhost`)

	mockHostsStale = []byte(`
# etc/hosts: mock file
# IPv4 and IPv6 localhost aliases
127.0.0.1       localhost
::1             localhost
10.0.0.1 foo.bar #testenv-hosts-override=preprod-foo.bar
10.0.0.2 foo.bar.baz.qux #testenv-hosts-override=preprod-foo.bar.baz.qux`)

	mockHostsOverride = []byte(`
# etc/hosts: mock file
# IPv4 and IPv6 localhost aliases
127.0.0.1       localhost
::1             localhost
10.0.0.10 foo.bar.baz #testenv-hosts-override=preprod-foo.bar.baz`)
)

type mockResolver struct{}

func (mockResolver) LookupHost(hostname string) ([]string, error) {
	switch hostname {
	case "preprod-foo.bar":
		return []string{"10.0.0.1"}, nil
	case "preprod-foo.bar.baz.qux":
		return []string{"10.0.0.2"}, nil
	case "preprod-foo.bar.baz":
		return []string{"10.0.0.10", "10.0.0.11"}, nil
	}
	return []string{}, errors.Errorf("Undefined host in mock resolver: %v", hostname)
}

func createMockHostFile(path string, data []byte) (string, error) {
	// Create a temp dir and a test host file
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return "", err
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func TestHostsUpdater(t *testing.T) {
	// Given a new host file with no override
	d := t.TempDir()
	hostsfile, err := createMockHostFile(filepath.Join(d, "hosts"), mockHostsBase)
	if err != nil {
		t.Fatalf("Failed to set up a mock host file: %v", err)
	}

	// Initialize HostsUpdater using new() for unit testing
	mockHosts, cleanup, err := newHostsUpdaterInternal(hostsfile, true, false, mockResolver{})
	if err != nil {
		t.Fatalf("Failed to instantiate a HostsUpdater: %v", err)
	}
	defer cleanup()

	// Override a new host to an alternate host.
	rule := fromTo{
		from: hostInfo{hostname: "foo.bar.baz", label: "foobarbaz-prod"},
		to:   hostInfo{hostname: "preprod-foo.bar.baz", label: "foobarbaz-preprod"},
	}
	expectedAddr := "10.0.0.10"
	entities, err := mockHosts.Override(rule)
	actualAddr := entities[string(rule.from.hostname)].ip
	if err != nil {
		t.Fatalf("Failed to override, from: %v, to:%v, %v", rule.from.label, rule.to.label, err)
	}

	// Verify that the host file is updated with the host changes.
	got, err := os.ReadFile(hostsfile)
	if err != nil {
		t.Fatalf("Failed to read the host file: %v", err)
	}
	if diff := cmp.Diff(expectedAddr, actualAddr); diff != "" {
		t.Errorf("Failed to look up %v, got: %v, want: %v", rule.to.label, actualAddr, expectedAddr)
	}
	if diff := cmp.Diff(mockHostsOverride, got); diff != "" {
		t.Fatalf("Failed to override hosts:\ndiff: %v\ngot: %v\nwant: %v", string(diff), string(got), string(mockHostsOverride))
	}
}

func TestHostsUpdater_Reset(t *testing.T) {
	// Given the stale host file with the overridden entities
	d := t.TempDir()
	hostsfile, err := createMockHostFile(filepath.Join(d, "hosts"), mockHostsStale)
	if err != nil {
		t.Fatalf("Failed to set up a mock host file: %v", err)
	}
	// HostsUpdater should reset any stale entities when reset = true is passed to new().
	_, cleanup, err := newHostsUpdaterInternal(hostsfile, true, false, mockResolver{})
	if err != nil {
		t.Fatalf("Failed to instantiate a HostsUpdater: %v", err)
	}
	defer cleanup()

	// Verify that the overrides that had existed before are removed.
	got, err := os.ReadFile(hostsfile)
	if err != nil {
		t.Fatalf("Failed to read the host file: %v", err)
	}
	if diff := cmp.Diff(mockHostsBase, got); diff != "" {
		t.Fatalf("Failed to clear the entities modified before, diff: %v", diff)
	}
}

func TestHostsUpdater_DuplicateOverrideFailure(t *testing.T) {
	// Given the stale host file with the overridden entities
	d := t.TempDir()
	hostsfile, err := createMockHostFile(filepath.Join(d, "hosts"), mockHostsBase)
	if err != nil {
		t.Fatalf("Failed to set up a mock host file: %v", err)
	}
	// HostsUpdater should reset any stale entities.
	mockHosts, cleanup, err := newHostsUpdaterInternal(hostsfile, true, false, mockResolver{})
	if err != nil {
		t.Fatalf("Failed to instantiate a HostsUpdater: %v", err)
	}
	defer cleanup()

	// Override a new host to an alternate host twice. The second try should fail.
	rule := fromTo{
		from: hostInfo{hostname: "foo.bar.baz", label: "foobarbaz-prod"},
		to:   hostInfo{hostname: "preprod-foo.bar.baz", label: "foobarbaz-preprod"},
	}
	if _, err = mockHosts.Override(rule); err != nil {
		t.Fatalf("Failed to override, from: %v, to:%v, %v", rule.from.label, rule.to.label, err)
	}
	if _, err = mockHosts.Override(rule); err == nil {
		t.Fatal("Override the same host and target just once to avoid any conflicts in /etc/hosts")
	}
}
