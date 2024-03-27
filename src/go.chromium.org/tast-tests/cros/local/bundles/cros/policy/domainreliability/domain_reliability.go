// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package domainreliability contains helpers to verify the domain reliability.
package domainreliability

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
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

// TestCases returns the map of policy settings enum to TestCase object for each
// value on which DomainReliabilityAllowed policy is tested.
func TestCases() map[networkrequestmonitor.PolicySetting]TestCase {
	return map[networkrequestmonitor.PolicySetting]TestCase{
		networkrequestmonitor.PolicyDisabled: {
			Name:                 "disabled",
			ShouldFindAnnotation: false,
			Policy:               &policy.DomainReliabilityAllowed{Val: false},
		},
		networkrequestmonitor.PolicyEnabled: {
			Name:                 "enabled",
			ShouldFindAnnotation: true,
			Policy:               &policy.DomainReliabilityAllowed{Val: true},
		},
	}
}

// TriggerDomainReliabilityAllowed triggers domain reliability diagnostic data
// reporting when allowed by policy.
// It is triggered by block images.google.com and return 500 error.
func TriggerDomainReliabilityAllowed(ctx context.Context, params networkrequestmonitor.OptionalServiceParams) (err error) {
	// Reserve 15 seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	cr := params.Chrome
	proxyOpts := params.ProxyOpts

	mp, err := proxy.NewMitmProxy(ctx, proxyOpts...)
	if err != nil {
		return errors.Wrap(err, "failed to launch and apply proxy")
	}
	reset, err := proxy.ConfigureChrome(ctx, mp, cr)
	if err != nil {
		return errors.Wrap(err, "failed to configure a proxy")
	}
	defer reset(cleanupCtx, cr)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		errors.Wrap(err, "failed to create Test API connection")
	}

	// Open new tab and navigate to domainReliabilityTestURL.
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

	// Wait for traffic error to ensure domain reliability report is created.
	ui := uiauto.New(tconn)
	if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(nodewith.NameContaining("Error injected by mitmproxy.").Role(role.StaticText))(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for traffic error")
	}

	return nil
}
