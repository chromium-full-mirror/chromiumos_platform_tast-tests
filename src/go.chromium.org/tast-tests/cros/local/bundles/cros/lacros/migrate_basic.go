// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/lacros/migrate"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MigrateBasic,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test basic functionality of Ash-to-Lacros profile migration",
		Contacts: []string{
			"lacros-team@google.com",
			"ythjkt@google.com", // Test author
			"neis@google.com",
			"hidehiko@google.com",
		},
		BugComponent: "b:1456869",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "lacros"},
	})
}

// MigrateBasic tests that migration is run to completion and Lacros is launchable after the migration.
func MigrateBasic(ctx context.Context, s *testing.State) {
	cr, err := migrate.StartChromeToClearMigrationState(ctx)
	if err != nil {
		s.Fatal("Failed to run Chrome to clear migration state: ", err)
	}
	cr.Close(ctx)

	cr, err = migrate.Run(ctx, []chrome.Option{}, []lacrosfixt.Option{})
	if err != nil {
		s.Fatal("Failed to migrate profile: ", err)
	}
	defer cr.Close(ctx)

	if err := migrate.VerifyLacrosLaunch(ctx, s, cr); err != nil {
		s.Fatal("Failed to launch lacros: ", err)
	}
}
