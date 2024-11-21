// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ui contains local Tast tests that exercise Chrome's integration with the rest of ChromeOS.
package ui

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NewConnForTargetAfterSuspend,
		Desc:         "Tests re-establishing chrome connection after power suspension",
		Contacts:     []string{"chromeos-sw-engprod@google.com", "jonfan@google.com"},
		BugComponent: "b:1034649",
		Attr:         []string{"group:mainline", "group:hw_agnostic", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
	})
}

func NewConnForTargetAfterSuspend(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	url := "about:blank"
	numConns := 3
	conns := make([]*chrome.Conn, numConns)

	// Create some chrome conn against the same url.
	for i := 0; i < numConns; i++ {
		conn, err := cr.NewConn(ctx, url)
		if err != nil {
			s.Fatalf("Failed to open page %v: %v", url, err)
		}
		conns[i] = conn
	}

	testing.ContextLog(ctx, "Before power suspension")
	cmd := testexec.CommandContext(ctx, "powerd_dbus_suspend", "--delay=0", "--suspend_for_sec=3")
	if err := cmd.Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to suspend: ", err)
	}
	testing.ContextLog(ctx, "After power suspension")

	s.Log("Reconnecting to Chrome")
	if err := cr.Reconnect(ctx); err != nil {
		s.Fatal("Failed to reconnect to Chrome: ", err)
	}

	testing.ContextLog(ctx, "Try accessing NewConnForTarget through TargetID")
	for _, c := range conns {
		conn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetID(c.TargetID))
		if err != nil {
			s.Fatalf("Failed when calling cr.NewConnForTarget for conn: %v  with err: %v", c, err)
		}
		// Verify that we can indeed focus on those pages.
		if err = conn.ActivateTarget(ctx); err != nil {
			s.Fatalf("Failed to activate target with TargetID: %s with err: %v", c.TargetID, err)
		}
	}
}
