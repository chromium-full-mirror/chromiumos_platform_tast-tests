// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/graphics/sshot"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ScreenshotChrome,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Takes a screenshot using Chrome",
		BugComponent: "b:885255", // ChromeOS > Platform > Graphics
		Contacts: []string{
			"chromeos-gfx@google.com",
			"jkardatzke@chromium.org",
		},
		Attr:         []string{"group:graphics", "graphics_perbuild"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeGraphics",
	})
}

func ScreenshotChrome(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	err := sshot.SShot(ctx, s, cr, func(ctx context.Context, path string) error {
		return screenshot.CaptureChrome(ctx, cr, path)
	})
	if err != nil {
		s.Fatal("Failure in screenshot comparison: ", err)
	}
}
