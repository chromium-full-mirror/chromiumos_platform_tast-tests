// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package projector is used for writing Projector tests.
package projector

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/projector"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SharedScreencast,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Opens a shared screencast in viewer mode",
		Contacts:     []string{"cros-projector+tast@google.com", "llin@chromium.org"},
		// ChromeOS > Software > Family > Projector
		BugComponent: "b:1080013",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		VarDeps: []string{
			"projector.sharedScreencastLink",
		},
		Params: []testing.Param{
			{
				Fixture: "projectorLogin",
				Val:     browser.TypeAsh,
			},
			{
				Name:              "lacros",
				Fixture:           "lacrosProjectorLogin",
				ExtraSoftwareDeps: []string{"lacros"},
				Val:               browser.TypeLacros,
			},
		},
	})
}

func SharedScreencast(ctx context.Context, s *testing.State) {
	// Leave 10 seconds for dumping the UI tree.
	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	sharedScreencast := s.RequiredVar("projector.sharedScreencastLink")

	defer faillog.DumpUITreeOnError(ctxForCleanUp, s.OutDir(), s.HasError, tconn)

	if err := projector.OpenSharedScreencast(ctx, tconn, cr, s.Param().(browser.Type), sharedScreencast); err != nil {
		s.Fatal("Failed to open shared screencast: ", err)
	}

	// Set timeout to one minute to allow the shared screencast to load over the network.
	ui := uiauto.New(tconn).WithTimeout(time.Minute)

	// Verify the shared screencast title rendered correctly.
	if err := ui.WaitUntilExists(nodewith.Name("Screencast for Tast (Do not modify)").Role(role.StaticText))(ctx); err != nil {
		s.Fatal("Failed to render shared screencast: ", err)
	}
}
