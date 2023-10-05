// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package quickanswers contains helpers to verify quick answers definition and
// unit conversion.
package quickanswers

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/event"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
)

const (
	// AnnotationHashCode is the hashcode of network annotation tag
	// quick_answers_loader.
	AnnotationHashCode = "46208118"

	originalUnitsText        = "50 kg"
	expectedConversionResult = "110.231 pounds"

	textToDefine           = "icosahedron"
	expectedDefinitionText = "twenty plane faces"

	testFileName = "quick_answers.html"
)

// DefinitionTestCase defines test expectations based on the value of policy QuickAnswersDefinitionEnabled.
type DefinitionTestCase struct {
	Name                  string
	ShouldFindAnnotation  bool
	ShouldShowContextMenu bool
	Policy                *policy.QuickAnswersDefinitionEnabled
}

// DefinitionTestCases returns the map of policy setting enum to DefinitionTestCase
// objects on which the QuickAnswersDefinitionEnabled policy is tested.
func DefinitionTestCases() map[networkrequestmonitor.PolicySetting]DefinitionTestCase {
	return map[networkrequestmonitor.PolicySetting]DefinitionTestCase{
		networkrequestmonitor.PolicyDisabled: {
			Name:                  "disabled",
			ShouldFindAnnotation:  false,
			ShouldShowContextMenu: false,
			Policy:                &policy.QuickAnswersDefinitionEnabled{Val: false},
		},
		networkrequestmonitor.PolicyEnabled: {
			Name:                  "enabled",
			ShouldFindAnnotation:  true,
			ShouldShowContextMenu: true,
			Policy:                &policy.QuickAnswersDefinitionEnabled{Val: true},
		},
		networkrequestmonitor.PolicyUnset: {
			Name:                  "unset",
			ShouldFindAnnotation:  true,
			ShouldShowContextMenu: true,
			Policy:                &policy.QuickAnswersDefinitionEnabled{Stat: policy.StatusUnset},
		},
	}
}

// UnitConversionTestCase defines test expectations based on the value of policy QuickAnswersUnitConversionEnabled.
type UnitConversionTestCase struct {
	Name                  string
	ShouldFindAnnotation  bool
	ShouldShowContextMenu bool
	Policy                *policy.QuickAnswersUnitConversionEnabled
}

// UnitConversionTestCases returns the map of policy setting enum to UnitConversionTestCase
// objects on which QuickAnswersUnitConversionEnabled policy is tested.
func UnitConversionTestCases() map[networkrequestmonitor.PolicySetting]UnitConversionTestCase {
	return map[networkrequestmonitor.PolicySetting]UnitConversionTestCase{
		networkrequestmonitor.PolicyDisabled: {
			Name:                  "disabled",
			ShouldFindAnnotation:  false,
			ShouldShowContextMenu: false,
			Policy:                &policy.QuickAnswersUnitConversionEnabled{Val: false},
		},
		networkrequestmonitor.PolicyEnabled: {
			Name:                  "enabled",
			ShouldFindAnnotation:  true,
			ShouldShowContextMenu: true,
			Policy:                &policy.QuickAnswersUnitConversionEnabled{Val: true},
		},
		networkrequestmonitor.PolicyUnset: {
			Name:                  "unset",
			ShouldFindAnnotation:  true,
			ShouldShowContextMenu: true,
			Policy:                &policy.QuickAnswersUnitConversionEnabled{Stat: policy.StatusUnset},
		},
	}
}

// DataFiles returns the list of data files needed to be copied to the dut
// for running tests related to quick answers.
func DataFiles() []string {
	return []string{"quick_answers.html"}
}

// TriggerQuickAnswersDefinition attempts to trigger quick answers definition and checks if the policy works as defined in the DefinitionTestCase param.
func TriggerQuickAnswersDefinition(ctx context.Context, params networkrequestmonitor.OptionalServiceParams) (err error) {
	server := params.Server
	br := params.Browser
	cr := params.Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		errors.Wrap(err, "failed to create Test API connection")
	}
	policyParam := DefinitionTestCases()[params.PolicySetting]

	// Open page with the query word on it.
	conn, err := br.NewConn(ctx, server.URL+"/"+testFileName, browser.WithNewWindow())
	if err != nil {
		return errors.Wrap(err, "failed to create new chrome connection")
	}
	defer conn.Close()
	defer conn.CloseTarget(ctx)

	ui := uiauto.New(tconn)

	// Wait for the query word to appear.
	query := nodewith.Name(textToDefine).Role(role.StaticText).First()
	if err := ui.WaitUntilExists(query)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for query to load")
	}

	// Select the word and setup watcher to wait for text selection event.
	if err := ui.WaitForEvent(nodewith.Root(),
		event.TextSelectionChanged,
		ui.Select(query, 0 /*startOffset*/, query, 2 /*endOffset*/))(ctx); err != nil {
		return errors.Wrap(err, "failed to select query")
	}

	quickAnswers := nodewith.ClassName("QuickAnswersView")
	definitionResult := nodewith.NameContaining(expectedDefinitionText).ClassName("QuickAnswersTextLabel")

	// Right click the selected word and ensure the Quick Answers UI shows the definition.
	if err := uiauto.Combine("Show context menu",
		ui.RightClick(query),
		ui.WaitUntilExists(quickAnswers),
		ui.WaitUntilExists(definitionResult),
	)(ctx); err != nil {
		if policyParam.ShouldShowContextMenu {
			return errors.Wrap(err, "quick answers result not showing up")
		}
		if !nodewith.IsNodeNotFoundErr(err) {
			return errors.Wrap(err, "failure while trying to show the context menu")
		}
		// else the context menu was not found, as expected.
	} else if !policyParam.ShouldShowContextMenu {
		return errors.New("quick answers result shows when it should be disabled")
	}

	// Dismiss the context menu and ensure the Quick Answers UI also dismisses.
	if err := uiauto.Combine("Dismiss context menu",
		ui.LeftClick(query),
		ui.WaitUntilGone(quickAnswers),
	)(ctx); err != nil {
		return errors.Wrap(err, "quick answers result not dismissed")
	}
	return nil
}

// TriggerQuickAnswersUnitConversion attempts to trigger quick answers unit conversion and checks if the policy works as defined in the UnitConversionTestCase param.
func TriggerQuickAnswersUnitConversion(ctx context.Context, params networkrequestmonitor.OptionalServiceParams) (err error) {
	server := params.Server
	br := params.Browser
	cr := params.Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		errors.Wrap(err, "failed to create Test API connection")
	}
	param := DefinitionTestCases()[params.PolicySetting]

	// Open page with source units on it.
	conn, err := br.NewConn(ctx, server.URL+"/"+testFileName, browser.WithNewWindow())
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
		// else the context menu was not found, as expected.
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
