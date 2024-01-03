// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TAPEEnrolledLogin,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Login using TAPE on an enrolled device",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com", // Test author
		},
		BugComponent: "b:1111617", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Policy Stack
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier"},
		Fixture:      fixture.TAPEEnrolled,
		VarDeps:      []string{tape.ServiceAccountVar},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.AllowDinosaurEasterEgg{}, pci.Served),
		},
	})
}

func TAPEEnrolledLogin(ctx context.Context, s *testing.State) {
	fixtData := fixture.TAPEAccountData{}
	if err := s.FixtFillValue(&fixtData); err != nil {
		s.Fatal("Failed to deserialize remote fixture data: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))

	tapePolicies := &tape.AllowDinosaurEasterEggUsers{
		AllowDinosaurEasterEgg: tape.NULLABLEBOOLEAN_FALSE,
	}
	if err := tapeClient.SetPolicy(ctx, tapePolicies, []string{} /*updateMask*/, nil /*additionalTargetKeys*/, fixtData.RequestID); err != nil {
		s.Fatal("Failed to set initial policy: ", err)
	}

	// TODO(b/316326530): Replace withgeneric cleanup in the fixture.
	defer func(ctx context.Context) {
		tapePolicies := &tape.AllowDinosaurEasterEggUsers{
			AllowDinosaurEasterEgg: tape.NULLABLEBOOLEAN_UNSET,
		}
		if err := tapeClient.SetPolicy(ctx, tapePolicies, []string{} /*updateMask*/, nil /*additionalTargetKeys*/, fixtData.RequestID); err != nil {
			s.Fatal("Failed to set initial policy: ", err)
		}
	}(cleanupCtx)

	cr, err := chrome.New(ctx,
		chrome.GAIALogin(chrome.Creds{User: fixtData.Username, Pass: fixtData.Password}),
		chrome.KeepEnrollment(),
	)
	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	expectedPolicies := []policy.Policy{
		&policy.AllowDinosaurEasterEgg{Stat: policy.StatusSet, Val: false},
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create a TestAPIConn: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return policyutil.Verify(ctx, tconn, expectedPolicies)
	}, &testing.PollOptions{Timeout: 10 * time.Minute}); err != nil {
		s.Fatal("Failed to verify initial policy: ", err)
	}
}
