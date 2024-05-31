// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package projector is used for writing Projector tests.
package projector

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/projector"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CreationFlow,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Launches the Projector app and goes through the new screencast creation flow with annotator",
		Contacts:     []string{"cros-projector+tast@google.com", "llin@chromium.org"},
		// ChromeOS > Software > Family > Projector
		BugComponent: "b:1080013",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "ondevice_speech", "gaia"},
		HardwareDeps: hwdep.D(hwdep.Microphone()),
		Timeout:      10 * time.Minute,
		Fixture:      "projectorLogin",
	})
}

func CreationFlow(ctx context.Context, s *testing.State) {
	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 4*time.Minute)
	defer cancel()

	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()
	app := s.FixtValue().(projector.HasApp).App()

	defer faillog.DumpUITreeOnError(ctxForCleanUp, s.OutDir(), s.HasError, tconn)

	cleanup, err := projector.SetUpProjectorApp(ctx, tconn, app)
	if err != nil {
		s.Fatal("Failed to set up Projector app: ", err)
	}
	defer cleanup(ctxForCleanUp)

	// We need to clean up any screencasts after the test to
	// prevent taking up Drive quota over time.
	defer projector.DeleteScreencastItems(ctxForCleanUp, tconn)

	s.Log("Launching the new screencast creation flow")
	if err := projector.LaunchCreationFlow(ctx, tconn, true /*launchAnnotator*/); err != nil {
		s.Fatal("Failed to go through the new screencast creation flow: ", err)
	}
}
