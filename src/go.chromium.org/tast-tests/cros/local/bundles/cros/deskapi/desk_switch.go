// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deskapi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/deskapi/apis"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeskSwitch,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks using desk API to switch desk",
		BugComponent: "b:1020793",
		Contacts: []string{
			"cros-commercial-productivity-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"aprilzhou@google.com",
		},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		VarDeps:      []string{"ui.gaiaPoolDefault"},
		Params: []testing.Param{{
			Name:    "ash",
			Val:     browser.TypeAsh,
			Fixture: fixture.DeskAPIAsh,
		}, {
			Name:              "lacros",
			Val:               browser.TypeLacros,
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.DeskAPILacros,
		}},
	})
}

func DeskSwitch(ctx context.Context, s *testing.State) {
	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Local test unable to access data from remote fixture in normal way. Use FixtFillValue instead
	structVal := fixture.DeskFixtData{}
	if err := s.FixtFillValue(&structVal); err != nil {
		s.Fatal("Failed to deserialize remote fixture data: ", err)
	}

	var opts []chrome.Option

	// Additional config for lacros
	if s.Param().(browser.Type) == browser.TypeLacros {
		var err error
		opts, err = lacrosfixt.NewConfig().Opts()
		if err != nil {
			s.Fatal("Failed to retrieve lacro config: ", err)
		}
	}

	// Use the same DMServer endpoint as the in the enrollment
	opts = append(opts, chrome.KeepState(), chrome.TryReuseSession(), chrome.GAIALogin(chrome.Creds{User: structVal.Username, Pass: structVal.Password}), chrome.DMSPolicy(policy.DMServerAlphaURL))

	cr, err := chrome.New(ctx, opts...)

	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	// Use the generic browser interface for both lacros and ash.
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	tconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	ac := uiauto.New(tconn)
	const url string = "https://continuous-sincere-relation.glitch.me"

	// Create a new browser connection with target page opened.
	conn, err := br.NewConn(ctx, url)
	if err != nil {
		s.Fatal("Failed to create a new browser connection: ", err)
	}
	defer conn.Close()

	// Get current active desk
	deskID, err := apis.GetActiveDesk(ctx, conn)
	if err != nil {
		s.Fatal("Failed to get active desk: ", err)
	}

	// Create a new desk
	deskID1, err := apis.LaunchDesk(ctx, conn)
	if err != nil {
		s.Fatal("Failed to launch a new desk: ", err)
	}
	// Clean up desk.
	defer apis.RemoveDesk(ctx, conn, deskID1)

	// Wait for launch desk animation settled.
	if err := ac.WithInterval(2*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
		s.Fatal("Failed to wait for launch desk animation to be completed: ", err)
	}

	deskID2, err := apis.GetActiveDesk(ctx, conn)
	if err != nil {
		s.Fatal("Failed to get active desk: ", err)
	}

	if deskID == deskID2 {
		s.Fatal("Failed to move to new desk: ", err)
	}

	if err := apis.SwitchDesk(ctx, conn, deskID); err != nil {
		s.Fatal("Failed to switch desk: ", err)
	}

	// Wait for switch desk animation settled.
	if err := ac.WithInterval(2*time.Second).WaitUntilNoEvent(nodewith.Root(), event.LocationChanged)(ctx); err != nil {
		s.Fatal("Failed to wait for switch desk animation to be completed: ", err)
	}

	deskID3, err := apis.GetActiveDesk(ctx, conn)
	if err != nil {
		s.Fatal("Failed to get active desk: ", err)
	}

	if deskID != deskID3 {
		s.Fatalf("Failed to switch back to previous desk, want:%s, got %s", deskID, deskID3)
	}

}
