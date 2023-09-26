// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/lacros/migrate"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MigratePWA,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test Ash-to-Lacros profile migration with PWA",
		Contacts: []string{
			"lacros-team@google.com",
			"ythjkt@google.com", // Test author
			"neis@google.com",
		},
		BugComponent: "crbug:OS>LaCrOS",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "lacros"},
		Data:         []string{"migrate_pwa/register_sw.js", "migrate_pwa/sw.js", "migrate_pwa/pwa.html"},
	})
}

func MigratePWA(ctx context.Context, s *testing.State) {
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer func() {
		if server != nil {
			server.Close()
		}
	}()

	url := server.URL + "/migrate_pwa/pwa.html"

	if err := setupPWA(ctx, s, url); err != nil {
		s.Fatal("Failed setupPWA: ", err)
	}

	// Now close the server to simulates an offline access to the site.
	server.Close()
	server = nil

	cr, err := migrate.Run(ctx, []chrome.Option{}, []lacrosfixt.Option{})
	if err != nil {
		s.Fatal("Failed to migrate profile: ", err)
	}
	defer cr.Close(ctx)

	if err := verifyPWA(ctx, cr, url); err != nil {
		s.Fatal("verifyPWA failed: ", err)
	}
}

// setupPWA visits the url and calls a JS code to register a service worker and cache web resources.
func setupPWA(ctx context.Context, s *testing.State, url string) error {
	cr, err := migrate.StartChromeToClearMigrationState(ctx)
	if err != nil {
		return errors.Wrap(err, "failed StartChromeToClearMigrationState")
	}
	defer cr.Close(ctx)

	conn, err := cr.NewConn(ctx, url)
	if err != nil {
		return errors.Wrap(err, "failed to visit "+url)
	}
	defer conn.Close()

	registerSWJS, err := os.ReadFile(s.DataPath("migrate_pwa/register_sw.js"))
	if err != nil {
		return errors.Wrap(err, "failed to read register_sw.js")
	}
	if err := conn.Call(ctx, nil, string(registerSWJS)); err != nil {
		return errors.Wrap(err, "registerSWJS failed")
	}

	if err := conn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close page")
	}

	return nil
}

// verifyPWA checks if `url` can be loaded offline on Lacros. Run this after stopping the http server for the `url`.
func verifyPWA(ctx context.Context, cr *chrome.Chrome, url string) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create a tconn")
	}

	l, err := lacros.Launch(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to launch Lacros")
	}
	defer l.Close(ctx)

	conn, err := l.NewConn(ctx, url)
	if err != nil {
		return errors.Wrap(err, "failed to visit "+url)
	}
	defer conn.Close()

	ui := uiauto.New(tconn)
	link := nodewith.Name("Detect Me!").Role(role.Link)
	if err := ui.WaitUntilExists(link)(ctx); err != nil {
		return errors.Wrap(err, "failed to find link 'Detect Me!'")
	}

	return nil
}
