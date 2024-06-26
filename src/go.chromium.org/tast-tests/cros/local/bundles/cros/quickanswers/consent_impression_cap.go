// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quickanswers

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/quickanswers"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ConsentImpressionCap,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test Quick Answers consent impression cap logic",
		Contacts: []string{
			"cros-assistive@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		BugComponent: "b:905229", // ChromeOS > Software > Assistive
		Attr: []string{
			"group:hw_agnostic",
			"group:mainline",
			"informational",
		},
		SoftwareDeps: []string{"chrome", "gaia"},
		Params: []testing.Param{
			{
				Fixture: quickanswers.Parameterize(
					quickanswers.NotEnabledWithBrowserFixture,
					quickanswers.VariantSingleWord,
				),
			}, {
				Name: "lacros",
				Fixture: quickanswers.Parameterize(
					quickanswers.NotEnabledWithBrowserLacrosFixture,
					quickanswers.VariantSingleWord,
				),
				ExtraSoftwareDeps: []string{"lacros"},
			},
		},
	})
}

const (
	consentImpressionCap         = 6
	consentImpressionMinDuration = time.Second

	consentUIShownTimeout = time.Second
)

func ConsentImpressionCap(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	queryWord := s.FixtValue().(quickanswers.HasQueryWord).QueryWord()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	query, err := quickanswers.SelectQueryWord(ctx, tconn, queryWord)
	if err != nil {
		s.Fatal("Failed to select a query word: ", err)
	}

	ui := uiauto.New(tconn)
	userConsent := nodewith.ClassName("UserConsentView")

	for i := 0; i < consentImpressionCap; i++ {
		// Right click the selected word and ensure the consent UI shows up.
		if err := uiauto.Combine("Show consent UI",
			ui.RightClick(query),
			ui.WithTimeout(consentUIShownTimeout).WaitUntilExists(userConsent),
		)(ctx); err != nil {
			s.Fatal("Quick Answers consent UI not showing up: ", err)
		}

		// GoBigSleepLint: a consent session must be longer than 1 second as it's
		// counted as an impression.
		if err := testing.Sleep(ctx, consentImpressionMinDuration); err != nil {
			s.Fatal("Failed to wait in a consent session: ", err)
		}

		// Dismiss the context menu and ensure the consent UI also dismiss.
		if err := uiauto.Combine("Dismiss context menu",
			ui.LeftClick(query),
			ui.WaitUntilGone(userConsent))(ctx); err != nil {
			s.Fatal("Quick Answers result not dismissed: ", err)
		}

		// Note: consent impression count increment and consent status pref writes
		// are done as async operations. If you observe a test failures suspecting
		// stale value reads, consider waiting pref write operations explicitly,
		// e.g., implementing pref change watcher, etc.
	}

	// Right click the selected word and ensure the consent UI is not shown in the timeout.
	// We have already tested that a consent UI is shown in the timeout for
	// consentImpressionCap times. It should be reasonable to assume that it can be shown
	// in the timeout if a logic is broken.
	if err := uiauto.Combine("Show consent UI",
		ui.RightClick(query),
		ui.WithTimeout(consentUIShownTimeout).WaitUntilExists(userConsent),
	)(ctx); err == nil {
		s.Fatal("Quick Answers consent UI is shown: ", err)
	}
}
