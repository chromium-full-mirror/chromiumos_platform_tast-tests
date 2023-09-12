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
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ScreenshotCLI,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Takes a screenshot using the CLI",
		BugComponent: "b:995569", // ChromeOS > Platform > Graphics > GPU
		Contacts: []string{
			"chromeos-gfx@google.com",
			"chromeos-gfx-compositor@google.com",
			"hob@google.com",
		},
		Attr:         []string{"group:cq-minimal", "group:mainline", "group:cq-medium"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Fixture: "chromeGraphics",
		}, {
			Name:    "passthrough",
			Fixture: "chromeGraphicsPassthrough",
		}, {
			Name:              "vulkan",
			Fixture:           "chromeGraphicsVulkan",
			ExtraSoftwareDeps: []string{"vulkan_composite"},
			ExtraAttr:         []string{"informational"},
		}},
	})
}

func ScreenshotCLI(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)
	err := sshot.SShot(ctx, s, cr, func(ctx context.Context, path string) error {
		return screenshot.Capture(ctx, path)
	})
	if err != nil {
		s.Fatal("Failure in screenshot comparison: ", err)
	}
}
