// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RegmonRecordPolicyViolation,
		Desc: "Test Regmon RecordPolicyViolation D-Bus method",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"chiav@google.com",
		},
		BugComponent: "b:1129862",
		Attr: []string{
			"group:golden_tier",
			"group:mainline",
			"informational",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "amd64"},
		Data:         []string{"autofill_address_enabled.html"},
		Timeout:      3 * time.Minute,
		Fixture:      fixture.FakeDMS,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PasswordManagerEnabled{}, pci.VerifiedValue),
		},
	})
}

// RegmonRecordPolicyViolation tests Regmon's end-to-end functionality by triggering
// a policy violation in Chrome, and then verifying that regmond in ChromeOS publishes
// the appropriate UMA metric.
func RegmonRecordPolicyViolation(ctx context.Context, s *testing.State) {
	const (
		histogramName = "ChromeOS.Regmon.PolicyViolation"
		// Chrome Network Annotation hash code for annotation: 'autofill_query'. This is
		// our test annotation that is expected to be reported as a violation when the
		// PasswordManagerEnabled policy is disabled and the network call is triggered.
		annotationHashCode = 88863520
	)
	var policyValue = policy.PasswordManagerEnabled{Val: false}

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	opts := []chrome.Option{
		// FakeDMS for setting policies.
		chrome.DMSPolicy(fdms.URL),
		// Fake login, to also support setting policies.
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		// Enable Regmon feature flags.
		chrome.ExtraArgs("--enable-features=CrOSLateBootRegmonPolicyMonitoringEnabled,NetworkAnnotationMonitoring"),
	}

	// Start Chrome.
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Update policies.
	policies := []policy.Policy{&policyValue}
	if err := policyutil.ServeAndVerify(ctx, fdms, cr, policies); err != nil {
		s.Fatal("Failed to update policies: ", err)
	}

	// Open the website with the address form. This will trigger the 'autofill_query' network
	// annotation.
	conn, err := cr.NewConn(ctx, server.URL+"/"+"autofill_address_enabled.html")
	if err != nil {
		s.Fatal("Failed to open website: ", err)
	}
	defer conn.Close()

	// Check for policy violation UMA metric.
	metricValue, err := metrics.WaitForHistogram(ctx, tconn, histogramName, 1*time.Minute)
	if err != nil {
		s.Fatal("Failed to find histogram: ", err)
	}
	var actualCount int64 = 0
	for _, b := range metricValue.Buckets {
		if b.Min == annotationHashCode {
			actualCount = b.Count
		}
	}
	if actualCount == 0 {
		s.Fatal("UMA metric was not found")
	}
}
