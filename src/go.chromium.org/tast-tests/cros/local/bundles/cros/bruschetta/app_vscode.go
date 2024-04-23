// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bruschetta

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bruschetta"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/guestos/apps"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/terminalapp"
	"go.chromium.org/tast-tests/cros/local/uidetection"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AppVscode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Opens Visual Studio Code from terminal and performs UI interactions",
		Contacts:     []string{"clumptini+oncall@google.com"},
		Attr:         []string{"group:mainline", "group:bruschetta_cq", "informational"},
		Vars:         screenshot.ScreenDiffVars,
		VarDeps:      uidetection.UIDetectionVars,
		SoftwareDeps: []string{"chrome", "vm_host", "untrusted_vm", "dlc", "amd64"},
		HardwareDeps: bruschetta.BruschettaHwDeps,
		BugComponent: "b:658562", // ChromeOS > Software > GuestOS
		Fixture:      bruschetta.BruschettaFixtureClamshell,
		Timeout:      15 * time.Minute,
	})
}

func AppVscode(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(bruschetta.FixtureData).Tconn
	keyboard := s.FixtValue().(bruschetta.FixtureData).KB
	bru := s.FixtValue().(bruschetta.FixtureData).BruschettaVM

	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "ui_tree")
	s.AttachErrorHandlers(handler, handler)

	// Open Terminal app.
	terminal, err := terminalapp.LaunchBruschetta(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to open Terminal app: ", err)
	}
	defer terminal.Exit(keyboard)(cleanupCtx)

	if err := apps.TestCreateFileWithVSCode(ctx, apps.VSCodium, terminal, keyboard, tconn, bru, nil); err != nil {
		s.Fatal("Failed to create file with Visual Studio Code in Terminal: ", err)
	}
}
