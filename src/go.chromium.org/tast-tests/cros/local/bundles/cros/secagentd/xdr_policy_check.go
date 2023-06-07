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

func setXdrPolicy(ctx context.Context, s *testing.State, v bool, t string) uint64 {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
		s.Fatal("Failed to clean up before updating policy: ", err)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{&policy.DeviceReportXDREvents{Val: v}})
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
		upstart.WithArg("SET_HEARTBEAT_PERIOD_S_FOR_TESTING", t),
		upstart.WithArg("PLUGIN_BATCH_INTERVAL_S_FOR_TESTING", t))
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
			name:          "xdr_policy_enabled",
			policy:        true,
			expectEnqueue: true,
		},
		{
			name:          "xdr_policy_disabled",
			policy:        false,
			expectEnqueue: false,
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			const batchIntervalS = 1
			agentPid := setXdrPolicy(ctx, s, param.policy, strconv.Itoa(batchIntervalS))
			stop, err := secagentddbusmonitor.SetupDbusMonitor(ctx, agentPid)
			if err != nil {
				s.Fatal("Failed to setup dbus monitoring: ", err)
			}

			// Start an arbitrary process that exits instantaneously. This will
			// cause Process events to be emitted if permitted by policy.
			cmd := testexec.CommandContext(ctx, "/bin/echo")
			cmd.Wait()
			// GoBigSleepLint: Small grace period for the events to be
			// processed and emitted by secagentd.
			// TODO(b/278252387): Convert this to poll when tast's
			// dbusutil.DbusEventMonitor supports it.
			testing.Sleep(ctx, 2*batchIntervalS*time.Second)

			calledMethods, err := stop()
			if err != nil {
				s.Fatal("Failed to capture EnqueueRecord dbus calls to missive: ", err)
			}
			s.Logf("secagentd enqueued %d events", len(calledMethods))
			if param.expectEnqueue && len(calledMethods) == 0 {
				s.Fatal("secagentd unexpectedly failed to enqueue any events to missive")
			} else if !param.expectEnqueue && len(calledMethods) != 0 {
				s.Fatalf("secagentd unexpectedly enqueued %d events to missive", len(calledMethods))
			}
		})
	}
}
