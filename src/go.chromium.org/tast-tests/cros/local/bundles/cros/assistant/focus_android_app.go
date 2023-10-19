// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assistant

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/assistant"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FocusAndroidApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that assistant focuses Android app if both web and Android versions are open",
		Contacts:     []string{"assistive-eng@google.com"},
		BugComponent: "b:905229", // ChromeOS > Software > Assistive
		Attr: []string{
			"group:mainline",
			"informational",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Fixture:      "assistantWithArc",
		Timeout:      3 * time.Minute,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

func FocusAndroidApp(ctx context.Context, s *testing.State) {
	const (
		QueryOpenGoogleNews = "Open Google News"
	)

	fixtData := s.FixtValue().(*assistant.FixtData)
	cr := fixtData.Chrome
	a := fixtData.ARC

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	if err := assistant.InstallTestApkAndWaitReady(ctx, tconn, a); err != nil {
		s.Fatal("Failed to install a test app: ", err)
	}

	if err := launcher.LaunchApp(tconn, assistant.GoogleNewsAppTitle)(ctx); err != nil {
		s.Fatal("Failed to launch Google News Android: ", err)
	}

	if err := assistant.WaitForGoogleNewsAppActivation(ctx, tconn); err != nil {
		s.Fatal("Failed to wait Google News Android gets active: ", err)
	}

	// TODO (b/245349115): Remove this work around once the bug gets fixed.
	s.Log("Wait 3 seconds for Ash and Arc WM state sync")
	// GoBigSleepLint: See TODO for work-around justification.
	if testing.Sleep(ctx, 3*time.Second); err != nil {
		s.Fatal("Failed to wait 3 seconds for Ash and Arc WM state sync: ", err)
	}

	if _, err = browser.Launch(ctx, tconn, cr, assistant.GoogleNewsWebURL); err != nil {
		s.Fatal("Failed to launch Google News Web: ", err)
	}

	if err := assistant.WaitForGoogleNewsWebActivation(ctx, tconn); err != nil {
		s.Fatal("Failed to wait Google News Web gets active: ", err)
	}

	if _, err := assistant.SendTextQuery(ctx, tconn, QueryOpenGoogleNews); err != nil {
		s.Fatal("Failed to send Assistant text query: ", err)
	}

	if err := assistant.WaitForGoogleNewsAppActivation(ctx, tconn); err != nil {
		s.Fatal("Failed to wait Google News Android gets active: ", err)
	}
}
