// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assistant

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/assistant"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         StartupInSignedOutMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Starts Google Assistant service in signed-out mode and checks the running status",
		Contacts: []string{
			"cros-assistive@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		BugComponent: "b:905229", // ChromeOS > Software > Assistive
		Attr: []string{
			"group:mainline",
			"group:hw_agnostic",
		},
		Pre:          assistant.VerboseLoggingEnabled(),
		SoftwareDeps: []string{"chrome", "chrome_internal"},
	})
}

func StartupInSignedOutMode(ctx context.Context, s *testing.State) {
	cr := s.PreValue().(*chrome.Chrome)

	// Create test API connection.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	// Enable the Assistant and wait for the ready signal.
	if err := assistant.EnableAndWaitForReady(ctx, tconn); err != nil {
		s.Fatal("Failed to enable Assistant: ", err)
	}
	defer func() {
		if err := assistant.Cleanup(ctx, s.HasError, cr, tconn); err != nil {
			s.Fatal("Failed to disable Assistant: ", err)
		}
	}()
}
