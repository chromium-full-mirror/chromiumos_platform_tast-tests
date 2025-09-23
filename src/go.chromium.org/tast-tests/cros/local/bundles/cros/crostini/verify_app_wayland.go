// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/guestos"
	"go.chromium.org/tast-tests/cros/local/guestos/verifyapp"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VerifyAppWayland,
		Desc:         "Runs a Wayland crostini application from the terminal and verifies that it renders",
		Contacts:     []string{"clumptini+oncall@google.com"},
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "vm_host"},
		BugComponent: "b:1122570",
		Params: []testing.Param{
			{
				Name:              "bookworm_stable",
				ExtraSoftwareDeps: []string{"dlc"},
				ExtraHardwareDeps: crostini.CrostiniOptimalPerf,
				Fixture:           "crostiniBookworm",
				Timeout:           3 * time.Minute,
			},
		},
	})
}

func VerifyAppWayland(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(crostini.FixtureData).Chrome
	cont := s.FixtValue().(crostini.FixtureData).Cont
	tconn := s.FixtValue().(crostini.FixtureData).Tconn

	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "ui_tree")
	s.AttachErrorHandlers(handler, handler)

	if err := verifyapp.RunTest(ctx, s.OutDir(), cr, cont, guestos.WaylandDemoConfig()); err != nil {
		s.Fatal("Failed to run test: ", err)
	}
}
