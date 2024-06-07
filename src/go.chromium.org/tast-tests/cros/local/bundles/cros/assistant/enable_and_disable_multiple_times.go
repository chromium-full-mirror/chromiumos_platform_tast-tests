// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assistant

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/assistant"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         EnableAndDisableMultipleTimes,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Enable/disable Google Assistant service multiple times and checks the running status",
		Contacts: []string{
			"cros-assistive@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		BugComponent: "b:905229", // ChromeOS > Software > Assistive
		Attr: []string{
			"group:mainline",
			"informational",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Timeout:      8 * time.Minute,
	})
}

func EnableAndDisableMultipleTimes(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}

	// Create test API connection.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	// Enable and disable the Assistant multiple times. Should not crash.
	// The max number 30 is an arbitrary number.
	// With the bug, it will crash with about 10 - 20 times of toggles.
	// Please see b/233157402.
	const maxNumOfToggles = 30
	for toggle := 0; toggle <= maxNumOfToggles; toggle++ {
		testing.ContextLog(ctx, "Enabling Assistant, toggle ", toggle)
		if err := assistant.Enable(ctx, tconn); err != nil {
			s.Fatal("Failed to enable Assistant: ", err)
		}
		testing.ContextLog(ctx, "Disabling Assistant, toggle ", toggle)
		if err := assistant.Disable(ctx, tconn); err != nil {
			s.Fatal("Failed to disable Assistant: ", err)
		}
	}

	// Make sure the Assistant can be enabled after the above operations.
	if err := assistant.EnableAndWaitForReady(ctx, tconn); err != nil {
		s.Fatal("Failed to enable Assistant: ", err)
	}
	defer func() {
		if err := assistant.Cleanup(ctx, s.HasError, cr, tconn); err != nil {
			s.Fatal("Failed to clean up Assistant: ", err)
		}
	}()
}
