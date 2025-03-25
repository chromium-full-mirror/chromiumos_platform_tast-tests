// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LocalDisplay,
		Desc:         "Get display information",
		Contacts:     []string{"tast-core@google.com"},
		BugComponent: "b:1034522", // ChromeOS > Test > Harness > Tast > Examples
		Attr:         []string{"group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
	})
}

func LocalDisplay(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to log in ash-chrome: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get tconn: ", err)
	}

	info, err := display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the primary display info: ", err)
	}

	logPrimaryDisplayInfo(ctx, tconn, info)

	if err := display.SetDisplayRotationSync(ctx, tconn, info.ID, display.Rotate90); err != nil {
		s.Fatal("Failed to set the primary display rotation by 90 degrees: ", err)
	}
	defer display.SetDisplayRotationSync(ctx, tconn, info.ID, display.Rotate0)

	info, err = display.GetPrimaryInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get the primary display info after rotation: ", err)
	}

	logPrimaryDisplayInfo(ctx, tconn, info)
}

func logPrimaryDisplayInfo(ctx context.Context, tconn *chrome.TestConn, info *display.Info) error {
	testing.ContextLog(ctx, "id: ", info.ID)
	testing.ContextLog(ctx, "IsPrimary: ", info.IsPrimary)
	testing.ContextLogf(ctx, "Bounds: %+v", info.Bounds)
	testing.ContextLogf(ctx, "Modes: %+v", info.Modes)

	for _, m := range info.Modes {
		testing.ContextLogf(ctx, "%+v", *m)
	}
	return nil
}
