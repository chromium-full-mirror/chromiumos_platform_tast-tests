// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package shortcutcustomization contains drivers for controlling the ui of shortcut customization SWA.
package shortcutcustomization

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ShortcutKeys represents shortcut keys and its role.
type ShortcutKeys struct {
	Keys string
	Role role.Role
}

// Launch starts the Shortcut Customization app via ctrl+alt+/.
func Launch(ctx context.Context, tconn *chrome.TestConn) (*nodewith.Finder, error) {
	// Set up keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find keyboard")
	}
	defer kb.Close(ctx)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Launch Shortcut customization app with ctrl+search+s.
		if err := kb.Accel(ctx, "Ctrl+Search+S"); err != nil {
			return errors.Wrap(err, "failed pressing ctrl+search+s")
		}
		// Verify Shortcut customization app is launched.
		if err = ash.WaitForApp(ctx, tconn, apps.ShortcutCustomization.ID, 20*time.Second); err != nil {
			return errors.Wrap(err, "could not find app in shelf after launch")
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Minute}); err != nil {
		return nil, errors.Wrap(err, "failed launching Shortcut customization app")
	}

	ui := uiauto.New(tconn).WithTimeout(20 * time.Second)

	shortcutCustomizationRootNode := nodewith.Name(apps.ShortcutCustomization.Name).Role(role.Window)
	if err := ui.WaitUntilExists(shortcutCustomizationRootNode)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to find shortcut customization app")
	}

	return shortcutCustomizationRootNode, nil
}

// VerifyShortcutCustomizationIsLaunched checks that the app is open and all essential elements are in place.
func VerifyShortcutCustomizationIsLaunched(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context) error {
	if err := ash.WaitForApp(ctx, tconn, apps.ShortcutCustomization.ID, time.Minute); err != nil {
		return errors.Wrap(err, "could not find app in shelf after launch")
	}
	// Verify categories exist.
	for _, category := range []string{"General", "Device", "Browser", "Text", "Windows and desks", "Accessibility"} {
		if err := uiauto.Combine(fmt.Sprintf("Verify %q category exists", category),
			ui.WaitUntilExists(nodewith.Name(category).Role(role.Button)),
		)(ctx); err != nil {
			return errors.Wrapf(err, "failed to find %q category", category)
		}
	}
	// Verify essential elements exist.
	keyboardsettinglink := nodewith.Name("Keyboard settings").Role(role.Link)
	searchBar := nodewith.Name("Search shortcuts").Role(role.SearchBox)
	if err := uiauto.Combine("Verify essential elements exist",
		ui.WaitUntilExists(keyboardsettinglink),
		ui.WaitUntilExists(searchBar),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to find element")
	}

	return nil
}

// VerifySubcategory checks that the subcategories in each category exists.
func VerifySubcategory(ctx context.Context, ui *uiauto.Context, subcategories []string) error {
	for _, subcategory := range subcategories {
		if err := uiauto.Combine(fmt.Sprintf("Verify %q subcategories exists", subcategory),
			ui.WaitUntilExists(nodewith.Name(subcategory).Role(role.StaticText)),
		)(ctx); err != nil {
			return errors.Wrapf(err, "failed to find %q subcategory", subcategory)
		}
	}

	return nil
}

// VerifyShortcuts checks that the shortcut descriptions and the shortcut keys exists.
func VerifyShortcuts(ctx context.Context, ui *uiauto.Context, description string, shortcutKeys ShortcutKeys) error {
	descriptionNode := nodewith.Name(description).Role(role.RowHeader).First()
	shortcutKeysNode := nodewith.Name(shortcutKeys.Keys).Role(shortcutKeys.Role).First()

	if err := uiauto.Combine("Verify shortcut description and shortcut keys exist",
		ui.WaitUntilExists(descriptionNode),
		ui.WaitUntilExists(shortcutKeysNode),
	)(ctx); err != nil {
		return errors.Wrapf(err, "failed to verify shortcut description %s and shortcut keys %s exist", description, shortcutKeys)
	}

	return nil
}

// VerifyShortcutsRegex checks that the shortcut descriptions and the shortcut keys input as regular expression, exists.
func VerifyShortcutsRegex(ctx context.Context, ui *uiauto.Context, description string, shortcutKeys ShortcutKeys) error {
	descriptionNode := nodewith.NameRegex(regexp.MustCompile(description)).Role(role.RowHeader).First()
	shortcutKeysNode := nodewith.NameRegex(regexp.MustCompile(shortcutKeys.Keys)).Role(shortcutKeys.Role).First()

	if err := uiauto.Combine("Verify shortcut description and shortcut keys exist",
		ui.WaitUntilExists(descriptionNode),
		ui.WaitUntilExists(shortcutKeysNode),
	)(ctx); err != nil {
		return errors.Wrapf(err, "failed to verify shortcut description %s and shortcut keys %s exist", description, shortcutKeys)
	}

	return nil
}

