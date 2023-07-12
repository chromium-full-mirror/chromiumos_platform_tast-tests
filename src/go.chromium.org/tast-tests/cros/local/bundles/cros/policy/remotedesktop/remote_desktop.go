// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package remotedesktop contains helpers to verify the successful registration
// of a client for remote support.
package remotedesktop

import (
	"context"
	"net/http/httptest"
	"strings"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/crd"
	"go.chromium.org/tast/core/errors"
)

const (
	// FTLMessagingClientReceiveMessagesHashCode is the hashcode of the network annotation tag
	// with id: ftl_messaging_client_receive_messages.
	FTLMessagingClientReceiveMessagesHashCode = "136248372"
	// FTLRegistrationManagerHashCode is the hashcode of the network annotation tag
	// with id: ftl_registration_manager.
	FTLRegistrationManagerHashCode = "38256901"
	// RemotingRegisterSupportHostRequestHashCode is the hashcode of the network annotaiton tag
	// with id: remoting_register_support_host_request.
	RemotingRegisterSupportHostRequestHashCode = "67117364"
)

// testCase defines test expectations based on the policy value.
type testCase struct {
	Name                   string
	ShouldCRDLaunchSucceed bool
	ShouldFindAnnotations  bool
	Policy                 *policy.RemoteAccessHostAllowRemoteSupportConnections
}

// TestCases returns the list of testCase objects on which
// RemoteAccessHostAllowRemoteSupportConnections policy is tested.
func TestCases() []testCase {
	return []testCase{
		{
			Name:                   "disabled",
			ShouldCRDLaunchSucceed: false,
			ShouldFindAnnotations:  false,
			Policy:                 &policy.RemoteAccessHostAllowRemoteSupportConnections{Val: false},
		},
		{
			Name:                   "unset",
			ShouldCRDLaunchSucceed: true,
			ShouldFindAnnotations:  true,
			Policy:                 &policy.RemoteAccessHostAllowRemoteSupportConnections{Stat: policy.StatusUnset},
		},
		{
			Name:                   "enabled",
			ShouldCRDLaunchSucceed: true,
			ShouldFindAnnotations:  true,
			Policy:                 &policy.RemoteAccessHostAllowRemoteSupportConnections{Val: true},
		},
	}
}

// TriggerRemoteSupportRegistration verifies that launching chrome remote
// desktop works as expected.
func TriggerRemoteSupportRegistration(ctx context.Context, _ *chrome.Chrome, br *browser.Browser, _ *httptest.Server, tconn *chrome.TestConn, paramIndex int) (err error) {
	param := TestCases()[paramIndex]

	didCRDLaunchSucceed := true
	errContainsRemoteSupportBlockedMessage := false
	if err := crd.Launch(ctx, br, tconn); err != nil {
		didCRDLaunchSucceed = false
		errContainsRemoteSupportBlockedMessage = strings.Contains(err.Error(), "Remote support connections blocked")
	}

	if param.ShouldCRDLaunchSucceed != didCRDLaunchSucceed {
		return errors.Errorf("unexpected success status of remote desktop launch: got %t want %t", didCRDLaunchSucceed, param.ShouldCRDLaunchSucceed)
	}

	if !param.ShouldCRDLaunchSucceed && errContainsRemoteSupportBlockedMessage == false {
		return errors.Wrap(err, "remote desktop failure message did not include connections blocked")
	}

	return nil
}
