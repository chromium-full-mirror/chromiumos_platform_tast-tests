// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package deskscuj

import (
	"slices"
	"strings"
)

const (
	// aimPrompt is the ~20 character prompt typed into the AI Mode popup.
	// It is never submitted.
	aimPrompt = "how do virtual desks"

	// aimEligibilityHeader is the heading on the AI Mode eligibility page
	// (chrome://omnibox/aim-eligibility) that is followed by the overall
	// eligibility.
	aimEligibilityHeader = "AIM Eligibility Diagnostic"

	// aimPopupFeature is the feature that renders the AI Mode popup with
	// WebUI. Without it, the AI Mode button navigates to Google Search.
	aimPopupFeature = "WebUIOmniboxAimPopup"
)

// aimEligibilityLabels are the breakdown entries logged from the
// eligibility page. Each label is followed by its value on the next line.
var aimEligibilityLabels = []string{
	"AIModeSettings Policy:",
	"Default Search Engine:",
	"Eligibility Response Source:",
	"Auth Type:",
	"AIM:",
}

// disablesFeature reports whether the command line args disable feature
// with --disable-features.
func disablesFeature(args []string, feature string) bool {
	const flag = "--disable-features="
	for _, a := range args {
		features, ok := strings.CutPrefix(a, flag)
		if !ok {
			continue
		}
		for _, f := range strings.Split(features, ",") {
			// Entries may have a field trial suffix, e.g. "Feature<Trial".
			if i := strings.IndexAny(f, "<:"); i >= 0 {
				f = f[:i]
			}
			if strings.TrimSpace(f) == feature {
				return true
			}
		}
	}
	return false
}

// parseAimEligibility parses the text lines of the eligibility page. It
// returns the overall eligibility, a summary of the breakdown, and whether
// the overall eligibility was found.
func parseAimEligibility(lines []string) (eligible bool, details string, ok bool) {
	var parts []string
	for i := 0; i+1 < len(lines); i++ {
		if lines[i] == aimEligibilityHeader {
			// The overall state, e.g. "✓ Eligible" or "✗ Not Eligible".
			eligible = strings.HasSuffix(lines[i+1], "Eligible") && !strings.Contains(lines[i+1], "Not Eligible")
			parts = append(parts, "Overall: "+lines[i+1])
			ok = true
		} else if slices.Contains(aimEligibilityLabels, lines[i]) {
			parts = append(parts, lines[i]+" "+lines[i+1])
		}
	}
	return eligible, strings.Join(parts, "; "), ok
}

// typedStrings returns every string the test types into the omnibox
// outside of a new tab: the corpus (which includes "omnit", typed by the
// desk-3 classic step) and the AI Mode prompt.
func (e *omniboxEnv) typedStrings() []string {
	return append(e.corpus(), aimPrompt)
}

// isUnmodifiedOmniboxText reports whether v looks like an unmodified page
// URL rather than text typed by the test. The AI Mode button submits
// modified text as a search, so it must only be clicked when this returns
// true.
func (e *omniboxEnv) isUnmodifiedOmniboxText(v string) bool {
	if v == "" || strings.ContainsAny(v, " \t") || !strings.Contains(v, ".") {
		return false
	}
	for _, t := range e.typedStrings() {
		if strings.EqualFold(v, t) {
			return false
		}
	}
	return true
}
