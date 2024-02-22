// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bruschetta

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bruschetta"
	"go.chromium.org/tast-tests/cros/local/bruschetta/constants"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/guestos/apps"
	"go.chromium.org/tast-tests/cros/local/screenshot"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AppVscodeFromFileManager,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Launches Visual Studio Code from File Manager",
		Contacts:     []string{"clumptini+oncall@google.com"},
		Attr:         []string{"group:mainline", "group:bruschetta_cq", "informational"},
		Vars:         screenshot.ScreenDiffVars,
		SoftwareDeps: []string{"chrome", "vm_host", "untrusted_vm", "dlc", "amd64"},
		HardwareDeps: bruschetta.BruschettaHwDeps,
		BugComponent: "b:658562", // ChromeOS > Software > GuestOS
		Fixture:      bruschetta.BruschettaFixtureClamshell,
		Timeout:      15 * time.Minute,
	})
}

func AppVscodeFromFileManager(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(bruschetta.FixtureData).Tconn
	keyboard := s.FixtValue().(bruschetta.FixtureData).KB
	bru := s.FixtValue().(bruschetta.FixtureData).BruschettaVM

	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "ui_tree")
	s.AttachErrorHandlers(handler, handler)

	// Open Files app.
	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to open Files app: ", err)
	}
	defer filesApp.Close(cleanupCtx)

	if err := apps.OpenVscodeFromFileManager(ctx, apps.VSCodium, filesApp, keyboard, tconn, bru, constants.BruschettaVMName); err != nil {
		s.Fatal("Failed to open file: ", err)
	}
}
