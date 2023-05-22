// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/camera/cca"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/vctray"
	"chromiumos/tast/local/screenshot"
	"chromiumos/tast/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAEffectsRetain,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks Video Effects retains after re-launching vc apps",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@chromium.org",
		},
		BugComponent: "b:187682",
		Timeout:      10 * time.Minute,
		Attr: []string{
			"group:video_conference", "video_conference_per_build", "group:external-dependency",
		},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		SearchFlags: []*testing.StringPair{
			{
				// Blur retain.
				Key:   "feature_id",
				Value: "screenplay-ad05d869-0d1a-4340-832c-b08fece20bb3",
			},
			{
				// Retain Blur and lighting improvement.
				Key:   "feature_id",
				Value: "screenplay-f6e8fc90-04dc-49eb-9051-00391c045fce",
			},
			{
				// Retain relighting.
				Key:   "feature_id",
				Value: "screenplay-bff2442b-de76-422e-a45c-f077f0f6eca3",
			},
		},
		Params: []testing.Param{
			{
				Fixture:   fixture.LoggedInWithFakeHALAndEffectsEnabled,
				ExtraAttr: []string{"video_conference_cq_critical"},
			},
			{
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.LoggedInLacrosWithFakeHALAndEffectsEnabled,
			},
		},
		Vars: screenshot.ScreenDiffVars,
	})
}

func CCAEffectsRetain(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	if err := apps.Launch(ctx, tconn, apps.Camera.ID); err != nil {
		s.Fatal("Failed to launch Camera app: ", err)
	}
	defer apps.Close(cleanupCtx, tconn, apps.Camera.ID)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	if err := uiauto.New(tconn).WaitUntilExists(cca.A11yCanvasNode)(ctx); err != nil {
		s.Fatal("Camera is not working appropriately: ", err)
	}

	vcTray := vctray.New(ctx, tconn)

	if err := vcTray.SetCameraEffects(vctray.BackgroundBlurFull, true)(ctx); err != nil {
		s.Fatal("Failed to set camera effects: ", err)
	}

	// Re-launch Camera app and verify screen again to check effects retain.
	if err := apps.Close(ctx, tconn, apps.Camera.ID); err != nil {
		s.Fatal("Failed to close Camera: ", err)
	}

	if err := apps.Launch(ctx, tconn, apps.Camera.ID); err != nil {
		s.Fatal("Failed to re-launch Camera: ", err)
	}

	// Maximize VC app window in clamshell mode to reduce resolution noises on different devices.
	// Skip Tablet mode as app is full screen by default.
	if inTabletMode, err := ash.TabletModeEnabled(ctx, tconn); err != nil {
		s.Fatal("Failed to get tablet-mode status: ", err)
	} else if !inTabletMode {
		if _, err := ash.MaximizeWindowTitleContains(ctx, tconn, "Camera"); err != nil {
			s.Fatal("Failed to maximize Camera window: ", err)
		}
	}

	d, err := screenshot.NewDifferFromChrome(ctx, s, cr,
		screenshot.Config{
			DefaultOptions: screenshot.Options{
				WindowState: ash.WindowStateDefault,
			},
			SkipDpiNormalization: true,
		})
	if err != nil {
		s.Fatal("Failed to start screen differ: ", err)
	}
	defer d.DieOnFailedDiffs()
	if err := d.Diff(ctx, "backgroundblur_full_portraitrelighting_on", cca.A11yCanvasNode,
		screenshot.Retries(5),
		screenshot.RetryInterval(time.Second),
	)(ctx); err != nil {
		s.Fatal("Failed the skia gold diff: ", err)
	}
}
