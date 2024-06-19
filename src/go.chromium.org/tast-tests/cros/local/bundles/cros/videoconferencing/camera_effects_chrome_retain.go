// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"image"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/effectshtml"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CameraEffectsChromeRetain,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks Video Effects retains after re-launching vc apps",
		Contacts: []string{
			"cros-video-conference-tast-tests@google.com",
			"charleszhao@google.com",
		},
		BugComponent: "b:187682",
		Timeout:      10 * time.Minute,
		Attr: []string{
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
		},
		TestBedDeps: []string{tbdep.Cbx(true)},
		Data: []string{
			"effects_frame_metrics.js",
			"effects_video_script.html",
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
				Name:    "ash",
				Fixture: fixture.LoggedInWithFakeHALAndEffectsEnabled,
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

func CameraEffectsChromeRetain(cleanupCtx context.Context, s *testing.State) {
	ctx, tconn, cr, br, srvURL, cleanupFunc := common.Setup(cleanupCtx, s)
	defer cleanupFunc()

	vcTray := vctray.New(ctx, tconn)

	ui := uiauto.New(tconn)

	url := srvURL + effectshtml.PageURL
	if err := effectshtml.OpenURLAndWaitForStreamToReady(ctx, tconn, br, url, vcTray); err != nil {
		s.Fatal("Fail to wait for camera stream: ", err)
	}

	// Only test camera effects in different resolution with BackgroundBlurFull and RelightingOn.
	if err := vcTray.SetCameraEffects(vctray.BackgroundBlurFull, true)(ctx); err != nil {
		s.Fatalf("Failed to set camera effects to BackgroundBlur %v; PortraitRelighting on: %v",
			vctray.BackgroundBlurFull, err)
	}

	// Take a screenshot before camera tab is closed.
	imageBefore, err := effectshtml.GrabVideoArea(ctx, cr, tconn, ui)
	if err != nil {
		s.Fatal("Fail to grab camera screen shot before: ", err)
	}

	if err := effectshtml.CloseURLAndWaitForWindowGone(ctx, tconn, br, url, vcTray); err != nil {
		s.Fatal("Fail to wait for close tab: ", err)
	}

	if err := effectshtml.OpenURLAndWaitForStreamToReady(ctx, tconn, br, url, vcTray); err != nil {
		s.Fatal("Fail to wait for camera stream: ", err)
	}
	s.Log("OpenURLAndWaitForStreamToReady successfully after closing the tab")

	var imageAfter image.Image
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Take a screenshot after a new tab is opened.
		imageAfter, err = effectshtml.GrabVideoArea(ctx, cr, tconn, ui)
		if err != nil {
			return err
		}
		s.Log("GrabVideoArea successfully")

		// Camera effects should be identical for new tab.
		notChangedThreshold := 0.95
		changedThreshold := 0.0
		notChanged, changed := effectshtml.ImageDiff(imageBefore, imageAfter, 0.0)
		if notChanged < notChangedThreshold || changed < changedThreshold {
			return errors.Errorf("Wrong percentage of pixel change: %f changed and %f not changed", changed, notChanged)
		}

		return nil

	}, &testing.PollOptions{Timeout: 3 * time.Second, Interval: time.Second}); err != nil {
		effectshtml.SaveImageToFaillog(ctx, s, imageBefore, effectshtml.BeforeEffectsImageName)
		effectshtml.SaveImageToFaillog(ctx, s, imageAfter, effectshtml.AfterEffectsImageName)
		s.Fatal("Screenshot diff unexpected: ", err)
	}
}
