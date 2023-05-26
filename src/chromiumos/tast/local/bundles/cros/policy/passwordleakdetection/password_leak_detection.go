// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package passwordleakdetection contains helpers to verify password leak
// detection.
package passwordleakdetection

import (
	"context"
	"net/http/httptest"
	"time"

	"chromiumos/tast/local/annotations"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/common/policy"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
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

// TriggerPasswordLeakDetection triggers password leak detection process when
// enabled.
// Note that there are a couple potential ways to trigger this:
//  1. Successfully submit a password form
//  2. Call the Credential Management API's `store()` method
//
// We currently use option #2 for this test as it only requires running some JS
// on a test webpage we control.
func TriggerPasswordLeakDetection(ctx context.Context, cr *chrome.Chrome, server *httptest.Server, br *browser.Browser) (err error) {
	// Open a webpage that will trigger password leak detection, when enabled.
	conn, err := br.NewConn(ctx, server.URL+"/"+testFileName)
	if err != nil {
		return errors.Wrap(err, "failed to create new Chrome connection")
	}
	defer conn.Close()

	// Check the logs for annotation lookup_single_password_leak. Use polling
	// since the password check happens in the background, and there may not
	// always be a UI popup to notify that it is done.
	var errorCheckingLogs error
	err = testing.Poll(ctx, func(ctx context.Context) error {
		foundAnnotation := false
		foundAnnotation, errorCheckingLogs = annotations.CheckLogs(ctx, cr, AnnotationHashCode)

		if errorCheckingLogs != nil || foundAnnotation {
			return nil
		}

		// Annotation not found yet, keep polling.
		return errors.New("Annotation ID not found yet")
	}, &testing.PollOptions{Timeout: 5 * time.Second})

	if errorCheckingLogs != nil {
		return errors.Wrap(errorCheckingLogs, "failed to check network logs")
	}

	return nil
}
