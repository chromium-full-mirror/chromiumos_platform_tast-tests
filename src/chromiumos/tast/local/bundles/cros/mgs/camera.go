// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mgs

import (
	"context"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/media/caps"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/camera/cca"
	"chromiumos/tast/local/camera/testutil"
	"chromiumos/tast/local/mgs"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Camera,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the camera is working in managed guest sessions",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"mpolzer@google.com", // Test author
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		SoftwareDeps: []string{"reboot", "camera_app", "chrome", caps.BuiltinOrVividCamera},
		Attr: []string{
			"group:camera_dependent",
			"group:complementary",
			"group:golden_tier",
			"group:hardware",
			"group:medium_low_tier",
		},
		Data:    []string{"cca_ui.js"},
		Fixture: fixture.FakeDMSEnrolled,
	})
}

func Camera(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	mgs, cr, err := mgs.New(
		ctx,
		fdms,
		mgs.DefaultAccount(),
		mgs.AutoLaunch(mgs.MgsAccountID),
	)
	if err != nil {
		s.Fatal("Failed to start MGS: ", err)
	}
	defer mgs.Close(ctx)

	scripts := []string{s.DataPath("cca_ui.js")}
	outDir := s.OutDir()
	tb, err := testutil.NewTestBridge(ctx, cr, testutil.UseRealCamera)
	if err != nil {
		s.Fatal("Failed to construct test bridge: ", err)
	}
	defer tb.TearDown(ctx)

	app, err := cca.New(ctx, cr, scripts, outDir, tb)
	if err != nil {
		s.Fatal("Failed to start CCA with no policy: ", err)
	}
	defer app.Close(ctx)

	// Test taking a photo.
	if err := app.SwitchMode(ctx, cca.Photo); err != nil {
		s.Error("Failed to switch to photo mode: ", err)
	}
	if _, err := app.TakeSinglePhoto(ctx, cca.TimerOff); err != nil {
		s.Error("Failed to take photo: ", err)
	}
}
