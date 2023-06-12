// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package domainreliability contains helpers to verify the domain reliability.
package domainreliability

import (
	"context"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/routing"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// AnnotationHashCode is the hashcode of network annotation tag
	// domain_reliability_report_upload.
	AnnotationHashCode = "108804096"

	// DomainReliabilityTestURL is the URL the test will attempt to connect
	// to. The URL must be a Google domain to trigger domain reliability.
	DomainReliabilityTestURL = "images.google.com"
)

// TestCase defines test expectations based on the value of the policy
// DomainReliabilityAllowed.
type TestCase struct {
	Name                 string
	ShouldFindAnnotation bool
	Policy               *policy.DomainReliabilityAllowed
}

// GetTestCases returns the list of TestCase objects for each policy value.
func GetTestCases() []TestCase {
	// Reordering the TestCase objects in the returned list may break tests.
	return []TestCase{
		{
			Name:                 "disabled",
			ShouldFindAnnotation: false,
			Policy:               &policy.DomainReliabilityAllowed{Val: false},
		},
		{
			Name:                 "enabled",
			ShouldFindAnnotation: true,
			Policy:               &policy.DomainReliabilityAllowed{Val: true},
		},
	}
}

// TriggerDomainReliabilityAllowed triggers domain reliability diagnostic data
// reporting when allowed by policy. It is triggered by preventing DNS resolution
// for all hostnames including a test domain reliability URL and attempting to connect
// to that URL.
func TriggerDomainReliabilityAllowed(ctx context.Context, s *testing.State, cr *chrome.Chrome, br *browser.Browser, _ *httptest.Server, tconn *chrome.TestConn, _ int) (err error) {
	// Reserve 10 seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Set up test topology and cleanup.
	testEnv := routing.NewSimpleNetworkEnvWithoutResetProfile(true, true, true, true)
	if err := testEnv.SetUp(ctx); err != nil {
		return errors.Wrap(err, "failed to set up routing test env")
	}
	defer func(ctx context.Context) error {
		if err := testEnv.TearDown(ctx); err != nil {
			return errors.Wrap(err, "failed to tear down routing test env")
		}
		return err
	}(cleanupCtx)

	// Wait for online and verify topology in host.
	if err := testEnv.ShillService.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait for service online")
	}
	var pingAddrs []string
	pingAddrs = append(pingAddrs, routing.TestDomainNameV4)
	pingAddrs = append(pingAddrs, routing.TestDomainNameV6)
	for _, target := range pingAddrs {
		if err := ping.ExpectPingSuccessWithTimeout(ctx, target, "chronos", 10*time.Second); err != nil {
			return errors.Wrapf(err, "network verification failed: %v is not reachable as user %s on host", target, "chronos")
		}
	}

	// Open new tab and navigate to domainReliabilityTestURL.
	// Here we cannot use cr.Conn, because the network test insfrastructure blocks all sites.
	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer keyboard.Close(cleanupCtx)
	if err := keyboard.Accel(ctx, "Ctrl+T"); err != nil {
		return errors.Wrap(err, "failed to press Ctrl+T")
	}
	if err := keyboard.Type(ctx, DomainReliabilityTestURL+"\n"); err != nil {
		return errors.Wrapf(err, "failed to type %s", DomainReliabilityTestURL)
	}

	// Wait for DNS probe to complete to ensure domain reliability report is created.
	ui := uiauto.New(tconn)
	if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(nodewith.Name("DNS_PROBE_FINISHED_NO_INTERNET").Role(role.StaticText))(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for DNS probe to complete")
	}

	return nil
}
