// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/data"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/effectshtml"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	percentageNotChanged  = 0.25
	percentageChanged     = 0.55
	vcBackgroundAppWindow = "Camera Background"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CameraEffectsReplace,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks camera effects background replace",
		Contacts: []string{
			"chromeos-platform-ml@google.com",
			"charleszhao@google.com",
		},
		BugComponent: "b:187682",
		Attr: []string{
			"group:camera_dependent",
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
		},
		TestBedDeps:  []string{tbdep.Cbx(true)},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.FeatureLevel(1)),
		Timeout:      15 * time.Minute,
		Data: []string{
			"effects_frame_metrics.js",
			"effects_video_script.html",
			data.BackgroundImageJpg,
			data.BackgroundMetadata,
		},
		Fixture: fixture.LoggedInWithFakeHALAndEffectsEnabledNoScreenRecorder,
	})
}

func CameraEffectsReplace(cleanupCtx context.Context, s *testing.State) {
	ctx, tconn, cr, br, srvURL, cleanupFunc := common.Setup(cleanupCtx, s)
	defer cleanupFunc()

	// Copy background image and metadata to the BackgroundImageDirname to apply.
	userPath, err := cryptohome.UserPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's userPath path: ", err)
	}

	imagePath := filepath.Join(userPath, data.BackgroundImageDirname)

	if err := os.MkdirAll(imagePath, 0777); err != nil {
		s.Fatal("Failed to create image path: ", err)
	}
	if err := fsutil.CopyFile(
		s.DataPath(data.BackgroundImageJpg), filepath.Join(imagePath, data.BackgroundImageJpg)); err != nil {
		s.Fatal("Failed to copy image to custom-camera-backgrounds: ", err)
	}
	if err := fsutil.CopyFile(
		s.DataPath(data.BackgroundMetadata), filepath.Join(imagePath, data.BackgroundMetadata)); err != nil {
		s.Fatal("Failed to copy metadata to custom-camera-backgrounds: ", err)
	}

	vcTray := vctray.New(ctx, tconn)

	ui := uiauto.New(tconn)

	if _, err := br.NewTab(ctx, srvURL+effectshtml.PageURL); err != nil {
		s.Fatal("Fail to open the fake html: ", err)
	}

	if err := effectshtml.WaitForCameraStreamToReady(ctx, ui, vcTray); err != nil {
		s.Fatal("Fail to wait for camera stream: ", err)
	}

	if err := vcTray.SetCameraEffects(vctray.BackgroundBlurOff, false)(ctx); err != nil {
		s.Fatalf("Failed to set camera effects to BackgroundBlur %v; PortraitRelighting off: %v",
			vctray.BackgroundBlurOff, err)
	}

	// Take a screen shot before background replace is applied.
	imageBefore, err := effectshtml.GrabVideoArea(ctx, cr, tconn, ui)
	if err != nil {
		s.Fatal("Fail to grab camera screen shot before: ", err)
	}

	// Clicking on "Create with AI" and then agree with terms of service.
	agreeButton := nodewith.Name("Got it").Role(role.Button).Ancestor(nodewith.Role(role.Dialog))
	if err := uiauto.NamedCombine("Open VcBackgroundApp",
		vcTray.OpenVcBackgroundApp(),
		ui.WaitUntilExists(agreeButton),
		ui.LeftClick(agreeButton))(ctx); err != nil {
		s.Fatal("Fail to open VcBackgroundApp: ", err)
	}

	// Clicking on the first recently used images in the VcBackgroundApp. This should apply the background image to the camera.
	recentlyUsedImageButton := nodewith.ClassName("recent-image-container").Role(role.GenericContainer).First()
	if err := ui.LeftClick(recentlyUsedImageButton)(ctx); err != nil {
		s.Fatal("Fail to click on the first image: ", err)
	}

	// The minimize button in VcBackgroundApp
	vcBackgroundAppMinimizeButton := nodewith.Name("Minimize").Role(role.Button).Ancestor(nodewith.ClassName("BrowserFrame").Name(vcBackgroundAppWindow).Role(role.Window))

	// Minimize the VcBackgroundApp to prepare for a second screenshot.
	if err :=
		ui.LeftClick(vcBackgroundAppMinimizeButton)(ctx); err != nil {
		s.Fatal("Fail to close VcBackgroundApp: ", err)
	}

	// Wait until the minimization animination complete, which is required for the screenshot of the tab.
	vcBackgroundWindow, err := ash.FindOnlyWindow(ctx, tconn,
		func(w *ash.Window) bool {
			return w.Title == vcBackgroundAppWindow
		})

	if err != nil {
		s.Fatal("Failed to get vcBackgroundAppWindow: ", err)
	}

	// Wait until the window is minimized.
	if err := ash.WaitWindowFinishAnimating(ctx, tconn, vcBackgroundWindow.ID); err != nil {
		s.Fatal("Failed to wait vcBackgroundAppWindow to minimize: ", err)
	}

	// Take a second screenshot after camera background already applied.
	imageAfter, err := effectshtml.GrabVideoArea(ctx, cr, tconn, ui)
	if err != nil {
		s.Fatal("Fail to grab camera screen shot after: ", err)
	}

	// Uncomment these two lines if you need debug it.
	// screenshot.SaveImageToFile(imageBefore, userPath, "before.png")(ctx)
	// screenshot.SaveImageToFile(imageAfter, userPath, "after.png")(ctx)

	// Verify two images have enough portion changed and unchanged.
	notChanged, changed := effectshtml.ImageDiff(imageBefore, imageAfter, 0.0)
	if notChanged < percentageNotChanged || changed < percentageChanged {
		effectshtml.SaveImageToFaillog(ctx, s, imageBefore, effectshtml.BeforeEffectsImageName)
		effectshtml.SaveImageToFaillog(ctx, s, imageAfter, effectshtml.AfterEffectsImageName)
		s.Fatalf("The percentage of pixel change is wrong: %f changed and %f not changed", changed, notChanged)
	}
}
