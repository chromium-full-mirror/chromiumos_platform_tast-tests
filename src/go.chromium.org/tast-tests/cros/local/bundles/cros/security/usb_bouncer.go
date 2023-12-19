// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package security

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/security/seccomp"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         USBBouncer,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that usb_bouncer works as intended",
		SoftwareDeps: []string{"chrome", "usbguard"},
		BugComponent: "b:1048474", // ChromeOS > Security > Usb_bouncer/Usbguard
		Contacts: []string{
			"chromeos-security@google.com",
			"allenwebb@chromium.org",
			"jorgelo@chromium.org",
		},
		Params: []testing.Param{{
			Name:      "check_seccomp_with_chrome",
			ExtraAttr: []string{"group:mainline", "informational"},
			Fixture:   fixture.ChromeLoggedIn,
			Val:       checkSeccompWithChrome,
		}, {
			Name:      "check_seccomp_without_chrome",
			ExtraAttr: []string{"group:mainline", "informational"},
			Val:       checkSeccompWithoutChrome,
		}, {
			Name:    "generate_seccomp_with_chrome",
			Fixture: fixture.ChromeLoggedIn,
			Val:     generateSeccompWithChrome,
		}, {
			Name: "generate_seccomp_without_chrome",
			Val:  generateSeccompWithoutChrome,
		}},
	})
}

type testType string

const (
	checkSeccompWithChrome       testType = "check_seccomp_with_chrome"
	checkSeccompWithoutChrome    testType = "check_seccomp_without_chrome"
	generateSeccompWithChrome    testType = "generate_seccomp_with_chrome"
	generateSeccompWithoutChrome testType = "generate_seccomp_without_chrome"
)

func enforceSeccomp(tt testType) bool {
	if tt == generateSeccompWithChrome || tt == generateSeccompWithoutChrome {
		return false
	}
	return true
}

func withChrome(tt testType) bool {
	if tt == checkSeccompWithChrome || tt == generateSeccompWithChrome {
		return true
	}
	return false
}

// pathOfTestDevice finds a path to a USB device in /sys/devices. It returns an empty string on
// failure.
func pathOfTestDevice() (string, error) {
	var devPath string
	if err := filepath.Walk("/sys/devices/", func(path string, info os.FileInfo, err error) error {
		if len(devPath) > 0 {
			return filepath.SkipDir
		}
		suffix := "/authorized"
		if strings.HasSuffix(path, suffix) {
			devPath = path[len("/sys") : len(path)-len(suffix)]
			return filepath.SkipDir
		}
		return nil
	}); err != nil {
		return "", errors.Wrap(err, "unable to get a test device path")
	}
	return devPath, nil
}

func testUsbBouncer(ctx context.Context, s *testing.State, m *seccomp.PolicyGenerator,
	devPath string, withChrome, enforceSeccomp bool) {
	cases := [][]string{
		{"udev", "add", devPath},
		{"cleanup"},
		{"udev", "remove", devPath},
		{"genrules"},
	}

	if withChrome {
		cases = append(cases, []string{"userlogin"})
	}

	for _, c := range cases {
		f, err := ioutil.TempFile(s.OutDir(), "strace-usb_bouncer")
		if err != nil {
			s.Error("TempFile failed: ", err)
			continue
		}
		if err := f.Close(); err != nil {
			s.Error("Close() failed: ", err)
			continue
		}
		logFile := f.Name()

		if !enforceSeccomp {
			c = append(c, "--seccomp=false")
		}

		cmd := seccomp.CommandContext(ctx, logFile, "usb_bouncer", c...)
		if err := cmd.Run(testexec.DumpLogOnError); err != nil {
			s.Fatalf("%q failed with %v", cmd.Args, err)
		}
		if err := m.AddStraceLog(logFile, seccomp.ExcludeSyscallsBeforeSandboxing); err != nil {
			s.Error()
		}
	}
}

func USBBouncer(ctx context.Context, s *testing.State) {
	const (
		defaultUser = "testuser@gmail.com"
		defaultPass = "testpass"
	)

	d, err := pathOfTestDevice()
	if err != nil {
		s.Fatal("Unable to find a suitable test USB device: ", err)
	}
	if len(d) == 0 {
		s.Fatal("Unable to find a suitable test USB device")
	}

	// Move the current state to a temporary location and restore it after the test. This ensures the
	// codepaths that create the state directory and file are exercised.
	globalStateDir := "/run/usb_bouncer"
	stashedGlobalStateDir := "/run/usb_bouncer.orig"
	if err = os.Rename(globalStateDir, stashedGlobalStateDir); err == nil {
		defer func() {
			if err := os.RemoveAll(globalStateDir); err != nil {
				s.Errorf("RemoveAll(%q) failed: %v", globalStateDir, err)
			}
			if err := os.Rename(stashedGlobalStateDir, globalStateDir); err != nil {
				s.Errorf("Rename(%q, %q) failed: %v", stashedGlobalStateDir, globalStateDir, err)
			}
		}()
	} else if !os.IsNotExist(err) {
		s.Fatalf("Failed to stash %q: %v", globalStateDir, err)
	}

	// Clear any usb_bouncer files from the test user's daemon-store. The sub directories need to be
	// preserved.
	userStateDir := "/run/daemon-store/usb_bouncer"
	if err := filepath.Walk(userStateDir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			return os.Remove(path)
		} else if info.Name() == "device-db" {
			// Chmod to 770 to trigger brillo::SafeFD::Rmdir at least once for seccomp coverage.
			os.Chmod(path, 770)
		}
		return nil
	}); err != nil {
		s.Fatalf("Failed to cleanup %q: %v", userStateDir, err)
	}

	tt := s.Param().(testType)

	m := seccomp.NewPolicyGenerator()
	testUsbBouncer(ctx, s, m, d, withChrome(tt) /*withChrome*/, enforceSeccomp(tt) /*withSeccomp*/)

	policyFile := filepath.Join(s.OutDir(), "usb_bouncer.policy")
	if err := ioutil.WriteFile(policyFile, []byte(m.GeneratePolicy()), 0644); err != nil {
		s.Fatal("Failed to record seccomp policy: ", err)
	}
	s.Logf("Wrote usb_bouncer seccomp policy to %q", policyFile)
}
