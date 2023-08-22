// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentd tests security event reporting functionality of the
// secagentd daemon.
package secagentd

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentddbusmonitor"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdupstart"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: XdrPolicyCheck,
		Desc: "Checks that the daemon adheres to the XDR reporting policy",
		Contacts: []string{
			"cros-enterprise-security@google.com",
			"aashay@google.com",
			"jasonling@google.com",
			"rborzello@google.com",
		},
		// ChromeOS > Security > ChromeOS Enterprise Security
		BugComponent: "b:1208373",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:enterprise-reporting",
		},
		Timeout:      3 * time.Minute,
		Fixture:      fixture.ChromeEnrolledLoggedIn,
		SoftwareDeps: []string{"reboot", "bpf", "chrome", "boot_perf_info"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceReportXDREvents{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func setXdrPolicy(ctx context.Context, s *testing.State, policyEnabled bool, timer string) uint64 {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
		s.Fatal("Failed to clean up before updating policy: ", err)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{&policy.DeviceReportXDREvents{Val: policyEnabled}})
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to update policies. Make sure Chrome has API keys and apply go/pavolshack if running on a VM: ", err)
	}
	// Restart secagentd. Don't pass in the flag that would override policy
	// checks. But do override the wait for missive to successfully enqueue
	// an event. Bypassing this wait will make secagentd emit more than one
	// event and will greatly reduce the chance of a flake. Similarly, reduce
	// some internal poll delays to emit more events sooner.
	agentPid, err := secagentdupstart.RestartSecagentd(ctx,
		upstart.WithArg("BYPASS_ENQ_OK_WAIT_FOR_TESTING", "true"),
		upstart.WithArg("SET_HEARTBEAT_PERIOD_S_FOR_TESTING", timer),
		upstart.WithArg("PLUGIN_BATCH_INTERVAL_S_FOR_TESTING", timer))
	if err != nil {
		s.Fatal("Failed to restart secagentd: ", err)
	}
	return agentPid
}

func XdrPolicyCheck(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 30*time.Second)
	defer func(ctx context.Context, s *testing.State) {
		cr := s.FixtValue().(chrome.HasChrome).Chrome()
		fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
		policyutil.ResetChrome(ctx, fdms, cr)
		upstart.RestartJob(ctx, "secagentd")
		cancel()
	}(cleanupCtx, s)

	for _, param := range []struct {
		name          string
		policy        bool
		expectEnqueue bool
	}{
		{
			name:   "xdr_policy_enabled",
			policy: true,
		},
		{
			name:   "xdr_policy_disabled",
			policy: false,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			const batchIntervalS = 1
			agentPid := setXdrPolicy(ctx, s, param.policy, strconv.Itoa(batchIntervalS))

			ew, cancel, err := secagentddbusmonitor.SetupDbusWatcherWithTimeout(ctx, agentPid, 30*time.Second)
			if err != nil {
				s.Fatal("Failed to setup dbus monitoring: ", err)
			}
			defer cancel()

			// Start an arbitrary process that exits instantaneously. This will
			// cause Process events to be emitted if permitted by policy.
			cmd := testexec.CommandContext(ctx, "/bin/echo")
			cmd.Wait()

			_, ok := <-ew.Events()
			if ok != param.policy {
				if param.policy {
					s.Fatal("Timed out waiting for expected events")
				} else {
					s.Fatal("secagentd unexpectedly enqueued events to missive")
				}
			}
		})
	}
}
