// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package quickanswers contains helper functions for the local Tast tests
// that exercise ChromeOS Quick answers feature.
package quickanswers

import (
	"context"
	"strings"
	"unicode/utf8"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
)

const (
	prefPathEnabled            = "settings.quick_answers.enabled"
	prefPathConsentStatus      = "settings.quick_answers.consent_status"
	prefPathPreferredLanguages = "settings.language.preferred_languages"

	prefValueConsentStatusUnknown = 0
)

// ResetPref resets quick answers pref values to default values.
func ResetPref(ctx context.Context, tconn *chrome.TestConn) error {
	// prefPathConsentStatus is not in chrome.settingsPrivate.setPref.
	if err := tconn.Call(ctx, nil,
		`tast.promisify(chrome.autotestPrivate.setAllowedPref)`,
		prefPathConsentStatus,
		prefValueConsentStatusUnknown); err != nil {
		return errors.Wrap(err, "failed to reset consent status")
	}
	return SetPrefValue(ctx, tconn, prefPathEnabled, false)
}

// SetPreferredLanguages sets preferred languages to languageCodes.
func SetPreferredLanguages(ctx context.Context, tconn *chrome.TestConn,
	languageCodes []string) error {
	return SetPrefValue(ctx, tconn, prefPathPreferredLanguages, strings.Join(languageCodes, ","))
}

// Enable enables quick answers pref.
func Enable(ctx context.Context, tconn *chrome.TestConn) error {
	return SetPrefValue(ctx, tconn, prefPathEnabled, true)
}

// SetPrefValue is a helper function to sets value for Quick answers related prefs.
// Note that the pref needs to be allowlisted here:
// https://cs.chromium.org/chromium/src/chrome/browser/extensions/api/settings_private/prefs_util.cc
// TODO(b/339097439): Make this a private function and expose them with respective
// pref path specific methods, e.g., Enabled.
func SetPrefValue(ctx context.Context, tconn *chrome.TestConn, prefName string, value interface{}) error {
	return tconn.Call(ctx, nil, `tast.promisify(chrome.settingsPrivate.setPref)`, prefName, value)
}

// BuildDataURL builds a plain text data url with the provided query word.
func BuildDataURL(queryWord string) string {
	return "data:text/plain;charset=UTF-8," + queryWord
}

// SelectQueryWord selects a query word and returns nodewith.Finder for the text.
func SelectQueryWord(ctx context.Context, tconn *chrome.TestConn,
	queryWord string) (*nodewith.Finder, error) {
	ui := uiauto.New(tconn)
	// Wait for the query word to appear.
	query := nodewith.Name(queryWord).Role(role.StaticText).Ancestor(
		nodewith.Role(role.WebView)).First()
	if err := ui.WaitUntilExists(query)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait for query to load")
	}

	// Select the word and setup watcher to wait for text selection event.
	if err := ui.WaitForEvent(nodewith.Root(),
		event.DocumentSelectionChanged,
		ui.Select(
			query, 0 /*startOffset*/, query,
			utf8.RuneCountInString(queryWord) /*endOffset*/))(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to select query")
	}

	return query, nil
}
