// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/proxy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SystemProxyForSystemServices,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that tlsdated can successfully connect to a web endpoint through the system-proxy daemon",
		Contacts: []string{
			"chromeos-commercial-networking@google.com", // Team
			"acostinas@google.com",                      // Test author
		},
		BugComponent: "b:1000044",
		SoftwareDeps: []string{"reboot", "chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			//"group:cq-medium",
		},
		Fixture: "chromeEnrolledLoggedIn",
		Timeout: 5 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ProxyMode{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.ProxyServer{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.SystemProxySettings{}, pci.VerifiedFunctionalityOS),
			{
				Key: "feature_id",
				// Verify that traffic from the device goes through the managed proxy (COM_FOUND_CUJ6_TASK3_WF1).
				Value: "screenplay-2e4de0e2-ea40-4348-9bbc-f518c68484b9",
			},
		},
	})
}

func SystemProxyForSystemServices(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	const username = "testuser"
	const password = "testpwd"

	// Start an HTTP proxy instance on the DUT which requires username and password authentication.
	ps := proxy.NewServer()
	defer ps.Stop(ctx)

	cred := &proxy.AuthCredentials{Username: username, Password: password}
	err := ps.Start(ctx, 3128, cred, []string{})
	if err != nil {
		s.Fatal("Failed to start a local proxy on the DUT: ", err)
	}

	// Configure the proxy on the DUT via policy to point to the local proxy instance started via the `ProxyService`.
	proxyModePolicy := &policy.ProxyMode{Val: "fixed_servers"}
	proxyServerPolicy := &policy.ProxyServer{Val: fmt.Sprintf("http://%s", ps.HostAndPort)}

	// Start system-proxy and configure it with the credentials of the local proxy instance.
	systemProxySettingsPolicy := &policy.SystemProxySettings{
		Val: &policy.SystemProxySettingsValue{
			SystemProxyEnabled:           true,
			SystemServicesUsername:       username,
			SystemServicesPassword:       password,
			PolicyCredentialsAuthSchemes: []string{},
		}}

	if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
		s.Fatal("Failed to clean up: ", err)
	}

	// Update policies.
	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{proxyModePolicy, proxyServerPolicy, systemProxySettingsPolicy}); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	if err = waitForSignal(ctx); err != nil {
		s.Fatal("Failed to observer event: ", err)
	}

	// It may take some time for Chrome to process the system-proxy worker active signal.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := runTLSDate(ctx); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Interval: 500 * time.Millisecond, Timeout: 15 * time.Second}); err != nil {
		s.Fatal("Not all targets finished closing: ", err)
	}
}

// runTLSDate runs tlsdate once, in the foreground. Returns an error if tlsdate didn't use system-proxy to connect to the web
// endpoint or if the certificate verification failed.
func runTLSDate(ctx context.Context) error {
	// tlsdated is a CrOS daemon that runs the tlsdate binary periodically in the background and does proxy resolution through Chrome.
	// The `-m <n>` option means tlsdate should run at most once every n seconds in steady state
	// The `-p` option means dry run.
	// The `-o` option means exit tlsdated after running once
	out, err := testexec.CommandContext(ctx, "/usr/bin/tlsdated", "-o", "-p", "-m", "60", "--", "/usr/bin/tlsdate", "-v", "-C", "/usr/share/chromeos-ca-certificates", "-l").CombinedOutput()

	//  The exit code 124 indicates that timeout sent a SIGTERM to terminate tlsdate.
	if err != nil && !strings.Contains(err.Error(), "Process exited with status 124") {
		return errors.Wrap(err, "error running tlsdate")
	}
	var result = string(out)
	// system-proxy has an address in the 100.115.92.0/24 subnet (assigned by patchpanel) and listens on port 3128.
	proxyMsg := regexp.MustCompile("V: using proxy http://100.115.92.[0-9]+:3128")
	const successMsg = "V: certificate verification passed"

	if !proxyMsg.Match(out) {
		return errors.Errorf("tlsdated is not using the system-proxy daemon: %s", result)
	}

	if !strings.Contains(result, successMsg) {
		return errors.Errorf("certificate verification failed: %s", result)
	}

	return nil
}

func waitForSignal(ctx context.Context) error {
	match := dbusutil.MatchSpec{
		Type:      "signal",
		Path:      "/org/chromium/SystemProxy",
		Interface: "org.chromium.SystemProxy",
		Member:    "WorkerActive",
	}
	signal, err := dbusutil.NewSignalWatcherForSystemBus(ctx, match)
	defer signal.Close(ctx)

	return err
}
