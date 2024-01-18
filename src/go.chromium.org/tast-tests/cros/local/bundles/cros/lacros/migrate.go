// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/lacros/migrate"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Migrate,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test functionality of Ash-to-Lacros profile migration",
		Contacts: []string{
			"lacros-team@google.com",
			"neis@google.com", // Test author
			"ythjkt@google.com",
			"hidehiko@google.com",
		},
		BugComponent: "b:1456869",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "lacros"},
		Data:         []string{"migrate/indexeddb_check.js", "migrate/indexeddb_set.js", "migrate/boring_page.html"},
		Timeout:      5 * time.Minute,
	})
}

func Migrate(ctx context.Context, s *testing.State) {
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()
	page := migrate.Page{
		URL:   server.URL + "/migrate/boring_page.html",
		Title: "Boring Page",
	}

	prepareAshProfile(ctx, s, kb, page)
	cr, err := migrate.Run(ctx, []chrome.Option{}, []lacrosfixt.Option{})
	if err != nil {
		s.Fatal("Failed to migrate profile: ", err)
	}
	defer cr.Close(ctx)
	verifyLacrosProfile(ctx, s, kb, cr, page)
}

// prepareAshProfile resets profile migration and sets up profile data.
func prepareAshProfile(ctx context.Context, s *testing.State, kb *input.KeyboardEventWriter, page migrate.Page) {
	// First restart Chrome with Lacros disabled in order to reset profile migration.
	cr, err := migrate.StartChromeToClearMigrationState(ctx)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)
	conn, err := cr.NewConn(ctx, chrome.NewTabURL)
	if err != nil {
		s.Fatal("Failed to open a new tab")
	}
	defer conn.Close()
	if err := migrate.SetupProfileData(ctx, cr, s, cr.Browser(), page); err != nil {
		s.Fatal("Failed to set up profile data: ", err)
	}
}

// verifyLacrosProfile checks that the edits done by prepareAshProfile were carried over to Lacros.
func verifyLacrosProfile(ctx context.Context, s *testing.State, kb *input.KeyboardEventWriter, cr *chrome.Chrome, page migrate.Page) {
	if _, err := os.Stat(migrate.LacrosFirstRunPath); err != nil {
		s.Fatal("Error reading 'First Run' file: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	l, err := lacros.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch lacros: ", err)
	}
	defer l.Close(ctx)

	if err := migrate.VerifyProfileData(ctx, cr, s, l.Browser(), page); err != nil {
		s.Fatal("Failed to verify: ", err)
	}
}
