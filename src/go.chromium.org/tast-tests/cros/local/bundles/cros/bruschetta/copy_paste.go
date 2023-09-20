// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bruschetta

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bruschetta"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/guestos"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {

	testing.AddTest(&testing.Test{
		Func:         CopyPaste,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test copy paste functionality",
		Contacts:     []string{"clumptini+oncall@google.com"},
		SoftwareDeps: []string{"chrome", "vm_host", "untrusted_vm", "dlc", "amd64"},
		HardwareDeps: bruschetta.BruschettaHwDeps,
		Attr:         []string{"group:mainline"},
		BugComponent: "b:658562", // ChromeOS > Software > GuestOS
		Fixture:      bruschetta.BruschettaFixture,
		Timeout:      7 * time.Minute,
		Data:         []string{guestos.CopyApplet, guestos.PasteApplet},
		Params: []testing.Param{
			{
				Name: "wayland_to_wayland",
				Val: guestos.CopyPasteConfig{
					Copy:  guestos.WaylandCopyConfig,
					Paste: guestos.WaylandPasteConfig,
				},
			},
			{
				Name: "wayland_to_x11",
				Val: guestos.CopyPasteConfig{
					Copy:  guestos.WaylandCopyConfig,
					Paste: guestos.X11PasteConfig,
				},
			},
			{
				Name: "x11_to_wayland",
				Val: guestos.CopyPasteConfig{
					Copy:  guestos.X11CopyConfig,
					Paste: guestos.WaylandPasteConfig,
				},
			},
			{
				Name: "x11_to_x11",
				Val: guestos.CopyPasteConfig{
					Copy:  guestos.X11CopyConfig,
					Paste: guestos.X11PasteConfig,
				},
			},
		},
	})
}

func CopyPaste(ctx context.Context, s *testing.State) {
	param := s.Param().(guestos.CopyPasteConfig)
	tconn := s.FixtValue().(bruschetta.FixtureData).Tconn
	bru := s.FixtValue().(bruschetta.FixtureData).BruschettaVM
	kb := s.FixtValue().(bruschetta.FixtureData).KB

	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "ui_tree")
	s.AttachErrorHandlers(handler, handler)

	if err := guestos.CopyPaste(ctx, param, s.DataPath, tconn, kb, bru); err != nil {
		s.Fatal("Copy and paste test failed: ", err)
	}
}
