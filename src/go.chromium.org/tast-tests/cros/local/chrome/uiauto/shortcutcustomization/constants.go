// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package shortcutcustomization

import (
	"fmt"
	"regexp"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
)

// SearchResultFinder is a finder of all possible search results if they exist.
var SearchResultFinder = nodewith.NameRegex(regexp.MustCompile(fmt.Sprintf(`(Search result \d+ of \d+: .*)`))).Onscreen()

// WarnMessageNoSearch is the warning message user will get when non search shortcut is input.
var WarnMessageNoSearch = "Shortcut without search key might conflict with some app's shortcut. Press this shortcut again to continue using it, or press a new shortcut using the search key."

// ShortcutsSearchQueryAndExpectation is a struct that encapsulates a search query and
// the expected description of at least one of the results. If no results are
// expected, `expectedDescriptionRegex` should be empty and `expectNoResults`
// should be true.
type ShortcutsSearchQueryAndExpectation struct {
	Query                    string
	ExpectedDescriptionRegex string
	ExpectNoResults          bool
}
