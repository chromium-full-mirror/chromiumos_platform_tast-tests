// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deskscuj

import (
	"strings"
	"testing"
)

func TestIsUnmodifiedOmniboxText(t *testing.T) {
	e := &omniboxEnv{hostPort: "127.0.0.1:12345"}
	for _, tc := range []struct {
		text string
		want bool
	}{
		// Unmodified page URLs, as shown in the omnibox.
		{"127.0.0.1:12345/p11", true},
		{"docs.google.com/document/d/19R_RWgGAqcHtgXic_YPQho7EwZyUAuUZyBq4n_V-BJ0/edit?usp=sharing", true},
		{"webglsamples.org/aquarium/aquarium.html?numFish=1000", true},
		// Empty text isn't the page URL.
		{"", false},
		// Strings typed by the test.
		{"omnit", false},
		{"omnitest 0", false},
		{"OMNITEST 1", false},
		{"127.0.0.1:12345/p0", false},
		{"127.0.0.1:12345/p1", false},
		{aimPrompt, false},
		// Anything that looks like a query.
		{"virtual desks", false},
		{"desks", false},
		{"a.b c", false},
	} {
		if got := e.isUnmodifiedOmniboxText(tc.text); got != tc.want {
			t.Errorf("isUnmodifiedOmniboxText(%q) = %t; want %t", tc.text, got, tc.want)
		}
	}

	// Every typed string must be rejected.
	for _, s := range e.typedStrings() {
		if e.isUnmodifiedOmniboxText(s) {
			t.Errorf("isUnmodifiedOmniboxText(%q) = true for a typed string", s)
		}
	}
}

func TestDisablesFeature(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"/opt/google/chrome/chrome", "--disable-features=WebUIOmniboxPopup,WebUIOmniboxAimPopup,WebUIOmniboxFullPopup"}, true},
		{[]string{"--disable-features=WebUIOmniboxAimPopup"}, true},
		{[]string{"--disable-features=Foo", "--disable-features=WebUIOmniboxAimPopup<Trial"}, true},
		{[]string{"--enable-features=WebUIOmniboxAimPopup"}, false},
		{[]string{"--disable-features=WebUIOmniboxAimPopupFoo,WebUIOmniboxPopup"}, false},
		{nil, false},
	} {
		if got := disablesFeature(tc.args, aimPopupFeature); got != tc.want {
			t.Errorf("disablesFeature(%q, %q) = %t; want %t", tc.args, aimPopupFeature, got, tc.want)
		}
	}
}

func TestParseAimEligibility(t *testing.T) {
	// Text lines as read from chrome://omnibox/aim-eligibility on R157.
	page := func(overall, aim string) []string {
		return []string{
			"Omnibox Debug Page",
			aimEligibilityHeader,
			overall,
			"AIModeSettings Policy:",
			"✓ Allowed",
			"Default Search Engine:",
			"✓ Google",
			"Eligibility Response Source:",
			"Server",
			"Auth Type:",
			"Cookie",
			"AIM:",
			aim,
		}
	}
	for _, tc := range []struct {
		name         string
		lines        []string
		wantEligible bool
		wantOK       bool
		wantDetails  []string
	}{
		{
			name:         "eligible",
			lines:        page("✓ Eligible", "✓ Eligible"),
			wantEligible: true,
			wantOK:       true,
			wantDetails:  []string{"Overall: ✓ Eligible", "Default Search Engine: ✓ Google", "AIM: ✓ Eligible"},
		},
		{
			name:        "not eligible",
			lines:       page("✗ Not Eligible", "✗ Not Eligible"),
			wantOK:      true,
			wantDetails: []string{"Overall: ✗ Not Eligible", "Auth Type: Cookie", "AIM: ✗ Not Eligible"},
		},
		{
			name:  "not loaded",
			lines: []string{"Omnibox Debug Page", "Loading"},
		},
		{
			name:  "header without value",
			lines: []string{aimEligibilityHeader},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eligible, details, ok := parseAimEligibility(tc.lines)
			if eligible != tc.wantEligible || ok != tc.wantOK {
				t.Errorf("parseAimEligibility() = (%t, _, %t); want (%t, _, %t)", eligible, ok, tc.wantEligible, tc.wantOK)
			}
			for _, d := range tc.wantDetails {
				if !strings.Contains(details, d) {
					t.Errorf("parseAimEligibility() details %q don't contain %q", details, d)
				}
			}
		})
	}
}
