// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package holdingspace

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/diagnosticsapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/holdingspace"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DiagnosticsApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that Diagnostics app logs appear in holding space",
		BugComponent: "b:1268276", // ChromeOS > Software > System UI Surfaces > HoldingSpace
		Contacts: []string{
			"tote-eng@google.com",
			"cros-system-ui-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"dmblack@google.com",
		},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-0d908149-1273-41b3-8c06-cb4ebf70892b",
		}},
		Fixture: "chromeLoggedIn",
	})
}

// DiagnosticsApp verifies that Diagnostics app logs appear in holding space.
func DiagnosticsApp(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	// Reset the holding space.
	if err := holdingspace.ResetHoldingSpace(ctx, tconn,
		holdingspace.ResetHoldingSpaceOptions{}); err != nil {
		s.Fatal("Failed to reset holding space: ", err)
	}

	// Ensure session log does not exist.
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get users Download path: ", err)
	}
	const filename = "session_log.txt"
	filePath := filepath.Join(downloadsPath, filename)
	if _, err := os.Stat(filePath); err == nil {
		os.Remove(filePath)
	}
	defer os.Remove(filePath)

	dxRootnode, err := diagnosticsapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Could not launch Diagnostics app: ", err)
	}

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("Save logs and verify file appears in holding space",
		ui.LeftClick(diagnosticsapp.DxLogButton.Ancestor(dxRootnode)),
		ui.LeftClick(nodewith.Name("Save").Role(role.Button)),
		ui.LeftClick(holdingspace.FindTray()),
		ui.WaitUntilExists(holdingspace.FindDownloadChip().Name(filename)),
	)(ctx); err != nil {
		s.Fatal("Failed to save logs and verify file appears in holding space: ", err)
	}
}
