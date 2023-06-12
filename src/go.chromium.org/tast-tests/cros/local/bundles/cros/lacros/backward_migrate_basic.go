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
		Func:         BackwardMigrateBasic,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test basic functionality of Lacros-to-Ash profile migration",
		BugComponent: "b:1088267",
		Contacts: []string{
			"lacros-team@google.com",
			"vsavu@google.com", // Test author
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "lacros"},
	})
}

// BackwardMigrateBasic tests forward and then backward lacros migration.
func BackwardMigrateBasic(ctx context.Context, s *testing.State) {
	if err := migrate.ClearMigrationState(ctx); err != nil {
		s.Fatal("Failed to run Chrome to clear migration state: ", err)
	}

	forwardMigrate(ctx, s)
	backwardMigrate(ctx, s)
}

func forwardMigrate(ctx context.Context, s *testing.State) {
	cr, err := migrate.Run(ctx, []chrome.Option{}, []lacrosfixt.Option{})
	if err != nil {
		s.Fatal("Failed to migrate profile: ", err)
	}
	defer cr.Close(ctx)

	if err := migrate.VerifyLacrosLaunch(ctx, s, cr); err != nil {
		s.Error("Failed to launch lacros: ", err)
	}
}

func backwardMigrate(ctx context.Context, s *testing.State) {
	cr, err := migrate.BackwardRun(ctx,
		[]chrome.Option{chrome.EnableFeatures("LacrosProfileBackwardMigration")})
	if err != nil {
		s.Fatal("Failed to backward migrate profile: ", err)
	}
	defer cr.Close(ctx)
}
