// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ClipboardHistoryEnabled,
		Desc:         "Checks that ClipboardHistoryEnabled policy controls ChromeOS clipboard history",
		Contacts: []string{
			"aidazolic@google.com",
			"ghostbusters-eng@google.com",
			"multipaste-eng@google.com",
			"cros-system-ui-eng@google.com",
			"chromeos-consumer-engprod@google.com",
			"newcomer@google.com",
		},
		BugComponent: "b:1268414", // ChromeOS > Software > System UI Surfaces > EnhancedClipboard
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ClipboardHistoryEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// ClipboardHistoryEnabled tests the behavior of the ClipboardHistoryEnabled policy.
func ClipboardHistoryEnabled(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)

	const (
		policyEnabled  = 0
		policyDisabled = 1

		clipboardHistoryItemClassName = "ClipboardHistoryTextItemView"
		disabledByPolicyToastText     = "Clipboard history was disabled by your IT admin"
	)

	for _, param := range []struct {
		name        string
		wantEnabled bool
		value       *policy.ClipboardHistoryEnabled
	}{
		{
			name:        "unset",
			wantEnabled: true,
			value:       &policy.ClipboardHistoryEnabled{Stat: policy.StatusUnset},
		},
		{
			name:        "enabled",
			wantEnabled: true,
			value:       &policy.ClipboardHistoryEnabled{Val: policyEnabled},
		},
		{
			name:        "disabled",
			wantEnabled: false,
			value:       &policy.ClipboardHistoryEnabled{Val: policyDisabled},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to update and verify policy: ", err)
			}

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_clipboard_history_"+param.name)

			// Populate clipboard buffer with two text items.
			if err := ash.SetClipboard(ctx, tconn, "item1"); err != nil {
				s.Fatal("Failed to write item1 to clipboard: ", err)
			}
			if err := ash.WaitUntilClipboardText(ctx, tconn, "item1"); err != nil {
				s.Fatal("Failed waiting for item1 in clipboard: ", err)
			}
			if err := ash.SetClipboard(ctx, tconn, "item2"); err != nil {
				s.Fatal("Failed to write item2 to clipboard: ", err)
			}
			if err := ash.WaitUntilClipboardText(ctx, tconn, "item2"); err != nil {
				s.Fatal("Failed waiting for item2 in clipboard: ", err)
			}

			// Press Search+V / Launcher+V shortcut to trigger clipboard history.
			if err := kb.Accel(ctx, "Search+V"); err != nil {
				s.Fatal("Failed to press Search+V: ", err)
			}

			menuItem := nodewith.Role(role.MenuItem).HasClass(clipboardHistoryItemClassName)
			menuItem1 := menuItem.Name("item1")
			menuItem2 := menuItem.Name("item2")
			disabledToast := nodewith.Name(disabledByPolicyToastText).Ancestor(nodewith.HasClass("ToastOverlay"))

			if param.wantEnabled {
				if err := uiauto.Combine("verify clipboard history menu opens and close it",
					ui.WaitUntilExists(menuItem1),
					ui.WaitUntilExists(menuItem2),
					ui.WaitUntilGone(disabledToast),
					kb.AccelAction("Esc"),
					ui.WaitUntilGone(menuItem.First()),
				)(ctx); err != nil {
					s.Fatal("Failed to verify enabled clipboard history menu: ", err)
				}
			} else {
				if err := uiauto.Combine("verify disabled toast appears and menu does not open",
					ui.WaitUntilExists(disabledToast),
					ui.EnsureGoneFor(menuItem.First(), time.Second),
				)(ctx); err != nil {
					s.Fatal("Failed to verify disabled clipboard history behavior: ", err)
				}

				// Verify single-item clipboard data still works when history is disabled.
				clipData, err := ash.ClipboardTextData(ctx, tconn)
				if err != nil {
					s.Fatal("Failed to read clipboard text: ", err)
				}
				if clipData != "item2" {
					s.Fatalf("Unexpected clipboard text data: got %q, want %q", clipData, "item2")
				}
			}
		})
	}
}
