// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assistant

import (
	"context"
	"time"

	assistantRemote "go.chromium.org/tast-tests/cros/common/assistant"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/local/assistant"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         StartupInSignedInMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Starts Google Assistant service in signed-in mode and checks the running status",
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
		SoftwareDeps: []string{"chrome", "chrome_internal", "gaia"},
		VarDeps:      []string{assistantRemote.AccountPoolVarName},
		Timeout:      chrome.GAIALoginTimeout + time.Minute,
	})
}

func StartupInSignedInMode(ctx context.Context, s *testing.State) {
	opts := []chrome.Option{
		chrome.GAIALoginPool(dma.CredsFromPool(assistantRemote.AccountPoolVarName)),
		assistant.VerboseLogging()}

	// Start Chrome browser and log in using a test account.
	cr, err := chrome.New(
		ctx,
		opts...,
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

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
