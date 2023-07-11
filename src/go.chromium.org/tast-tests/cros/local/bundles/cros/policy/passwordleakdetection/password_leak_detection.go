// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package passwordleakdetection contains helpers to verify password leak
// detection.
package passwordleakdetection

import (
	"context"
	"net/http/httptest"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast/core/errors"
)

// TestCase defines test expectations based on the policy value.
type TestCase struct {
	Name                 string
	ShouldFindAnnotation bool
	Policy               *policy.PasswordLeakDetectionEnabled
}

const (
	// AnnotationHashCode is the hashcode of network annotation tag
	// lookup_single_password_leak.
	AnnotationHashCode = "16927377"

	// Test webpage that stores password credentials using the Credential
	// Management API.
	testFileName = "password_leak_detection.html"
)

// GetTestCases returns the list of TestCase objects on which
// PasswordLeakDetectionEnabled policy is tested.
func GetTestCases() []TestCase {
	// Reordering the TestCase objects in the returned list may break tests.
	return []TestCase{
		{
			Name:                 "disabled",
			ShouldFindAnnotation: false,
			Policy:               &policy.PasswordLeakDetectionEnabled{Val: false},
		},
		{
			Name:                 "enabled",
			ShouldFindAnnotation: true,
			Policy:               &policy.PasswordLeakDetectionEnabled{Val: true},
		},
		{
			Name:                 "unset",
			ShouldFindAnnotation: true,
			Policy:               &policy.PasswordLeakDetectionEnabled{Stat: policy.StatusUnset},
		},
	}
}

// GetDataFiles returns the list of data files needed to be copied to the dut
// for running tests related to password leak detection.
func GetDataFiles() []string {
	return []string{"password_leak_detection.html"}
}

// TriggerPasswordLeakDetection triggers password leak detection process when
// enabled.
// Note that there are a couple potential ways to trigger this:
//  1. Successfully submit a password form
//  2. Call the Credential Management API's `store()` method
//
// We currently use option #2 for this test as it only requires running some JS
// on a test webpage we control.
func TriggerPasswordLeakDetection(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, server *httptest.Server, _ *chrome.TestConn, _ int) (err error) {
	// Open a webpage that will trigger password leak detection, when enabled.
	conn, err := br.NewConn(ctx, server.URL+"/"+testFileName)
	if err != nil {
		return errors.Wrap(err, "failed to create new Chrome connection")
	}
	defer conn.Close()

	return nil
}
