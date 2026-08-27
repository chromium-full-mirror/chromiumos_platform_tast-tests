// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package searchsuggestion contains helpers to verify
// search suggestion policy.
package searchsuggestion

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser/browserui"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestCase defines test expectations based on the policy value.
type TestCase struct {
	Name                 string
	ShouldFindAnnotation bool
	Policy               *policy.SearchSuggestEnabled
	Enabled              bool
	// TrafficShouldFind states the traffic should be found during tests.
	TrafficShouldFind []string
	// TrafficShouldNotFind states the traffic should NOT be found during tests.
	TrafficShouldNotFind []string
}

// AnnotationHashCode is the hashcode for annotation omnibox_suggest.
const AnnotationHashCode = "47815025"

const regexSearchSuggestURL = "https://www.google.com/complete/search\\?client=chrome-omni.*"

// TestCases returns the map of policy setting enum to TestCase objects
// on which the SearchSuggestEnabled policy is tested.
func TestCases() map[networkrequestmonitor.PolicySetting]TestCase {
	return map[networkrequestmonitor.PolicySetting]TestCase{
		networkrequestmonitor.PolicyDisabled: {
			Name:                 "disabled",
			ShouldFindAnnotation: false,
			Policy:               &policy.SearchSuggestEnabled{Val: false},
			Enabled:              false,
			TrafficShouldFind:    []string{},
			TrafficShouldNotFind: []string{regexSearchSuggestURL},
		},
		networkrequestmonitor.PolicyEnabled: {
			Name:                 "enabled",
			ShouldFindAnnotation: true,
			Policy:               &policy.SearchSuggestEnabled{Val: true},
			Enabled:              true,
			TrafficShouldFind:    []string{regexSearchSuggestURL},
			TrafficShouldNotFind: []string{},
		},
		networkrequestmonitor.PolicyUnset: {
			Name:                 "unset",
			ShouldFindAnnotation: true,
			Policy:               &policy.SearchSuggestEnabled{Stat: policy.StatusUnset},
			Enabled:              true,
			TrafficShouldFind:    []string{regexSearchSuggestURL},
			TrafficShouldNotFind: []string{},
		},
	}
}

// TriggerSearchSuggestion verifies suggestions are not shown when policy is off.
func TriggerSearchSuggestion(ctx context.Context, params networkrequestmonitor.OptionalServiceParams) (err error) {
	cr := params.Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		errors.Wrap(err, "failed to create Test API connection")
	}
	policyParam := TestCases()[params.PolicySetting]

	// Open a keyboard device.
	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open keyboard device")
	}
	defer keyboard.Close(ctx)

	conn, err := cr.NewConn(ctx, "")
	if err != nil {
		return errors.Wrap(err, "failed to create new Chrome connection")
	}
	defer conn.Close()

	// Try to open a tab.
	if err := keyboard.Accel(ctx, "ctrl+t"); err != nil {
		return errors.Wrap(err, "failed to write events")
	}

	// Click the address bar.
	addressBar := browserui.AddressBarFinder
	ui := uiauto.New(tconn)
	if err := uiauto.Combine("find and click the address bar",
		ui.WaitUntilExists(addressBar),
		ui.DoDefault(addressBar),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to find and click the address bar")
	}

	// GoBigSleepLint - Wait for a second before typing to make sure the module
	// for suggestions is loaded.
	testing.Sleep(ctx, time.Second)

	// Type something so suggestions pop up.
	if err := keyboard.Type(ctx, "google"); err != nil {
		return errors.Wrap(err, "failed to write events")
	}

	// Wait for the omnibox popup node.
	if err := ui.WaitUntilExists(nodewith.ClassName("OmniboxPopupViewViews"))(ctx); err != nil {
		return errors.Wrap(err, "failed to find omnibox popup")
	}

	// Get all the omnibox results.
	omniboxResults, err := ui.NodesInfo(ctx, nodewith.Role(role.ListBoxOption))
	if err != nil {
		return errors.Wrap(err, "failed to get omnibox results")
	}

	suggest := false
	for _, result := range omniboxResults {
		if strings.Contains(result.Name, "search suggestion") {
			suggest = true
			break
		}
	}

	if suggest != policyParam.Enabled {
		errors.Errorf("unexpected existence of search suggestions: got %t; want %t", suggest, policyParam.Enabled)
	}

	if suggest != policyParam.ShouldFindAnnotation {
		errors.Errorf("unexpected existence of search suggestions: got %t; want %t", suggest, policyParam.ShouldFindAnnotation)
	}

	return nil
}
