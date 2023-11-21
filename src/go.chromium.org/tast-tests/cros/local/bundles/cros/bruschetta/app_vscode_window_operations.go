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
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AppVscodeWindowOperations,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Performs maximize/restore/minimize/close actions on VS Code",
		Contacts:     []string{"clumptini+oncall@google.com", "jamesye@google.com"},
		Attr:         []string{"group:mainline"},
		Vars:         screenshot.ScreenDiffVars,
		SoftwareDeps: []string{"chrome", "vm_host", "untrusted_vm", "dlc", "amd64"},
		HardwareDeps: bruschetta.BruschettaHwDeps,
		BugComponent: "b:658562", // ChromeOS > Software > GuestOS
		Fixture:      bruschetta.BruschettaFixtureClamshell,
		Timeout:      15 * time.Minute,
	})
}

func AppVscodeWindowOperations(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(bruschetta.FixtureData).Tconn
	keyboard := s.FixtValue().(bruschetta.FixtureData).KB
	bru := s.FixtValue().(bruschetta.FixtureData).BruschettaVM

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "ui_tree")
	s.AttachErrorHandlers(handler, handler)

	if err := apps.InitialiseVSCode(ctx, apps.VSCodium, bru, keyboard, tconn); err != nil {
		s.Fatal("Failed to configure VS Code: ", err)
	}

	for _, err := range apps.TestMaximizeRestoreMinimizeClose(ctx, apps.VSCodium.AppName, apps.VSCodium.AppName, "File", keyboard, tconn, nil, true) {
		s.Error("Window operation failed: ", err)
	}
}
