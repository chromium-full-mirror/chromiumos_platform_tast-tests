// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package developertools contains helpers to verify the developer tools.
package developertools

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestCase defines test expectations based on the value of the policy
// DeveloperToolsAvailability.
type TestCase struct {
	Name        string
	Value       *policy.DeveloperToolsAvailability
	WantAllowed bool
}

// TriggerDeveloperToolsAvailability verifies developer tools works as expected.
// i.e. the existence of developer tools panel using four keys combinations
// depending on the policy value.
func TriggerDeveloperToolsAvailability(ctx context.Context, param TestCase, tconn *chrome.TestConn, s *testing.State) (err error) {
	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer keyboard.Close(ctx)

	var failedKeys []string
	for _, keys := range []string{
		"Ctrl+Shift+C",
		"Ctrl+Shift+I",
		"F12",
		"Ctrl+Shift+J",
	} {
		if !s.Run(ctx, keys, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeOnErrorToFile(ctx, s.OutDir(), s.HasError, tconn, fmt.Sprintf("ui_tree_%s_%s.txt", param.Name, keys))

			// Open new tab and navigate to chrome://user-actions.
			// Here we cannot use cr.Conn, because Chrome DevTools Protocol
			// relies on DevTools.
			if err := keyboard.Accel(ctx, "Ctrl+T"); err != nil {
				s.Fatal("Failed to press Ctrl+T: ", err)
			}
			if err := keyboard.Type(ctx, "chrome://user-actions\n"); err != nil {
				s.Fatal("Failed to type chrome://user-actions: ", err)
			}

			// Check that we have access to chrome://user-actions
			// accessibility tree.
			ui := uiauto.New(tconn)
			userAction := nodewith.Name("User Action").Role(role.ColumnHeader)
			if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(userAction)(ctx); err != nil {
				s.Fatal("Failed to wait for page nodes: ", err)
			}
			// Press keys combination to open DevTools.
			if err := keyboard.Accel(ctx, keys); err != nil {
				s.Fatalf("Failed to press %s: %v", keys, err)
			}
			timeout := 15 * time.Second
			elements := nodewith.Name("Elements").Role(role.Tab)
			if param.WantAllowed {
				if err := ui.WithTimeout(timeout).WaitUntilExists(elements)(ctx); err != nil {
					s.Error("Failed to wait for DevTools: ", err)
				}
			} else {
				if err := policyutil.VerifyNotExists(ctx, tconn, elements, timeout); err != nil {
					s.Errorf("Failed to verify that DevTools are not available after %s: %s", timeout, err)
				}
			}
		}) {
			failedKeys = append(failedKeys, keys)
		}
	}

	if len(failedKeys) > 0 {
		return errors.Errorf("some subtests did not pass: %v", failedKeys)
	}

	return nil
}
