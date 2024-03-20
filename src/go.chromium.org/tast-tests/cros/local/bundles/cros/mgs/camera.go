// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mgs

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/mgs"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Camera,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the camera is working in managed guest sessions",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		SoftwareDeps: []string{"reboot", "camera_app", "chrome", "vpd", caps.BuiltinOrVividCamera},
		Attr: []string{
			"group:camera_dependent",
			"group:complementary",
			"group:golden_tier",
			"group:hardware",
			"group:medium_low_tier",
			"group:hw_agnostic",
		},
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

	outDir := s.OutDir()
	tb, err := testutil.NewTestBridge(ctx, cr, testutil.UseRealCamera)
	if err != nil {
		s.Fatal("Failed to construct test bridge: ", err)
	}
	defer tb.TearDown(ctx)

	app, err := cca.New(ctx, cr, outDir, tb)
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
