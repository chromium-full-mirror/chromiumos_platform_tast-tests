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
	"chromiumos/tast/errors"
	"chromiumos/tast/local/bundles/cros/lacros/migrate"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/local/policyutil/fixtures"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BackwardMigratePolicy,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test policy triggering of Lacros-to-Ash profile migration",
		BugComponent: "b:1088267",
		Contacts: []string{
			"lacros-team@google.com",
			"vsavu@google.com", // Test author
			"artyomchen@google.com",
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

	cr, err := forwardMigratePolicy(ctx, fdms, s)
	if err != nil {
		if cr != nil {
			cr.Close(ctx)
		}
		s.Fatal("Failed to perform forward migration: ", err)
	}

	err = backwardMigratePolicy(ctx, fdms, cr)
	if err != nil {
		s.Fatal("Failed to perform backward migration: ", err)
	}
}

func forwardMigratePolicy(ctx context.Context, fdms *fakedms.FakeDMS, s *testing.State) (*chrome.Chrome, error) {
	// Start forward migration with policies.
	cr, err := chrome.New(ctx,
		chrome.DMSPolicy(fdms.URL),
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.KeepState(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start ash")
	}
	defer func() {
		if cr != nil {
			cr.Close(ctx)
		}
	}()

	blob := policy.NewBlob()
	blob.AddPolicies([]policy.Policy{
		&policy.LacrosDataBackwardMigrationMode{Val: "keep_all"},
		&policy.LacrosAvailability{Val: "lacros_only"},
	})

	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, blob); err != nil {
		return nil, errors.Wrap(err, "failed to update policies")
	}

	cr.Close(ctx)
	cr = nil

	// Wait for forward migration to finish.
	crForward, err := migrate.Run(ctx, []chrome.Option{
		chrome.DMSPolicy(fdms.URL),
		// By default migrate.Run runs forward migration for chrome.DefaultUser.
		// Passing credentials from fixtures to set up the forward migration more explicitly.
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
	}, []lacrosfixt.Option{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to migrate profile")
	}

	// Verify that Lacros launches properly.
	if err := migrate.VerifyLacrosLaunch(ctx, s, crForward); err != nil {
		return crForward, errors.Wrap(err, "failed to launch lacros")
	}

	return crForward, nil
}

func backwardMigratePolicy(ctx context.Context, fdms *fakedms.FakeDMS, cr *chrome.Chrome) error {
	defer func() {
		if cr != nil {
			cr.Close(ctx)
		}
	}()

	// Start backward migration with policies.
	blob := policy.NewBlob()
	blob.AddPolicies([]policy.Policy{
		&policy.LacrosDataBackwardMigrationMode{Val: "keep_all"},
		&policy.LacrosAvailability{Val: "lacros_disallowed"},
	})

	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, blob); err != nil {
		return errors.Wrap(err, "failed to update policies")
	}

	cr.Close(ctx)
	cr = nil

	// Wait for backward migration to finish.
	crBackward, err := migrate.BackwardRun(ctx, []chrome.Option{
		chrome.DMSPolicy(fdms.URL),
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.ExtraArgs("--vmodule=*=1"),
	})
	if err != nil {
		return errors.Wrap(err, "failed to backward migrate profile")
	}

	crBackward.Close(ctx)

	return nil
}
