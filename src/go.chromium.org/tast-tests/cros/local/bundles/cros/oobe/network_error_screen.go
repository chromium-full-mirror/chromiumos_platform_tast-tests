// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package oobe

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NetworkErrorScreen,
		Desc:         "Verifies that the Network Error Screen loads correctly in OOBE without crashing",
		Contacts:     []string{"yoshiki@google.com", "cros-oobe@google.com"},
		BugComponent: "b:1263090", // ChromeOS > Software > OOBE
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
	})
}

func NetworkErrorScreen(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	// Start Chrome in OOBE mode (NoLogin)
	cr, err := chrome.New(ctx, chrome.NoLogin())
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	// Connect to the OOBE connection
	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to connect to OOBE: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	ui := uiauto.New(tconn)

	// Trigger the Network Error Screen using JS
	// If the internet-shared bug is present, this will cause the UI to crash/hang
	// and the 'error-message' element will never fully render.
	s.Log("Attempting to show the error-message screen")
	if err := oobeConn.Eval(ctx, `Oobe.getInstance().showScreen({id: 'error-message', data: {}})`, nil); err != nil {
		s.Fatal("Failed to evaluate JS to show error screen: ", err)
	}

	// network-list-item is the component that should trigger the crash
	// If there's an infinite spinner, this element won't be rendered.
	networkList := nodewith.ClassName("network-list-item").First()

	// Wait for the UI to update. If it crashes, this WaitUntilExists will timeout.
	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(networkList)(ctx); err != nil {
		s.Log("The network list items failed to render, confirming the Polymer crash")
		s.Fatal("Failed to find network items: ", err)
	}

	s.Log("Network error screen successfully rendered (no crash occurred)")
}
