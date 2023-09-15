// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package safesearch

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast/core/errors"
)

// TestGoogleSafeSearch checks whether safe search is automatically enabled for
// Google search.
func TestGoogleSafeSearch(ctx context.Context, br *browser.Browser, safeSearchExpected bool) error {
	conn, err := br.NewConn(ctx, "https://www.google.com/search?q=kittens")
	if err != nil {
		return errors.Wrap(err, "failed to connect to Chrome")
	}
	defer conn.Close()

	originalCondition := `new URL(document.URL).searchParams.get("safe") == "active"`
	// If page was redirected for security check the original url will be encoded in the new url query params:
	// https://www.google.com/sorry/index?continue=https://www.google.com/search%3Fq%3Dkittens%26safe%3Dactive
	redirectCondition := `document.URL.toLowerCase().indexOf("safe%3dactive") > -1`
	var isSafe bool
	if err := conn.Eval(ctx, fmt.Sprintf("%s || %s", originalCondition, redirectCondition), &isSafe); err != nil {
		return errors.Wrap(err, "could not read safe search param from URL")
	}

	if isSafe != safeSearchExpected {
		return errors.Errorf("unexpected safe search behavior; got %t, want %t", isSafe, safeSearchExpected)
	}

	return nil
}
