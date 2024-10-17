// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	diagcommon "go.chromium.org/tast-tests/cros/common/network/diag"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/diag"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/testing"
)

type dnsResolverPresentProblem uint32

const (
	// problemNoNameServersFound - IP config has no name servers available
	problemNoNameServersFound dnsResolverPresentProblem = 0
)

type dnsResolverPresentParams struct {
	NameServers     []string
	ExpectedProblem dnsResolverPresentProblem
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DiagFailDNSResolverPresent,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that the DNS resolver present network diagnostic test fails as expected with malformed DNS names",
		Contacts: []string{
			"cros-network-health-team@google.com", // network-health team
			"khegde@chromium.org",                 // test maintainer
			"stevenjb@chromium.org",               // network-health tech lead
		},
		BugComponent: "b:1166446",
		SoftwareDeps: []string{"chrome", "no_qemu"},
		Attr:         []string{"group:mainline"},
		Fixture:      "networkDiagnosticsShillReset",
		Params: []testing.Param{{
			// Note: Shill ignores empty or malformed nameservers, so only non-functional
			// nameservers resulting in a "not found" failure can be tested.
			Name: "name_servers_not_found",
			Val: &dnsResolverPresentParams{
				NameServers:     []string{"0.0.0.0"},
				ExpectedProblem: problemNoNameServersFound,
			},
			ExtraAttr: []string{"informational"},
		}},
	})
}

// DiagFailDNSResolverPresent tests that when the domain name server (DNS) are
// misconfigured that the network routine reports the correct errors.
func DiagFailDNSResolverPresent(ctx context.Context, s *testing.State) {
	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed creating shill manager proxy: ", err)
	}

	params := s.Param().(*dnsResolverPresentParams)
	serviceProps := map[string]interface{}{
		shillconst.ServicePropertyType: "ethernet",
		shillconst.ServicePropertyStaticIPConfig: map[string]interface{}{
			shillconst.IPConfigPropertyNameServers: params.NameServers,
		},
	}

	if _, err := manager.ConfigureServiceForProfile(ctx, shillconst.DefaultProfileObjectPath, serviceProps); err != nil {
		s.Fatal("Failed to configure shill service: ", err)
	}

	if _, err := manager.WaitForServiceProperties(ctx, serviceProps, 5*time.Second); err != nil {
		s.Fatal("Failed to find shill service: ", err)
	}

	mojo := s.FixtValue().(*diag.MojoAPI)
	// After the property change is emitted, Chrome still needs to process it.
	// Since Chrome does not emit a change, poll to test whether the expected
	// problem occurs.
	expectedResult := &diagcommon.RoutineResult{
		Verdict:  diagcommon.VerdictProblem,
		Problems: []uint32{uint32(params.ExpectedProblem)},
	}
	if err := mojo.PollRoutine(ctx, diagcommon.RoutineDNSResolverPresent, expectedResult); err != nil {
		s.Fatal("Failed to poll routine: ", err)
	}
}
