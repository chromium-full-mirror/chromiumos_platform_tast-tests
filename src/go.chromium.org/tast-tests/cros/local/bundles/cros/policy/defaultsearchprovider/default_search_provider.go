// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package defaultsearchprovider contains helpers to verify
// default search provider enabled policy.
package defaultsearchprovider

import (
	"context"
	"net/http/httptest"
	"strings"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/browser/browserui"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
)

// TestCase defines test expectations based on the policy value.
type TestCase struct {
	// Name is the subtest name.
	Name string
	// Enabled is the expected enabled state of the policy.
	Enabled bool
	// Value is the policy value.
	Value *policy.DefaultSearchProviderEnabled
	// ShouldFindAnnotation states whether navigation_url_loader annotation
	// should be found in the net-export log.
	ShouldFindAnnotation bool
}

// GetTestCases returns the list of TestCase objects on which
// DefaultSearchProviderEnabled policy is tested.
func GetTestCases() []TestCase {
	// Reordering the TestCase objects in the returned list may break tests.
	return []TestCase{
		{
			Name:    "disabled",
			Enabled: false,
			Value:   &policy.DefaultSearchProviderEnabled{Val: false},
			// When searching for “abc” Annotation is recorded even when policy is set
			// to false, because a Url is loaded with http://abc vs when policy is set
			// to true/unset, URL loaded is http://google.com/q=abc.
			ShouldFindAnnotation: true,
		},
		{
			Name:                 "enabled",
			Enabled:              true,
			Value:                &policy.DefaultSearchProviderEnabled{Val: true},
			ShouldFindAnnotation: true,
		},
		{
			Name:                 "unset",
			Enabled:              true,
			Value:                &policy.DefaultSearchProviderEnabled{Stat: policy.StatusUnset},
			ShouldFindAnnotation: true,
		},
	}
}

const (
	// AnnotationHashCode is the hashcode for annotation navigation_url_loader.
	AnnotationHashCode = "63171670"
	// search engine checked in the test
	defaultSearchEngine = "google.com"
)

// TriggerDefaultSearchProvider verifies the default search provider policy.
func TriggerDefaultSearchProvider(ctx context.Context, _ *chrome.Chrome, br *browser.Browser, _ *httptest.Server, tconn *chrome.TestConn, paramIndex int) (err error) {
	param := GetTestCases()[paramIndex]

	addressBarNode := browserui.AddressBarFinder

	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open keyboard device")
	}
	defer kb.Close(ctx)

	// Open an empty page.
	// Use chrome://newtab to open new tab page (see https://crbug.com/1188362#c19).
	conn, err := br.NewConn(ctx, "chrome://newtab/")
	if err != nil {
		return errors.Wrap(err, "failed to create new Chrome connection")
	}
	defer conn.Close()

	uiauto := uiauto.New(tconn)

	// Connect to Test API of the used browser to clear the browser
	// history. We need a second connection as the clearing of the
	// history has to be executed from the used browser while the
	// uiauto package needs a connection to the ash browser.
	tconn2, err := br.TestAPIConn(ctx)
	if err != nil {
		errors.Wrap(err, "failed to create Test API connection")
	}

	// Clear the browser history, otherwise the previous search results can
	// interfere with the test.
	if err := tconn2.Eval(ctx, `tast.promisify(chrome.browsingData.removeHistory({"since": 0}))`, nil); err != nil {
		errors.Wrap(err, "failed to clear browsing history")
	}

	// Click the address and search bar.
	if err := uiauto.LeftClick(addressBarNode)(ctx); err != nil {
		return errors.Wrap(err, "could not find the address bar")
	}

	// Type something.
	if err := kb.Type(ctx, "vy6ys\n"); err != nil {
		return errors.Wrap(err, "failed to write events")
	}

	// Wait for the page to load.
	if err := uiauto.WaitForLocation(addressBarNode)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for location change")
	}

	// Find the address bar.
	nodeInfo, err := uiauto.Info(ctx, addressBarNode)
	if err != nil {
		return errors.Wrap(err, "could not get new info for the address bar")
	}
	location := nodeInfo.Value

	defaultSearchEngineUsed := strings.Contains(location, defaultSearchEngine)
	if param.Enabled != defaultSearchEngineUsed {
		return errors.Errorf("unexpected usage of search engine: got %t; want %t (got %q; want %q)", defaultSearchEngineUsed, param.Enabled, location, defaultSearchEngine)
	}
	return nil
}
