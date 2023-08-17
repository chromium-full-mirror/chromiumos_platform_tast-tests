// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package urlkeydatacollection contains utility to trigger UKM log
// and verify log on UKM app.
package urlkeydatacollection

import (
	"context"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	policyannotations "go.chromium.org/tast-tests/cros/local/bundles/cros/policy/policy_annotations"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const ukmTestURL = "https://www.google.com"

// UkmNetworkAnnotationID is annotation hash code.
const UkmNetworkAnnotationID = "727478"

var ukmFalseMSBBTrueTestCase = policyannotations.AnnotationTestParams{
	Name:                  "ukm_false_msbb_true",
	AnnotationLogExpected: false,
	Policies: []policy.Policy{
		&policy.UrlKeyedAnonymizedDataCollectionEnabled{Val: false},
		&policy.SyncDisabled{Val: false},
		&policy.EnableSyncConsent{Val: true},
	},
}

var ukmTrueMSBBTrueTestCase = policyannotations.AnnotationTestParams{
	Name:                  "ukm_true_msbb_true",
	AnnotationLogExpected: true,
	Policies: []policy.Policy{
		&policy.UrlKeyedAnonymizedDataCollectionEnabled{Val: true},
		&policy.SyncDisabled{Val: false},
		&policy.EnableSyncConsent{Val: true},
	},
}

var ukmTrueMSBBFalseTestCase = policyannotations.AnnotationTestParams{
	Name:                  "ukm_true_msbb_false",
	AnnotationLogExpected: true,
	Policies: []policy.Policy{
		&policy.UrlKeyedAnonymizedDataCollectionEnabled{Val: true},
		&policy.SyncDisabled{Val: true},
		&policy.EnableSyncConsent{Val: false},
	},
}

var ukmFalseMSBBFalseTestCase = policyannotations.AnnotationTestParams{
	Name:                  "ukm_false_msbb_false",
	AnnotationLogExpected: false,
	Policies: []policy.Policy{
		&policy.UrlKeyedAnonymizedDataCollectionEnabled{Val: false},
		&policy.SyncDisabled{Val: true},
		&policy.EnableSyncConsent{Val: false},
	},
}

// TestCases returns the list of TestCase objects on which
// UrlKeyedAnonymizedDataCollectionEnabled policy is tested.
func TestCases() []policyannotations.AnnotationTestParams {
	return []policyannotations.AnnotationTestParams{
		ukmFalseMSBBTrueTestCase,
		ukmTrueMSBBTrueTestCase,
		ukmTrueMSBBFalseTestCase,
		ukmFalseMSBBFalseTestCase,
	}
}

// UmbrellaTestCases returns the map of policy setting enum to AnnotationTestParams
// for the NetworkRequestMonitor test.
func UmbrellaTestCases() map[networkrequestmonitor.PolicySetting]policyannotations.AnnotationTestParams {
	return map[networkrequestmonitor.PolicySetting]policyannotations.AnnotationTestParams{
		networkrequestmonitor.PolicyDisabled: ukmFalseMSBBTrueTestCase,
	}
}

// TriggerAndVerifyUkmAppFromIndex triggers and verifies the appearance
// of logs in chrome://ukm using index of the test case.
func TriggerAndVerifyUkmAppFromIndex(ctx context.Context, br *browser.Browser, paramIndex int) error {
	return triggerAndVerifyUkmApp(ctx, br, TestCases()[paramIndex])
}

// TriggerAndVerifyUkmAppFromPolicySetting triggers and verifies the appearance
// of logs in chrome://ukm using the policy setting enum.
func TriggerAndVerifyUkmAppFromPolicySetting(ctx context.Context, _ *chrome.Chrome, br *browser.Browser, _ *httptest.Server, _ *chrome.TestConn, policySetting networkrequestmonitor.PolicySetting) error {
	return triggerAndVerifyUkmApp(ctx, br, UmbrellaTestCases()[policySetting])
}

func triggerAndVerifyUkmApp(ctx context.Context, br *browser.Browser, param policyannotations.AnnotationTestParams) error {
	ukmAppConn, err := navigateToPageAndLogElement(ctx, br,
		"chrome://ukm", `document.getElementsByClassName("ukm-collection-status")[0]`)
	if err != nil {
		return errors.Wrap(err, "failed to open website")
	}
	defer ukmAppConn.Close()

	// Open the website to log to ukm.
	conn, err := br.NewConn(ctx, ukmTestURL)
	if err != nil {
		return errors.Wrap(err, "failed to open website")
	}
	defer conn.Close()

	// verify logs on ukm app.
	if err := verifyOnUkmApp(ctx, ukmAppConn, param); err != nil {
		return errors.Wrap(err, "test failed")
	}

	return nil
}

// verifyOnUkmApp - verifying that the test url is logged in ukm app.
func verifyOnUkmApp(ctx context.Context, ukmAppConn *chrome.Conn, param policyannotations.AnnotationTestParams) error {
	refreshBtn := `document.getElementById("refresh").click()`
	if err := ukmAppConn.Eval(ctx, refreshBtn, nil); err != nil {
		return errors.Wrap(err, "failed to refresh")
	}

	// Find a row in UKM.
	urlxPath := `//td[contains(@class, 'url') and normalize-space(text()) = '` + ukmTestURL + `/']`
	xpathFinder := `document.evaluate("` + urlxPath +
		`", document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue`
	err := ukmAppConn.WaitForExprWithTimeout(ctx, xpathFinder, 3*time.Second)

	// Test url is logged but it was not expected.
	if !param.AnnotationLogExpected && err == nil {
		return errors.Wrap(err, "found url in the ukm app when not expected")
	}

	// Test url is not logged but it was expected.
	if param.AnnotationLogExpected && err != nil {
		return errors.Wrap(err, "failed to find url in the ukm app when expected")
	}

	// Remaining cases are success criteria:
	// Test url is not logged (err != nil) and it was not expected.
	// Test url is logged (err == nil) and it was expected.
	return nil
}

// navigateToPageAndLogElement navigates to a page and log the contents
// of an element on it.
func navigateToPageAndLogElement(ctx context.Context, br *browser.Browser, url, element string) (newConn *chrome.Conn, err error) {
	conn, err := br.NewConn(ctx, url)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open url app "+url)
	}

	if err := conn.WaitForExpr(ctx, element); err != nil {
		return nil, errors.Wrap(err, "failed to wait for the element "+element)
	}
	var content string
	if err := conn.Eval(ctx, element+".innerText", &content); err != nil {
		return nil, errors.Wrap(err, "failed to get element "+element)
	}
	testing.ContextLog(ctx, content)

	return conn, nil
}
