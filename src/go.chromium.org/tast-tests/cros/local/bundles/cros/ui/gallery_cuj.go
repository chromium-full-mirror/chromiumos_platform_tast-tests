// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/gallerycuj"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GalleryCUJ,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measure the performance of a critical user journey for the Gallery and files app",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"cienet-development@googlegroups.com",
			"chicheny@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		Attr:         []string{"group:cuj"},
		SoftwareDeps: []string{"chrome"},
		Data:         gallerycuj.GalleryFiles,
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      30 * time.Minute,
		Fixture:      "loggedInToCUJUser",
	})
}

func GalleryCUJ(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	if err := gallerycuj.Run(ctx, cr, s.OutDir(), s.DataPath); err != nil {
		s.Fatal("Failed to run Gallery CUJ: ", err)
	}
}
