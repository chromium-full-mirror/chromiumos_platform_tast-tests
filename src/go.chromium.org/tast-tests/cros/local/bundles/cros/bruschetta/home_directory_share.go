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
	"go.chromium.org/tast-tests/cros/local/guestos"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HomeDirectoryShare,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test sharing the VM home directory with the Files app",
		Contacts:     []string{"clumptini+oncall@google.com", "sidereal@google.com", "jamesye@google.com"},
		SoftwareDeps: []string{"chrome", "vm_host", "untrusted_vm", "dlc", "amd64"},
		HardwareDeps: bruschetta.BruschettaHwDeps,
		Attr:         []string{"group:mainline"},
		BugComponent: "b:658562", // ChromeOS > Software > GuestOS
		Fixture:      bruschetta.BruschettaFixture,
	})
}

func HomeDirectoryShare(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(bruschetta.FixtureData).Tconn
	guest := s.FixtValue().(bruschetta.FixtureData).BruschettaVM
	kb := s.FixtValue().(bruschetta.FixtureData).KB

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Open Files app.
	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to open Files app: ", err)
	}
	defer filesApp.Close(cleanupCtx)

	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "ui_tree")
	s.AttachErrorHandlers(handler, handler)

	if err := guestos.FilesAppToGuest(ctx, filesApp, kb, guest, constants.BruschettaVMName, "."); err != nil {
		s.Fatal("Files app to guest failed: ", err)
	}

	if err := guestos.GuestToFilesApp(ctx, filesApp, guest, constants.BruschettaVMName, "."); err != nil {
		s.Fatal("Guest to Files app failed: ", err)
	}
}