// SearchAndCheck searches for the given query in the Shortcuts `SearchBox`.
// Then, it verifies if any of the results match the given regex, and also
// whether the existence of search results or not matches expectations.
func SearchAndCheck(ctx context.Context, ui *uiauto.Context, kb *input.KeyboardEventWriter, queryAndExpectation ShortcutsSearchQueryAndExpectation) (*uiauto.NodeInfo, error) {
	testing.ContextLogf(ctx, "Search for %q", queryAndExpectation.Query)
	infos, noResults, err := SearchWithQuery(ctx, ui, kb, queryAndExpectation.Query)
	if err != nil {
		return nil, err
	}

	// Expected no results, and got no results, so we can return early with no errors.
	if queryAndExpectation.ExpectNoResults && noResults {
		return nil, nil
	}

	// Expected no results, but got some results.
	if queryAndExpectation.ExpectNoResults && !noResults {
		return nil, errors.New("expected no results to exist, but some results found")
	}

	// Expected results, but got no results.
	if !queryAndExpectation.ExpectNoResults && noResults {
		return nil, errors.New("expected results to exist, but no results found")
	}

	if len(infos) == 0 {
		// The keyword did not return a search result (ie. there is a mismatch).
		// We can return as we do not need to verify the resulting string any further.
		// The check for whether the correct node with the name `No search results found` is
		// returned is done in function searchWithKeyword() below,
		// so there is no need for further verification.
		return nil, nil
	} else if len(infos) > 5 || len(infos) < 1 {
		// The results should show a minimum of 1 or maximum of 5 results.
		return nil, errors.Errorf("unexpected result count, want: [1,5], got: %d", len(infos))
	}

	// Verify result.
	rExpected := regexp.MustCompile(queryAndExpectation.ExpectedDescriptionRegex)
	for idx, info := range infos {
		if rExpected.MatchString(info.Name) {
			testing.ContextLogf(ctx, "Found: %q", infos[idx].Name)
			return &infos[idx], nil
		}
	}

	return nil, errors.Errorf("no match results found for expected regex %q, the first result is %q", queryAndExpectation.ExpectedDescriptionRegex, infos[0].Name)
}

// SearchWithQuery searches for the given query in the `SearchBox`.
func SearchWithQuery(ctx context.Context, ui *uiauto.Context, kb *input.KeyboardEventWriter, query string) (results []uiauto.NodeInfo, noResults bool, err error) {

	if err := uiauto.Combine(fmt.Sprintf("query with query %q", query),
		kb.TypeAction(query),
		ui.WaitUntilExists(nodewith.HasClass("ContentsWebView").Focused()),
	)(ctx); err != nil {
		return nil, false, errors.Wrap(err, "failed to type into search box")
	}

	if err := ui.WaitForLocation(SearchResultFinder.First())(ctx); err == nil {
		// No errors, so retrieve the search result's info and return the results.
		results, err = ui.NodesInfo(ctx, SearchResultFinder)
		return results, false, err
	}

	SearchNoResultsFinder := nodewith.Name("No search results found").Role(role.StaticText).First()
	if err := ui.WaitUntilExists(SearchNoResultsFinder)(ctx); err == nil {
		// No errors, but no search results found.
		return nil, true, nil
	}

	return nil, false, errors.New("Shortcuts app search error")
}

// ClearSearch clears text in `SearchBox` and waits for the search results to be gone.
func ClearSearch(ctx context.Context, ui *uiauto.Context) uiauto.Action {
	clearSearchBtn := nodewith.NameContaining("Clear search").Role(role.Button)
	return uiauto.Combine("clear text in search box",
		uiauto.IfSuccessThen(ui.WaitUntilExists(clearSearchBtn), ui.LeftClick(clearSearchBtn)),
		ui.WaitUntilGone(clearSearchBtn),
		ui.WaitUntilGone(SearchResultFinder),
	)
}

// AddCustomShortcut adds any custom shortcut to given action/widget with or without an alert and returns the result
func AddCustomShortcut(ctx context.Context, ui *uiauto.Context, kb *input.KeyboardEventWriter, action, newShortcut, warnMessage string) error {

	// Open the edit dialog and input the custom shortcut for the action with a warning alert
	if warnMessage != "" {
		testing.ContextLogf(ctx, "Warning message to be checked %q", warnMessage)
		if err := uiauto.Combine("Add custom shortcut",
			ui.WaitUntilExists(nodewith.Name(action).Role(role.StaticText)),
			ui.LeftClick(nodewith.Name("Add shortcut").Role(role.Button)),
			kb.AccelAction(newShortcut),
			ui.WaitUntilExists(nodewith.Name(warnMessage).Role(role.StaticText).First()),
			kb.AccelAction(newShortcut),
			ui.LeftClick(nodewith.Name("Done").Role(role.Button)),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to add custom shortcut")
		}
	} else {
		// Open the edit dialog and input the custom shortcut for the action with no warning alert
		if err := uiauto.Combine("Add custom shortcut",
			ui.WaitUntilExists(nodewith.Name(action).Role(role.StaticText)),
			ui.LeftClick(nodewith.Name("Add shortcut").Role(role.Button)),
			kb.AccelAction(newShortcut),
			ui.LeftClick(nodewith.Name("Done").Role(role.Button)),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to add custom shortcut")
		}
	}

	return nil
}
