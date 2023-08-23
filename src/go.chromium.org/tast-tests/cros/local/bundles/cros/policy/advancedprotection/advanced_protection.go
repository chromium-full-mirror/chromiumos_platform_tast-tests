// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package advancedprotection contains helpers to verify Advanced Protection.
package advancedprotection

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
)

// UploadAnnotationHashCode is the hashcode of network annotation
// safe_browsing_binary_upload_app.
const UploadAnnotationHashCode = "4306022"

// suspiciousFile is a test suspicious file which should trigger safe browsing
// warnings on download. This file was copied from:
// https://testsafebrowsing.appspot.com/s/bad_app_file_on_scan.exe
const suspiciousFile = "bad_app_file_on_scan.exe"

// DataFiles returns the list of data files needed to be copied to the dut
// for running tests related to Advanced Protection.
func DataFiles() []string {
	return []string{suspiciousFile}
}

// testCase defines test expectations based on the policy value.
type testCase struct {
	Name                 string
	ShouldFindAnnotation bool
	Policy               *policy.AdvancedProtectionAllowed
}

// TestCases returns the map of policy setting enum to testCase object
// for each AdvancedProtectionAllowed policy value.
func TestCases() map[networkrequestmonitor.PolicySetting]testCase {
	// Reordering the TestCase objects in the returned list may break tests.
	return map[networkrequestmonitor.PolicySetting]testCase{
		networkrequestmonitor.PolicyDisabled: {
			Name:                 "disabled",
			ShouldFindAnnotation: false,
			Policy:               &policy.AdvancedProtectionAllowed{Val: false},
		},
		networkrequestmonitor.PolicyEnabled: {
			Name:                 "enabled",
			ShouldFindAnnotation: true,
			Policy:               &policy.AdvancedProtectionAllowed{Val: true},
		},
		networkrequestmonitor.PolicyUnset: {
			Name:                 "unset",
			ShouldFindAnnotation: true,
			Policy:               &policy.AdvancedProtectionAllowed{Stat: policy.StatusUnset},
		},
	}
}

// TriggerUploadForScanning downloads a suspicious file and then uploads it for
// scanning.
func TriggerUploadForScanning(ctx context.Context, params networkrequestmonitor.OptionalServiceParams) (err error) {
	cr := params.Chrome
	server := params.Server
	br := params.Browser
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		errors.Wrap(err, "failed to create Test API connection")
	}

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

	// Open the Chrome downloads page. Note: There is usually also a popup dialog
	// with the same info, but this dialog can be different on ash vs lacros. The
	// downloads page is more consistent, so we prefer it here.
	downloadsConn, err := br.NewConn(ctx, "chrome://downloads")
	if err != nil {
		return errors.Wrap(err, "failed to open chrome://downloads")
	}
	defer downloadsConn.Close()

	ui := uiauto.New(tconn)
	sendFile := nodewith.Name("Scan for malware").Role(role.Button)
	if err := uiauto.Combine("Upload file for scanning",
		// If prompted to upload file for scanning, click 'Scan'. This will
		// only show if Advanced Protection is allowed.
		uiauto.IfSuccessThen(
			ui.WithTimeout(3*time.Second).WaitUntilExists(sendFile),
			ui.DoDefault(sendFile)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to upload file for scanning")
	}

	return nil
}
