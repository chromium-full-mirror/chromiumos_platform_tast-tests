// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfaillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/cws"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         EduCUJ,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Runs Edu user CUJ in Lacros including installing apps and open webpages with authentication",
		Contacts:     []string{"lacros-team@google.com"},
		BugComponent: "b:1456869",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("markarth")),
		SoftwareDeps: []string{"chrome", "lacros"},
		Fixture:      "lacrosEduGaiaLogin",
		// It takes about 40 seconds to install an extension, 20 seconds to uninstall an extension.
		Timeout: chrome.GAIALoginTimeout + 10*time.Minute,
	})
}

type cujWebpage struct {
	name string
	url  string
}

// EduCUJ makes sure multiple Lacros windows can open multiple education CUJ
// webpages without any issues after installation of new extensions.
func EduCUJ(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	l, err := lacros.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch lacros-chrome: ", err)
	}
	defer lacrosfaillog.SaveIf(cleanupCtx, tconn, s.HasError)
	// Extensions in cws will be installed.
	apps := []cws.App{
		cws.ReadAndWrite,
		cws.Kami,
		cws.Screencastify,
	}
	// Install the the extensions above.
	for _, app := range apps {
		cws.InstallApp(ctx, l.Browser(), tconn, app)
		defer cws.UninstallApp(ctx, l.Browser(), tconn, app)
	}
	// The webpages that will be open in the first window.
	firstWindowCUJWebPages := []cujWebpage{
		{
			name: "Kids Space",
			url:  "https://families.google.com/kidsspace/",
		},
		{
			name: "Youtube",
			url:  "https://www.youtube.com",
		},
		{
			name: "Google Slides",
			url:  "https://slides.google.com",
		},
	}
	// The webpage that will be open in the second window.
	secondWindowCUJWebPage := cujWebpage{
		name: "Grow with Google",
		url:  "https://grow.google/",
	}
	// Start to open the webpages in the first window.
	for _, t := range firstWindowCUJWebPages {
		c, err := l.NewConn(ctx, t.url)
		if err != nil {
			s.Fatalf("Failed to open Lacros with URL %s: %v", t.url, err)
		}
		defer c.Close()
	}
	// Open webpage in the second window.
	c, err := l.NewConn(ctx, secondWindowCUJWebPage.url, browser.WithNewWindow())
	if err != nil {
		s.Fatalf("Failed to open Lacros with URL %s: %v", secondWindowCUJWebPage.url, err)
	}
	defer c.Close()
}
