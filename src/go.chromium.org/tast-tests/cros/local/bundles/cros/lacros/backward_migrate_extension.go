// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/lacros/migrate"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BackwardMigrateExtension,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test basic functionality of Lacros-to-Ash profile migration",
		BugComponent: "b:1088267",
		Contacts: []string{
			"lacros-team@google.com",
			"ythjkt@google.com", // Test author
			"artyomchen@google.com",
		},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "lacros"},
	})
}

// BackwardMigrateExtension tests forward and then backward lacros migration with an extension installed from CWS.
// - Clear migration state.
// - Install an extension on Ash from Chrome Web Store (CWS).
// - Run forward migration.
// - Run backward migration.
// - Confirm that the extension is present on Ash.
func BackwardMigrateExtension(ctx context.Context, s *testing.State) {
	if err := setupAshProfileWithExtensionForBackwardMigration(ctx); err != nil {
		s.Fatal("Failed to setup Ash with an extension: ", err)
	}

	if err := migrateAndVerifyLacrosLaunch(ctx, s); err != nil {
		s.Fatal("Failed to migrate and verify Lacros launch: ", err)
	}

	crBackward, err := migrate.BackwardRun(ctx,
		[]chrome.Option{chrome.EnableFeatures("LacrosProfileBackwardMigration")})
	if err != nil {
		s.Fatal("Failed to backward migrate profile: ", err)
	}
	defer crBackward.Close(ctx)

	if err := verifyAshProfileWithExtension(ctx, crBackward); err != nil {
		s.Fatal("verifyAshProfileWithExtension failed: ", err)
	}
}

func setupAshProfileWithExtensionForBackwardMigration(ctx context.Context) error {
	cr, err := migrate.StartChromeToClearMigrationState(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to run Chrome to clear migration state")
	}
	defer cr.Close(ctx)

	if err := policyutil.EnsureGoogleCookiesAccepted(ctx, cr.Browser()); err != nil {
		return errors.Wrap(err, "failed to accept cookies")
	}

	return migrate.SetupExtension(ctx, cr, cr.Browser())
}

func migrateAndVerifyLacrosLaunch(ctx context.Context, s *testing.State) error {
	cr, err := migrate.Run(ctx, []chrome.Option{}, []lacrosfixt.Option{})
	if err != nil {
		return errors.Wrap(err, "failed to forward migrate Ash profile")
	}
	defer cr.Close(ctx)
	if err := migrate.VerifyLacrosLaunch(ctx, s, cr); err != nil {
		return errors.Wrap(err, "failed to launch lacros")
	}

	return nil
}

func verifyAshProfileWithExtension(ctx context.Context, cr *chrome.Chrome) error {
	if err := migrate.VerifyExtension(ctx, cr, cr.Browser()); err != nil {
		return errors.Wrap(err, "failed to verify extension in Ash")
	}

	return nil
}
