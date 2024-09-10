// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/effectshtml"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         VctrayLabelCheck,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks all label texts shows on VCtray",
		Contacts: []string{
			"cros-video-conference-tast-tests@google.com",
			"xiuwen@google.com",
		},
		BugComponent: "b:187682",
		Timeout:      10 * time.Minute,
		Data: []string{
			"effects_frame_metrics.js",
			"effects_video_script.html",
		},
		Attr: []string{
			"group:cbx", "cbx_feature_enabled", "cbx_unstable", "group:video_conference_face_framing_per_build",
		},
		TestBedDeps:  []string{tbdep.Cbx(true)},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		Fixture:      fixture.LoggedInWithFakeHALAndEffectsEnabled,
	})
}

func VctrayLabelCheck(cleanupCtx context.Context, s *testing.State) {
	ctx, tconn, cr, br, srvURL, cleanupFunc := common.Setup(cleanupCtx, s)
	defer cleanupFunc()
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	vcTray := vctray.New(ctx, tconn)
	ui := uiauto.New(tconn)

	url := srvURL + effectshtml.PageURL
	if err := effectshtml.OpenURLAndWaitForStreamToReady(ctx, tconn, br, url, vcTray); err != nil {
		s.Fatal("Fail to wait for camera stream: ", err)
	}

	vcTray.ExpandPanel(ctx)

	// There are four feature labels: "Appearance effects", "Noise cancellation", "Live Caption" and "Camera framing".
	// "Camera framing" only will be shown on some devices.
	// If we can guarantee that all other 3 labels could be found in A11y tree. That means the UI is not broken. b/353895309
	featureLabels := map[string]*nodewith.Finder{
		"Appearance effects": nodewith.Role(role.StaticText).Name("Appearance effects").HasClass("ToggleEffectsButtonLabel"),
		"Noise cancellation": nodewith.Role(role.StaticText).Name("Noise cancellation").HasClass("ToggleEffectsButtonLabel"),
		"Live Caption":       nodewith.Role(role.StaticText).Name("Live Caption").HasClass("ToggleEffectsButtonLabel"),
	}

	backgroundLabels := map[string]*nodewith.Finder{
		"Light Blur": nodewith.Role(role.StaticText).Name("Light Blur").HasClass("Label"),
		"Full Blur":  nodewith.Role(role.StaticText).Name("Full Blur").HasClass("Label"),
		"Image":      nodewith.Role(role.StaticText).Name("Image").HasClass("Label"),
	}

	for label, finder := range featureLabels {
		if err := ui.WaitUntilExists(finder)(ctx); err != nil {
			s.Fatalf("Fail to find feaure label %s: %v", label, err)
		}
	}

	for label, finder := range backgroundLabels {
		if err := ui.WaitUntilExists(finder)(ctx); err != nil {
			s.Fatalf("Fail to find label %s: %v", label, err)
		}
	}
}
