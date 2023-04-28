// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"chromiumos/tast/local/bundles/cros/videoconferencing/common"
	"chromiumos/tast/local/camera/arcapp"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/apps/thirdparty/googlemeet"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/vctray"
	"chromiumos/tast/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TrayReturnToApp,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks VC tray returns to app is functional",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@chromium.org",
		},
		BugComponent: "b:187682",
		Timeout:      3 * time.Minute,
		Attr: []string{
			"group:camera_dependent",
			"group:external-dependency",
			"group:video_conference",
			"video_conference_per_build",
		},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		Params: []testing.Param{
			{
				Name:    "ash",
				Val:     ash.Web,
				Fixture: fixture.GAIALoggedInTabletWithFakeHALAndEffectsEnabled,
			},
			{
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Val:               ash.Web,
				Fixture:           fixture.GAIALoggedInClamshellWithFakeHALAndEffectsEnabled,
			},
			{
				Name:      "arc",
				Val:       ash.Arc,
				ExtraData: []string{"ArcCameraTest.apk"},
				Fixture:   fixture.GAIALoggedInARCWithInternalCameraAndEffectsEnabled,
			},
		},
	})
}

func TrayReturnToApp(ctx context.Context, s *testing.State) {

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	browserType := s.FixtValue().(fixture.FixtData).BrowserType()

	switch s.Param().(ash.AppType) {
	case ash.Arc:
		verifyReturnToARCApp(ctx, s, cr, tconn)
	case ash.Web:
		verifyReturnToGoogleMeet(ctx, s, cr, browserType, tconn)
	default:
		s.Fatalf("App type %q is not supported", s.Param().(ash.AppType))
	}
}

func verifyReturnToGoogleMeet(ctx context.Context, s *testing.State, cr *chrome.Chrome, browserType browser.Type, tconn *chrome.TestConn) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	br, cleanup, err := browserfixt.SetUp(ctx, cr, browserType)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer cleanup(cleanupCtx)

	gm, err := googlemeet.StartNewMeeting(ctx, cr, br, nil, googlemeet.WithAllPermissions)
	if err != nil {
		s.Fatal("Failed to start meeting: ", err)
	}
	defer gm.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_with_meet")

	// Turn on camera to trigger vcTray.
	if err := uiauto.Combine("configure Meet",
		gm.MuteIfMicAvailable,
		gm.SwitchVideo(true),
	)(ctx); err != nil {
		s.Fatal("Failed to configure Meet: ", err)
	}

	if err := common.VerifyReturnToApp(ctx, tconn); err != nil {
		s.Fatal("Failed to verify returnToApp: ", err)
	}
}

func verifyReturnToARCApp(ctx context.Context, s *testing.State, cr *chrome.Chrome, tconn *chrome.TestConn) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	a := s.FixtValue().(fixture.FixtData).ARC()

	if err := a.Install(ctx, s.DataPath(arcapp.CameraAppApk)); err != nil {
		s.Fatal("Failed to install the APK: ", err)
	}

	cleanupFunc, err := arcapp.LaunchARCCameraApp(ctx, a, tconn)
	if err != nil {
		s.Fatal("Failed to launch ARC camera app: ", err)
	}
	defer cleanupFunc(cleanupCtx, tconn)

	if err := vctray.New(ctx, tconn).WaitUntilExists(ctx); err != nil {
		s.Fatal("Failed to wait for vcTray appears: ", err)
	}

	if err := common.VerifyReturnToApp(ctx, tconn); err != nil {
		s.Fatal("Failed to verify returnToApp: ", err)
	}
}
