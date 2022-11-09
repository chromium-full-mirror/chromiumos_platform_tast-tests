// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/bundles/cros/lacros/migrate"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/local/policyutil/fixtures"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BackwardMigratePolicy,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test policy triggering of Lacros-to-Ash profile migration",
		Contacts: []string{
			"vsavu@google.com", // Test author
			"lacros-team@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "lacros"},
		Fixture:      fixture.FakeDMS,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.LacrosDataBackwardMigrationMode{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityJS),
		},
	})
}

func BackwardMigratePolicy(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(*fakedms.FakeDMS)

	if err := migrate.ClearMigrationState(ctx); err != nil {
		s.Fatal("Failed to run Chrome to clear migration state: ", err)
	}

	forwardMigratePolicy(ctx, fdms, s)
	backwardMigratePolicy(ctx, fdms, s)
}

func forwardMigratePolicy(ctx context.Context, fdms *fakedms.FakeDMS, s *testing.State) {
	blob := policy.NewBlob()
	blob.AddPolicies([]policy.Policy{
		&policy.LacrosDataBackwardMigrationMode{Val: "keep_all"},
		&policy.LacrosAvailability{Val: "lacros_only"},
	})

	if err := fdms.WritePolicyBlob(blob); err != nil {
		s.Fatal("Failed to write policy blob: ", err)
	}

	cr, err := chrome.New(ctx,
		chrome.DMSPolicy(fdms.URL),
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.KeepState(),
	)
	if err != nil {
		s.Fatal("Failed to start ash: ", err)
	}

	// Make sure to always close Chrome.
	defer func() {
		if cr != nil {
			if err := cr.Close(ctx); err != nil {
				s.Error("Failed to close Chrome: ", err)
			}
		}
	}()

	if err := policyutil.RefreshChromePolicies(ctx, cr); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	if err := cr.Close(ctx); err != nil {
		s.Fatal("Failed to close Chrome: ", err)
	}

	cr, err = chrome.New(ctx,
		chrome.DMSPolicy(fdms.URL),
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.KeepState(),
	)
	if err != nil {
		s.Fatal("Failed to start ash: ", err)
	}

	blob = policy.NewBlob()
	blob.AddPolicies([]policy.Policy{
		&policy.LacrosDataBackwardMigrationMode{Val: "keep_all"},
		&policy.LacrosAvailability{Val: "lacros_disallowed"},
	})

	if err := fdms.WritePolicyBlob(blob); err != nil {
		s.Fatal("Failed to write policy blob: ", err)
	}

	if err := policyutil.RefreshChromePolicies(ctx, cr); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	if err := cr.Close(ctx); err != nil {
		s.Fatal("Failed to close Chrome: ", err)
	}
	cr = nil
}

func backwardMigratePolicy(ctx context.Context, fdms *fakedms.FakeDMS, s *testing.State) {
	cr, err := migrate.BackwardRun(ctx, []chrome.Option{
		chrome.DMSPolicy(fdms.URL),
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.ExtraArgs("--vmodule=*=1"),
	})
	if err != nil {
		s.Fatal("Failed to backward migrate profile: ", err)
	}

	defer cr.Close(ctx)
}
