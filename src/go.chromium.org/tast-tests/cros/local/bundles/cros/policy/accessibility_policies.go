// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/testing"
)

type accessibilityTestCase struct {
	name      string          // name is the subtest name.
	policyKey string          // policyKey is the key for the policy value in chrome.accessibilityFeatures map.
	wantValue bool            // wantValue is the expected value of the policy once set.
	policies  []policy.Policy // policies is the policies values.
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AccessibilityPolicies,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks set values for the Accessability polices in the chrome.accessibilityFeatures map",
		Contacts: []string{
			"chromium-accessibility@chromium.org",
		},
		// ChromeOS Public Tracker > Experiences > Accessibility
		BugComponent: "b:1272759",
		SoftwareDeps: []string{"chrome"},
		// autoclick case needs to be disabled.
		Attr:    []string{"group:hw_agnostic"},
		Fixture: fixture.ChromePolicyLoggedIn,
		Params: []testing.Param{
			// TODO(crbug.com/1186655): Find a way to close/avoid the dialog about disabling autoclick.
			{
				Name: "autoclick",
				Val: []accessibilityTestCase{
					{
						name:      "enabled",
						policyKey: "autoclick",
						wantValue: true,
						policies:  []policy.Policy{&policy.AutoclickEnabled{Val: true}},
					},
					{
						name:      "disabled",
						policyKey: "autoclick",
						wantValue: false,
						policies:  []policy.Policy{&policy.AutoclickEnabled{Val: false}},
					},
					{
						name:      "unset",
						policyKey: "autoclick",
						wantValue: false,
						policies:  []policy.Policy{&policy.AutoclickEnabled{Stat: policy.StatusUnset}},
					},
				},
				ExtraSearchFlags: []*testing.StringPair{
					pci.SearchFlag(&policy.AutoclickEnabled{}, pci.VerifiedFunctionalityJS),
				},
			},
			{
				Name:      "caret_highlight",
				ExtraAttr: []string{"group:golden_tier"},
				Val: []accessibilityTestCase{
					{
						name:      "enabled",
						policyKey: "caretHighlight",
						wantValue: true,
						policies:  []policy.Policy{&policy.CaretHighlightEnabled{Val: true}},
					},
					{
						name:      "disabled",
						policyKey: "caretHighlight",
						wantValue: false,
						policies:  []policy.Policy{&policy.CaretHighlightEnabled{Val: false}},
					},
					{
						name:      "unset",
						policyKey: "caretHighlight",
						wantValue: false,
						policies:  []policy.Policy{&policy.CaretHighlightEnabled{Stat: policy.StatusUnset}},
					},
				},
				ExtraSearchFlags: []*testing.StringPair{
					pci.SearchFlag(&policy.CaretHighlightEnabled{}, pci.VerifiedFunctionalityJS),
				},
			},
			{
				Name:      "cursor_highlight",
				ExtraAttr: []string{"group:golden_tier"},
				Val: []accessibilityTestCase{
					{
						name:      "enabled",
						policyKey: "cursorHighlight",
						wantValue: true,
						policies:  []policy.Policy{&policy.CursorHighlightEnabled{Val: true}},
					},
					{
						name:      "disabled",
						policyKey: "cursorHighlight",
						wantValue: false,
						policies:  []policy.Policy{&policy.CursorHighlightEnabled{Val: false}},
					},
					{
						name:      "unset",
						policyKey: "cursorHighlight",
						wantValue: false,
						policies:  []policy.Policy{&policy.CursorHighlightEnabled{Stat: policy.StatusUnset}},
					},
				},
				ExtraSearchFlags: []*testing.StringPair{
					pci.SearchFlag(&policy.CursorHighlightEnabled{}, pci.VerifiedFunctionalityJS),
				},
			},
			{
				Name:      "docked_magnifier",
				ExtraAttr: []string{"group:golden_tier"},
				Val: []accessibilityTestCase{
					{
						// Value 1 for this policy is allowed but does not
						// enable docked magnifier. Hence, the value for the
						// 'dockedMagnifier' key is expected to be false.
						name:      "enabled-full-screen",
						policyKey: "dockedMagnifier",
						wantValue: false, // Negative test case as this value applies to screenMagnifier.
						policies:  []policy.Policy{&policy.ScreenMagnifierType{Val: 1}},
					},
					{
						name:      "enabled-docked",
						policyKey: "dockedMagnifier",
						wantValue: true,
						policies:  []policy.Policy{&policy.ScreenMagnifierType{Val: 2}},
					},
					{
						name:      "disabled",
						policyKey: "dockedMagnifier",
						wantValue: false,
						policies:  []policy.Policy{&policy.ScreenMagnifierType{Val: 0}},
					},
					{
						name:      "unset",
						policyKey: "dockedMagnifier",
						wantValue: false,
						policies:  []policy.Policy{&policy.ScreenMagnifierType{Stat: policy.StatusUnset}},
					},
				},
				ExtraSearchFlags: []*testing.StringPair{
					pci.SearchFlag(&policy.ScreenMagnifierType{}, pci.VerifiedFunctionalityJS),
				},
			},
			{
				Name:      "focus_highlight",
				ExtraAttr: []string{"group:golden_tier"},
				Val: []accessibilityTestCase{
					{
						name:      "enabled",
						policyKey: "focusHighlight",
						wantValue: true,
						policies:  []policy.Policy{&policy.KeyboardFocusHighlightEnabled{Val: true}},
					},
					{
						name:      "disabled",
						policyKey: "focusHighlight",
						wantValue: false,
						policies:  []policy.Policy{&policy.KeyboardFocusHighlightEnabled{Val: false}},
					},
					{
						name:      "unset",
						policyKey: "focusHighlight",
						wantValue: false,
						policies:  []policy.Policy{&policy.KeyboardFocusHighlightEnabled{Stat: policy.StatusUnset}},
					},
				},
				ExtraSearchFlags: []*testing.StringPair{
					pci.SearchFlag(&policy.KeyboardFocusHighlightEnabled{}, pci.VerifiedFunctionalityJS),
				},
			},
			{
				Name:      "high_contrast",
				ExtraAttr: []string{"group:golden_tier"},
				Val: []accessibilityTestCase{
					{
						name:      "enabled",
						policyKey: "highContrast",
						wantValue: true,
						policies:  []policy.Policy{&policy.HighContrastEnabled{Val: true}},
					},
					{
						name:      "disabled",
						policyKey: "highContrast",
						wantValue: false,
						policies:  []policy.Policy{&policy.HighContrastEnabled{Val: false}},
					},
					{
						name:      "unset",
						policyKey: "highContrast",
						wantValue: false,
						policies:  []policy.Policy{&policy.HighContrastEnabled{Stat: policy.StatusUnset}},
					},
				},
				ExtraSearchFlags: []*testing.StringPair{
					pci.SearchFlag(&policy.HighContrastEnabled{}, pci.VerifiedFunctionalityJS),
				},
			},
			{
				Name:      "large_cursor",
				ExtraAttr: []string{"group:golden_tier"},
				Val: []accessibilityTestCase{
					{
						name:      "enabled",
						policyKey: "largeCursor",
						wantValue: true,
						policies:  []policy.Policy{&policy.LargeCursorEnabled{Val: true}},
					},
					{
						name:      "disabled",
						policyKey: "largeCursor",
						wantValue: false,
						policies:  []policy.Policy{&policy.LargeCursorEnabled{Val: false}},
					},
					{
						name:      "unset",
						policyKey: "largeCursor",
						wantValue: false,
						policies:  []policy.Policy{&policy.LargeCursorEnabled{Stat: policy.StatusUnset}},
					},
				},
				ExtraSearchFlags: []*testing.StringPair{
					pci.SearchFlag(&policy.LargeCursorEnabled{}, pci.VerifiedFunctionalityJS),
				},
			},
			{
				Name:      "screen_magnifier",
				ExtraAttr: []string{"group:golden_tier"},
				Val: []accessibilityTestCase{
					{
						name:      "enabled-full-screen",
						policyKey: "screenMagnifier",
						wantValue: true,
						policies:  []policy.Policy{&policy.ScreenMagnifierType{Val: 1}},
					},
					{
						// Value 2 for this policy is allowed but enables
						// docked magnifier. Hence, the value for the
						// 'screenMagnifier' key is expected to be false.
						name:      "enabled-docker",
						policyKey: "screenMagnifier",
						wantValue: false, // Negative test case as this value applies to dockedMagnifier.
						policies:  []policy.Policy{&policy.ScreenMagnifierType{Val: 2}},
					},
					{
						name:      "disabled",
						policyKey: "screenMagnifier",
						wantValue: false,
						policies:  []policy.Policy{&policy.ScreenMagnifierType{Val: 0}},
					},
					{
						name:      "unset",
						policyKey: "screenMagnifier",
						wantValue: false,
						policies:  []policy.Policy{&policy.ScreenMagnifierType{Stat: policy.StatusUnset}},
					},
				},
				ExtraSearchFlags: []*testing.StringPair{
					pci.SearchFlag(&policy.ScreenMagnifierType{}, pci.VerifiedFunctionalityJS),
				},
			},
			{
				Name:      "select_to_speak",
				ExtraAttr: []string{"group:golden_tier"},
				Val: []accessibilityTestCase{
					{
						name:      "enabled",
						policyKey: "selectToSpeak",
						wantValue: true,
						policies:  []policy.Policy{&policy.SelectToSpeakEnabled{Val: true}},
					},
					{
						name:      "disabled",
						policyKey: "selectToSpeak",
						wantValue: false,
						policies:  []policy.Policy{&policy.SelectToSpeakEnabled{Val: false}},
					},
					{
						name:      "unset",
						policyKey: "selectToSpeak",
						wantValue: false,
						policies:  []policy.Policy{&policy.SelectToSpeakEnabled{Stat: policy.StatusUnset}},
					},
				},
				ExtraSearchFlags: []*testing.StringPair{
					pci.SearchFlag(&policy.SelectToSpeakEnabled{}, pci.VerifiedFunctionalityJS),
				},
			},
			{
				Name:      "spoken_feedback",
				ExtraAttr: []string{"group:golden_tier"},
				Timeout:   3 * time.Minute,
				Val: []accessibilityTestCase{
					{
						name:      "enabled",
						policyKey: "spokenFeedback",
						wantValue: true,
						policies:  []policy.Policy{&policy.SpokenFeedbackEnabled{Val: true}},
					},
					{
						name:      "disabled",
						policyKey: "spokenFeedback",
						wantValue: false,
						policies:  []policy.Policy{&policy.SpokenFeedbackEnabled{Val: false}},
					},
					{
						name:      "unset",
						policyKey: "spokenFeedback",
						wantValue: false,
						policies:  []policy.Policy{&policy.SpokenFeedbackEnabled{Stat: policy.StatusUnset}},
					},
				},
				ExtraSearchFlags: []*testing.StringPair{
					pci.SearchFlag(&policy.SpokenFeedbackEnabled{}, pci.VerifiedFunctionalityJS),
				},
			},
			{
				Name:      "sticky_keys",
				ExtraAttr: []string{"group:golden_tier"},
				Val: []accessibilityTestCase{
					{
						name:      "enabled",
						policyKey: "stickyKeys",
						wantValue: true,
						policies:  []policy.Policy{&policy.StickyKeysEnabled{Val: true}},
					},
					{
						name:      "disabled",
						policyKey: "stickyKeys",
						wantValue: false,
						policies:  []policy.Policy{&policy.StickyKeysEnabled{Val: false}},
					},
					{
						name:      "unset",
						policyKey: "stickyKeys",
						wantValue: false,
						policies:  []policy.Policy{&policy.StickyKeysEnabled{Stat: policy.StatusUnset}},
					},
				},
				ExtraSearchFlags: []*testing.StringPair{
					pci.SearchFlag(&policy.StickyKeysEnabled{}, pci.VerifiedFunctionalityJS),
				},
			},
			{
				Name:      "virtual_keyboard",
				ExtraAttr: []string{"group:golden_tier"},
				Timeout:   3 * time.Minute,
				Val: []accessibilityTestCase{
					{
						name:      "enabled",
						policyKey: "virtualKeyboard",
						wantValue: true,
						policies:  []policy.Policy{&policy.VirtualKeyboardEnabled{Val: true}},
					},
					{
						name:      "disabled",
						policyKey: "virtualKeyboard",
						wantValue: false,
						policies:  []policy.Policy{&policy.VirtualKeyboardEnabled{Val: false}},
					},
					{
						name:      "unset",
						policyKey: "virtualKeyboard",
						wantValue: false,
						policies:  []policy.Policy{&policy.VirtualKeyboardEnabled{Stat: policy.StatusUnset}},
					},
				},
				ExtraSearchFlags: []*testing.StringPair{
					pci.SearchFlag(&policy.VirtualKeyboardEnabled{}, pci.VerifiedFunctionalityJS),
				},
			},
		},
	})
}

// AccessibilityPolicies checks that accessibility policies have the correct
// value in chrome.accessibilityFeatures.
func AccessibilityPolicies(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Connect to Test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	tcs := s.Param().([]accessibilityTestCase)

	for _, tc := range tcs {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to reset Chrome: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, tc.policies); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Run actual test.
			script := fmt.Sprintf(`(async () => {
				let result = await tast.promisify(tast.bind(chrome.accessibilityFeatures['%s'], "get"))({});
				return result.value;
			  })()`, tc.policyKey)

			var policyValue bool
			if err := tconn.Eval(ctx, script, &policyValue); err != nil {
				s.Fatalf("Failed to retrieve %q enabled value: %s", tc.policyKey, err)
			}

			if policyValue != tc.wantValue {
				s.Errorf("Unexpected value of chrome.accessibilityFeatures[%q]: got %t; want %t", tc.policyKey, policyValue, tc.wantValue)
			}
		})
	}
}
