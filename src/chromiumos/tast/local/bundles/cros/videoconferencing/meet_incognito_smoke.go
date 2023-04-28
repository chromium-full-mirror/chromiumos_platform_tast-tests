// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"chromiumos/tast/local/bundles/cros/videoconferencing/common"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/apps/thirdparty/googlemeet"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/vctray"
	"chromiumos/tast/local/chrome/webutil"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MeetIncognitoSmoke,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks Meet VC features in incognito mode",
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
		Fixture:      fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
		SearchFlags: []*testing.StringPair{
			{
				Key: "feature_id",
				// Trigger VC tray with Camera and Mic both ON.
				Value: "screenplay-f0d3d1f4-551a-45c2-9438-c4748c826d9d",
			},
			{
				Key: "feature_id",
				// Foreground of apps.
				Value: "screenplay-4be82867-9e4a-44cb-a532-e72d75892e00",
			},
		},
	})
}

func MeetIncognitoSmoke(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	br := cr.Browser()

	if err := kb.Accel(ctx, "Ctrl+Shift+N"); err != nil {
		s.Fatal("Failed to launch incognito Chrome browser: ", err)
	}

	browserConn, err := br.NewConnForTarget(ctx, chrome.MatchTargetURL(chrome.NewTabURL))
	if err != nil {
		s.Fatal("Failed to setup incognito Chrome browser: ", err)
	}

	if err := browserConn.Navigate(ctx, "https://accounts.google.com"); err != nil {
		s.Fatal("Failed to negavite to account.google.com: ", err)
	}

	if err := webutil.LoginGoogleAccount(ctx, cr, cr.Creds().User, cr.Creds().Pass); err != nil {
		s.Fatal("Failed to login Google account: ", err)
	}

	gm, err := googlemeet.StartNewMeetingWithConn(ctx, cr, browserConn, nil)
	if err != nil {
		s.Fatal("Failed to start meeting: ", err)
	}
	defer gm.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_meet")

	vcTray := vctray.New(ctx, tconn)
	if err := vcTray.WaitUntilExists(ctx); err != nil {
		s.Fatal("Failed to verify camera triggers vcTray: ", err)
	}

	// Verify returnToApp via vcTray.
	if err := common.VerifyReturnToApp(ctx, tconn); err != nil {
		s.Fatal("Failed to verify returnToApp: ", err)
	}
}
