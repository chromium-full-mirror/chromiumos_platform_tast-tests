// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package holdingspace

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/arcdownload"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/holdingspace"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ArcDownload,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that ARC downloads are shown in holding space",
		BugComponent: "b:1268276", // ChromeOS > Software > System UI Surfaces > HoldingSpace
		Contacts: []string{
			"tote-eng@google.com",
			"cros-system-ui-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"dmblack@google.com",
		},
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      "arcBooted",
		Data:         []string{"light_resistance.txt"},
		SoftwareDeps: []string{"chrome", "android_vm"},
		VarDeps:      []string{ui.GaiaPoolDefaultVarName},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-cbd2ebb4-8f09-4902-a7ae-9eb7619f7409",
		}},
		Timeout: 5 * time.Minute,
	})
}

// ArcDownload verifies that ARC downloads are shown in holding space.
// TODO(crbug.com/1347600): Replace "Chrome (Beta)" app usage to avoid flakes.
func ArcDownload(ctx context.Context, s *testing.State) {
	const (
		filename        = "light_resistance.txt"
		targetPath      = "/storage/emulated/0/Download/" + filename
		localServerPort = 8080
	)

	sourcePath := s.DataPath(filename)

	a := s.FixtValue().(*arc.PreData).ARC
	d := s.FixtValue().(*arc.PreData).UIDevice
	cr := s.FixtValue().(*arc.PreData).Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	didDownloadFile := false
	defer func(ctx context.Context) {
		if didDownloadFile {
			if err := a.RemoveAll(ctx, targetPath); err != nil {
				s.Fatalf("Failed to remove %s: %v", targetPath, err)
			}
		}
	}(ctx)

	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_dump")

	if err := holdingspace.ResetHoldingSpace(ctx, tconn, holdingspace.ResetHoldingSpaceOptions{}); err != nil {
		s.Fatal("Failed to reset holding space: ", err)
	}

	uia := uiauto.New(tconn)
	err = uia.EnsureGoneFor(holdingspace.FindTray(), 5*time.Second)(ctx)
	if err != nil {
		s.Fatal("Tray exists: ", err)
	}

	if err := arcdownload.DownloadTestFile(ctx, cr, a, d, sourcePath, targetPath); err != nil {
		s.Fatal("Failed to download test file from arc: ", err)
	}

	didDownloadFile = true

	if err := uiauto.Combine("check for download chip",
		// Left click the tray to open the bubble.
		uia.LeftClick(holdingspace.FindTray()),
		// Verify that the ARC download exists in holding space.
		uia.WaitUntilExists(holdingspace.FindDownloadChip().Name(filename)),
	)(ctx); err != nil {
		s.Fatal("Download chip not found: ", err)
	}
}
