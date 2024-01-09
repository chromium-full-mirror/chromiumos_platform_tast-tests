// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/lacros/migrate"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MigrateExtension,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test the functionality of Ash-to-Lacros profile migration with an extension downloaded from Chrome Web Store",
		Contacts: []string{
			"lacros-team@google.com",
			"ythjkt@google.com", // Test author
			"neis@google.com",
			"hidehiko@google.com",
		},
		BugComponent: "crbug:OS>LaCrOS",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "lacros"},
		Timeout:      4 * time.Minute,
	})
}

func MigrateExtension(ctx context.Context, s *testing.State) {
	if err := setupAshProfileWithExtension(ctx); err != nil {
		s.Fatal("Failed to set up Ash profile with the extension: ", err)
	}

	cr, err := migrate.Run(ctx, []chrome.Option{}, []lacrosfixt.Option{})
	if err != nil {
		s.Fatal("Failed to migrate profile: ", err)
	}
	defer cr.Close(ctx)

	if err := verifyLacrosProfileForExtension(ctx, cr); err != nil {
		s.Fatal("Failed to verify Lacros profile with the extension: ", err)
	}
}

func setupAshProfileWithExtension(ctx context.Context) error {
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

func verifyLacrosProfileForExtension(ctx context.Context, cr *chrome.Chrome) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	l, err := lacros.Launch(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to launch Lacros")
	}
	defer l.Close(ctx)

	if err := migrate.VerifyExtension(ctx, cr, l.Browser()); err != nil {
		return errors.Wrap(err, "failed to verify extension on Lacros")
	}

	return nil
}
