// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/lacros/migrate"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
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
			pci.SearchFlag(&policy.DeveloperToolsAvailability{}, pci.Served),
		},
		Data:    []string{"migrate/indexeddb_check.js", "migrate/indexeddb_set.js"},
		Timeout: 3 * time.Minute,
	})
}

func BackwardMigratePolicy(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(*fakedms.FakeDMS)

	crCleanup, err := migrate.StartChromeToClearMigrationState(ctx)
	if err != nil {
		s.Fatal("Failed to run Chrome to clear migration state: ", err)
	}
	crCleanup.Close(ctx)

	crForward, err := forwardMigratePolicy(ctx, fdms, s)
	if err != nil {
		s.Fatal("Failed to perform forward migration: ", err)
	}
	defer func() {
		if err := crForward.Close(ctx); err != nil {
			s.Error("Failed to close ash chrome: ", err)
		}
	}()

	// Setup profile data before the backward migration.
	tconn, err := crForward.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	l, err := lacros.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch lacros: ", err)
	}
	defer func() {
		if l != nil {
			l.Close(ctx)
		}
	}()
	if err := migrate.SetupProfileData(ctx, crForward, s, l.Browser()); err != nil {
		s.Fatal("Failed to set up profile data: ", err)
	}

	l.Close(ctx)
	l = nil

	crBackward, err := backwardMigratePolicy(ctx, fdms, crForward)
	if err != nil {
		s.Fatal("Failed to perform backward migration: ", err)
	}
	defer func() {
		if err := crBackward.Close(ctx); err != nil {
			s.Error("Failed to close ash chrome: ", err)
		}
	}()

	// Restore the browsing session from Lacros before calling `VerifyProfileData()`.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)
	if err := kb.Accel(ctx, "Ctrl+Shift+T"); err != nil {
		s.Fatal("Failed to press Ctrl+Shift+T: ", err)
	}
	if err := migrate.VerifyProfileData(ctx, crBackward, s, crBackward.Browser()); err != nil {
		s.Fatal("Failed to verify: ", err)
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
		// Configuring the DeveloperToolsAvailability explicitly,
		// otherwise extension installation does not work for now.
		// TODO(b/292493121): Remove once the bug is fixed.
		&policy.DeveloperToolsAvailability{Val: 1},
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

func backwardMigratePolicy(ctx context.Context, fdms *fakedms.FakeDMS, cr *chrome.Chrome) (*chrome.Chrome, error) {
	// Start backward migration with policies.
	blob := policy.NewBlob()
	blob.AddPolicies([]policy.Policy{
		&policy.LacrosDataBackwardMigrationMode{Val: "keep_all"},
		&policy.LacrosAvailability{Val: "lacros_disallowed"},
	})

	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, blob); err != nil {
		return nil, errors.Wrap(err, "failed to update policies")
	}

	// Wait for backward migration to finish.
	crBackward, err := migrate.BackwardRun(ctx, []chrome.Option{
		chrome.DMSPolicy(fdms.URL),
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.ExtraArgs("--vmodule=*=1"),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to backward migrate profile")
	}

	return crBackward, nil
}
