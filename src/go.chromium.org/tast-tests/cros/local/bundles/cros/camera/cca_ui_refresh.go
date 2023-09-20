// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIRefresh,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test for checking Chrome Camera App still works after refreshing",
		Contacts:     []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Fixture:      "ccaLaunchedWithFakeHALCamera",
	})
}

// CCAUIRefresh checks CCA still works after refresing the page.
func CCAUIRefresh(ctx context.Context, s *testing.State) {
	app := s.FixtValue().(cca.FixtureData).App()
	tb := s.FixtValue().(cca.FixtureData).TestBridge()
	s.Log("Refreshing CCA")
	if err := app.Refresh(ctx, tb); err != nil {
		s.Fatal("Failed to complete refresh: ", err)
	}

	if err := app.WaitForVideoActive(ctx); err != nil {
		s.Fatal("Preview is not shown after refreshing: ", err)
	}
}
