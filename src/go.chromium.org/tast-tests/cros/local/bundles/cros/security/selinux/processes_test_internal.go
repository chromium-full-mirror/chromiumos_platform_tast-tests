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

// ProcessTestCaseSelector specifies what kind of test cases will be run.
type ProcessTestCaseSelector int

const (
	// Stable to run test cases proven to be stable.
	Stable ProcessTestCaseSelector = iota
	// Unstable to run newly introduced test cases or flaky cases.
	Unstable
)

const domainIsolationErrorMessage = "THIS IS A SECURITY BUG. Follow steps 1~3 of https://chromium.googlesource.com/chromiumos/docs/+/HEAD/security/selinux.md#Writing-SELinux-policy-for-a-daemon to create a permissive domain for the daemon."

// ProcessesTestInternal runs the test suite for SELinuxProcesses(Experimental|Informational)?
func ProcessesTestInternal(ctx context.Context, s *testing.State, testSelector []ProcessTestCaseSelector) {
	type processSearchType int
	const (
		exe        processSearchType = iota // absolute executable path
		notExe                              // not absolute executable path
		cmdline                             // partial regular expression matched against command line
		notCmdline                          // not matching the regular expression
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

	ps, err := GetProcesses(PrivilegedOnly)
	if err != nil {
		s.Fatal("Failed to get processes: ", err)
	}

	testCases := make([]testCaseType, 0)
	for _, sel := range testSelector {
		switch sel {
		case Stable:
			testCases = append(testCases, []testCaseType{
				{exe, "/sbin/minijail0", matchRegexp, "(minijail|.*_minijail0|cros_.*_minijail)", ""},

				{exe, "/usr/bin/metrics_daemon", matchRegexp, "cros_metrics_daemon", ""},

				{exe, "/usr/sbin/cryptohomed", matchRegexp, "cros_cryptohomed", ""},
				{exe, "/usr/sbin/chapsd", matchRegexp, "cros_chapsd", ""},
				{exe, "/usr/sbin/hpsd", matchRegexp, "cros_hpsd", ""},
				{exe, "/usr/sbin/tcsd", matchRegexp, "cros_tcsd", ""},

				// moblab, autotest, devserver, rotatelogs, apache2, envoy, containerd are all required for
				// normal operation of moblab devices.
				// python3 is for crbug.com/1151463.
				// mkdir is for crbug.com/1156295.
				{notCmdline, ".*(frecon|agetty|ping|recover_dts|udevadm|update_rw_vpd|mosys|vpd|flashrom|moblab|autotest|devserver|rotatelogs|apache2|envoy|containerd|python3|mkdir).*", notString, "chromeos", domainIsolationErrorMessage},
				{notCmdline, ".*(frecon|agetty|ping|recover_duts).*", notString, "unconfined_proc", domainIsolationErrorMessage},
				// python3.6m is for nebraska.py (b/247248201).
				{notExe, "(/sbin/init|/bin/bash|/usr/local/bin/python3.6m)", notString, "cros_init", domainIsolationErrorMessage},
				// coreutils and ping are excluded for recover_duts scripts.
				// logger is common to redirect output widely used from init conf scripts.
				{notExe, "(/bin/([db]a)?sh|/usr/bin/coreutils|/usr/bin/logger|/bin/ping|brcm_patchram_plus)", notString, "cros_init_scripts", domainIsolationErrorMessage},
				{notExe, "/sbin/minijail0", notString, "minijail", domainIsolationErrorMessage},
			}...)
		case Unstable:
			testCases = append(testCases, []testCaseType{
				{exe, "/sbin/minijail0", matchRegexp, "(minijail|.*_minijail0|cros_.*_minijail)", ""},
				{notExe, "(/bin/([db]a)?sh|/usr/bin/coreutils|/usr/bin/logger)", notString, "cros_init_scripts", domainIsolationErrorMessage},
				{notExe, "(/sbin/init|/bin/bash)", notString, "cros_init", domainIsolationErrorMessage},
				{notCmdline, ".*(ping|frecon|agetty|recover_duts).*", notString, "chromeos", domainIsolationErrorMessage},
				{cmdline, ".*", notString, "unconfined_proc", domainIsolationErrorMessage},
			}...)
		}
	}

	for _, testCase := range testCases {
		var p []Process
		var err error
		switch testCase.field {
		case exe:
			p, err = FindProcessesByExe(ps, testCase.query, false)
		case notExe:
			p, err = FindProcessesByExe(ps, testCase.query, true)
		case cmdline:
			p, err = FindProcessesByCmdline(ps, testCase.query, false)
		case notCmdline:
			p, err = FindProcessesByCmdline(ps, testCase.query, true)
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
