// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package advancedprotection contains helpers to verify Advanced Protection.
package advancedprotection

import (
	"context"
	"fmt"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// UploadAnnotationHashCode is the hashcode of network annotation
// safe_browsing_binary_upload_app.
const UploadAnnotationHashCode = "4306022"

// suspiciousFile is a test suspicious file which should trigger safe browsing
// warnings on download. This file was copied from:
// https://testsafebrowsing.appspot.com/s/bad_app_file_on_scan.exe
const suspiciousFile = "bad_app_file_on_scan.exe"

// testCase defines test expectations based on the policy value.
type testCase struct {
	Name                 string
	ShouldFindAnnotation bool
	Policy               *policy.AdvancedProtectionAllowed
}

// GetTestCases returns the list of TestCase objects for each policy value.
func GetTestCases() []testCase {
	// Reordering the TestCase objects in the returned list may break tests.
	return []testCase{
		{
			Name:                 "disabled",
			ShouldFindAnnotation: false,
			Policy:               &policy.AdvancedProtectionAllowed{Val: false},
		},
		{
			Name:                 "enabled",
			ShouldFindAnnotation: true,
			Policy:               &policy.AdvancedProtectionAllowed{Val: true},
		},
		{
			Name:                 "unset",
			ShouldFindAnnotation: true,
			Policy:               &policy.AdvancedProtectionAllowed{Stat: policy.StatusUnset},
		},
	}
}

// TriggerUploadForScanning downloads a suspicious file and then uploads it for
// scanning.
func TriggerUploadForScanning(ctx context.Context, _ *testing.State, _ *chrome.Chrome, br *browser.Browser, server *httptest.Server, tconn *chrome.TestConn, paramIndex int) (err error) {
	// Open the browser and download the test suspicious file from the local file
	// server.
	conn, err := br.NewConn(ctx, "")
	if err != nil {
		return errors.Wrap(err, "failed to open the browser")
	}
	defer conn.Close()
	if err := conn.Eval(ctx,
		fmt.Sprintf("window.location.href = \"%s/%s\";", server.URL, suspiciousFile),
		nil,
	); err != nil {
		return errors.Wrap(err, "failed to download file")
	}

	// After the file is downloaded, a notification box will appear to notify
	// the user that this file is potentially dangerous. If Advanced
	// Protection is enabled, then there will be an option to upload the file for
	// additional scanning.
	ui := uiauto.New(tconn)
	sendFile := nodewith.Name("Send").Role(role.Button)
	discardFile := nodewith.Name("Discard").Role(role.Button)
	if err := uiauto.Combine("Upload file for scanning",
		// If prompted to upload file for scanning, click 'Send'. This will
		// only show if Advanced Protection is allowed.
		uiauto.IfSuccessThen(
			ui.WithTimeout(3*time.Second).WaitUntilExists(sendFile),
			ui.DoDefault(sendFile)),
		// In both enabled and disabled cases, the file will be marked as dangerous.
		// Click 'Discard' to cleanup.
		ui.WaitUntilExists(discardFile),
		ui.DoDefault(discardFile),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to upload file for scanning")
	}

	return nil
}
