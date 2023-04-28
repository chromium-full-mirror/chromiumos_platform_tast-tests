// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quickanswersutil

import (
	"context"
	"net/http/httptest"

	"chromiumos/tast/common/policy"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/event"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/errors"
)

const (
	originalUnitsText        = "50 kg"
	expectedConversionResult = "110.231 pounds"
)

// UnitConversionTestCase defines test expectations based on the value of policy QuickAnswersUnitConversionEnabled.
type UnitConversionTestCase struct {
	Name                  string
	ShouldFindAnnotation  bool
	ShouldShowContextMenu bool
	Policy                *policy.QuickAnswersUnitConversionEnabled
}

// TriggerQuickAnswersUnitConversion attempts to trigger quick answers unit conversion and checks if the policy works as defined in the UnitConversionTestCase param.
func TriggerQuickAnswersUnitConversion(ctx context.Context, param UnitConversionTestCase, server *httptest.Server, br *browser.Browser, tconn *chrome.TestConn) (err error) {
	// Open page with source units on it.
	conn, err := br.NewConn(ctx, server.URL+"/quick_answers.html", browser.WithNewWindow())
	if err != nil {
		return errors.Wrap(err, "failed to create new chrome connection")
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	ui := uiauto.New(tconn)

	// Wait for the source units to appear.
	units := nodewith.Name(originalUnitsText).Role(role.StaticText).First()
	if err := ui.WaitUntilExists(units)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for units to load")
	}

	// Select the units and setup watcher to wait for text selection event.
	if err := ui.WaitForEvent(nodewith.Root(),
		event.TextSelectionChanged,
		ui.Select(units, 0 /*startOffset*/, units, 5 /*endOffset*/))(ctx); err != nil {
		return errors.Wrap(err, "failed to select units")
	}

	quickAnswers := nodewith.ClassName("QuickAnswersView")
	unitConversionResult := nodewith.NameContaining(expectedConversionResult).ClassName("QuickAnswersTextLabel")

	// Right click the selected units and ensure the Quick Answers UI shows the conversion result in pounds.
	if err := uiauto.Combine("Show context menu",
		ui.RightClick(units),
		ui.WaitUntilExists(quickAnswers),
		ui.WaitUntilExists(unitConversionResult),
	)(ctx); err != nil {
		if param.ShouldShowContextMenu {
			return errors.Wrap(err, "quick answers result not showing up")
		}
		if !nodewith.IsNodeNotFoundErr(err) {
			return errors.Wrap(err, "failure while trying to show the context menu")
		}
		// else the context menu was not found, as expected
	} else if !param.ShouldShowContextMenu {
		return errors.New("quick answers result shows when it should be disabled")
	}

	// Dismiss the context menu and ensure the Quick Answers UI also dismisses.
	if err := uiauto.Combine("Dismiss context menu",
		ui.LeftClick(units),
		ui.WaitUntilGone(quickAnswers),
	)(ctx); err != nil {
		return errors.Wrap(err, "quick answers result not dismissed")
	}
	return nil
}
