// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package searchsuggestion contains helpers to verify
// search suggestion policy.
package searchsuggestion

import (
	"context"
	"net/http/httptest"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser/browserui"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
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
}

// AnnotationHashCode is the hashcode for annotation omnibox_suggest.
const AnnotationHashCode = "47815025"

// GetTestCases returns the list of TestCase objects on which
// SearchSuggestEnabled policy is tested.
func GetTestCases() []TestCase {
	// Reordering the TestCase objects in the returned list may break tests.
	return []TestCase{
		{
			Name:                 "disabled",
			ShouldFindAnnotation: false,
			Policy:               &policy.SearchSuggestEnabled{Val: false},
			Enabled:              false,
		},
		{
			Name:                 "enabled",
			ShouldFindAnnotation: true,
			Policy:               &policy.SearchSuggestEnabled{Val: true},
			Enabled:              true,
		},
		{
			Name:                 "unset",
			ShouldFindAnnotation: true,
			Policy:               &policy.SearchSuggestEnabled{Stat: policy.StatusUnset},
			Enabled:              true,
		},
	}
}

// TriggerSearchSuggestion verifies suggestions are not shown when policy is off.
func TriggerSearchSuggestion(ctx context.Context, _ *chrome.Chrome, br *browser.Browser, _ *httptest.Server, tconn *chrome.TestConn, paramIndex int) (err error) {
	param := GetTestCases()[paramIndex]

	// Open a keyboard device.
	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open keyboard device")
	}
	defer keyboard.Close(ctx)

	conn, err := br.NewConn(ctx, "")
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
	omniboxResults, err := ui.NodesInfo(ctx, nodewith.ClassName("OmniboxResultView"))
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

	if suggest != param.Enabled {
		errors.Errorf("unexpected existence of search suggestions: got %t; want %t", suggest, param.Enabled)
	}

	if suggest != param.ShouldFindAnnotation {
		errors.Errorf("unexpected existence of search suggestions: got %t; want %t", suggest, param.ShouldFindAnnotation)
	}

	return nil
}
