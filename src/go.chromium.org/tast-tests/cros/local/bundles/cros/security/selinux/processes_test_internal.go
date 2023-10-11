// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package selinux

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const domainIsolationErrorMessage = "THIS IS A SECURITY BUG. Follow steps 1~3 of https://chromium.googlesource.com/chromiumos/docs/+/HEAD/security/selinux.md#Writing-SELinux-policy-for-a-daemon to create a permissive domain for the daemon."

// ProcessesTestInternal runs the test suite for SELinuxProcesses.
func ProcessesTestInternal(ctx context.Context, s *testing.State) {
	type processSearchType int
	const (
		exe     processSearchType = iota // absolute executable path
		cmdline                          // partial regular expression matched against command line
	)
	type contextMatchType int
	const (
		matchRegexp contextMatchType = iota // matches a regexp positively
		notString                           // does not include string
	)
	type testCaseType struct {
		field        processSearchType // field to search for processes
		query        string            // search keyword for given field
		contextMatch contextMatchType  // how to match the expected SELinux context (domain)
		context      string            // expected SELinux process context (domain).
		errorMsg     string            // an optional error message that may help developers understand why it fails or how to fix.
	}

	assertContext := func(processes []Process, testCase testCaseType) {
		for _, proc := range processes {
			var errorLine strings.Builder
			fmt.Fprintf(&errorLine, "Process %+v has context %q", proc, proc.SEContext)

			switch testCase.contextMatch {
			case matchRegexp:
				expectedContext, err := ProcessContextRegexp(testCase.context)
				if err != nil {
					s.Errorf("Failed to compile expected context %q: %v", testCase.context, err)
					return
				}
				if !expectedContext.MatchString(proc.SEContext) {
					fmt.Fprintf(&errorLine, "; want %q", expectedContext)
					if testCase.errorMsg != "" {
						fmt.Fprintf(&errorLine, "; %v", testCase.errorMsg)
					}
					s.Error(errorLine.String())
				}
			case notString:
				if strings.Contains(proc.SEContext, ":"+testCase.context+":") {
					fmt.Fprintf(&errorLine, "; expected to have its own SELinux domain and not %q", testCase.context)
					if testCase.errorMsg != "" {
						fmt.Fprintf(&errorLine, "; %v", testCase.errorMsg)
					}
					s.Error(errorLine.String())
				}
			default:
				s.Errorf("%+v has invalid contextMatchType %d", testCase, int(testCase.contextMatch))
			}
		}
	}

	ps, err := GetProcesses(All)
	if err != nil {
		s.Fatal("Failed to get processes: ", err)
	}

	testCases := []testCaseType{
		// Check SELinux enforced domains.
		{exe, "/bin/lsblk", matchRegexp, "cros_lsblk", ""},
		{exe, "/lib/udev/hps-dev.sh", matchRegexp, "cros_hps_udev", ""},
		{exe, "/sbin/featured", matchRegexp, "cros_featured", ""},
		{exe, "/sbin/is_running_from_installer", matchRegexp, "cros_is_running_from_installer", ""},
		{exe, "/usr/bin/cros_installer", matchRegexp, "cros_installer", ""},
		{exe, "/usr/bin/hps-factory", matchRegexp, "cros_hps_factory", ""},
		{exe, "/usr/bin/metrics_daemon", matchRegexp, "cros_metrics_daemon", ""},
		{exe, "/usr/bin/periodic_scheduler", matchRegexp, "cros_periodic_scheduler", ""},
		{exe, "/usr/bin/rootdev", matchRegexp, "cros_rootdev", ""},
		{exe, "/usr/sbin/chromeos-postinst", matchRegexp, "cros_chromeos_postinst", ""},
		{exe, "/usr/sbin/bootstat", matchRegexp, "cros_bootstat", ""},
		{exe, "/usr/sbin/chapsd", matchRegexp, "cros_chapsd", ""},
		{exe, "/usr/sbin/cryptohomed", matchRegexp, "cros_cryptohomed", ""},
		{exe, "/usr/sbin/cryptohome-namespace-mounte", matchRegexp, "cros_cryptohome_namespace_mounter", ""},
		{exe, "/usr/sbin/hpsd", matchRegexp, "cros_hpsd", ""},
		{exe, "/usr/sbin/os_install_service", matchRegexp, "cros_os_install_service", ""},
		{exe, "/usr/sbin/parted", matchRegexp, "cros_parted", ""},
		{exe, "/usr/sbin/spaced", matchRegexp, "cros_spaced", ""},
		{exe, "/usr/sbin/tcsd", matchRegexp, "cros_tcsd", ""},
		// Check other below.
	}

	for _, testCase := range testCases {
		var p []Process
		var err error
		switch testCase.field {
		case exe:
			p, err = FindProcessesByExe(ps, testCase.query, false)
		case cmdline:
			p, err = FindProcessesByCmdline(ps, testCase.query, false)
		default:
			err = errors.Errorf("%+v has invalid processSearchType %d", testCase, int(testCase.field))
		}
		if err != nil {
			s.Error("Failed to find processes: ", err)
			continue
		}
		// Check that the processes are running with the right context.
		assertContext(p, testCase)
	}
}
