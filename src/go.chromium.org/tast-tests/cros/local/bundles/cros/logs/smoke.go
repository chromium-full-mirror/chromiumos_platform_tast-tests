// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/systemlogs"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		BugComponent: "b:1166446",
		Func:         Smoke,
		Desc:         "Tests that writing system logs succeeds",
		Contacts: []string{
			"cros-network-health-team@google.com", // Team alias
			"stevenjb@chromium.org",               // Test author
		},
		Attr:         []string{"group:mainline", "group:hw_agnostic", "informational"},
		SoftwareDeps: []string{"chrome"},
		Pre:          chrome.LoggedIn(),
	})
}

func Smoke(ctx context.Context, s *testing.State) {
	cr := s.PreValue().(*chrome.Chrome)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	const expectedKey = "CHROME VERSION"
	result, err := systemlogs.GetSystemLogs(ctx, tconn, expectedKey)
	if err != nil {
		s.Fatal("Error getting system logs: ", err)
	}
	if result == "" {
		s.Fatal("System logs result empty")
	}
}
