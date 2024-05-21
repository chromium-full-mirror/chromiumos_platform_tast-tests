// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/erpserver"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast/core/testing"
)

type testTypeEnum int

const (
	publicKey testTypeEnum = iota
	signature
	keyID
)

type keysTestParam struct {
	testType testTypeEnum
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingBadServerKeys,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that our pipeline doesn't work with incorrect keys",
		Contacts: []string{
			"cros-reporting-alerts+tast@google.com",
			"albertojuarez@google.com", // Test author
		},
		Timeout:      3 * time.Minute,
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational", "group:enterprise-reporting-daily", "group:enterprise-reporting", "group:hw_agnostic"},
		Fixture:      fixture.FakeDMSEnrolled,
		VarDeps:      []string{"erpserver.key_id", "erpserver.public_key", "erpserver.private_key", "erpserver.signature"},
		Params: []testing.Param{
			{
				Name: "public_key",
				Val: keysTestParam{
					testType: publicKey,
				},
			},
			{
				Name: "signature",
				Val: keysTestParam{
					testType: signature,
				},
			},
			{
				Name: "key_id",
				Val: keysTestParam{
					testType: keyID,
				},
			},
		},
	})
}

func ReportingBadServerKeys(ctx context.Context, s *testing.State) {
	testType := s.Param().(keysTestParam).testType
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	erpPublicKey := s.RequiredVar("erpserver.public_key")
	erpPrivateKey := s.RequiredVar("erpserver.private_key")
	erpSignature := s.RequiredVar("erpserver.signature")
	erpKeyID, err := strconv.Atoi(s.RequiredVar("erpserver.key_id"))
	if err != nil {
		s.Fatal("Could not convert key ID to int: ", err)
	}

	// Change the variable depending on the type of test to make the key invalid.
	switch testType {
	case publicKey:
		erpPublicKey = strings.TrimSuffix(erpPublicKey, "=") + "A"
	case signature:
		erpSignature = strings.TrimSuffix(erpSignature, "==") + "XX"
	case keyID:
		erpKeyID += 3
	}

	// Create a new log reader.
	logReader, err := syslog.NewReader(ctx)
	if err != nil {
		s.Fatal("Failed to create log reader: ", err)
	}

	// Set buffer size to 1 since this test doesn't expect any records.
	server, err := erpserver.New(erpPublicKey, erpPrivateKey, erpSignature, erpKeyID, 1)
	if err != nil {
		s.Fatal("Reporting server creation failed: ", err)
	}

	if err = server.Start(ctx); err != nil {
		s.Fatal("Reporting server failed to start: ", err)
	}
	defer server.Close()

	// Start a Chrome instance with the fake server.
	_, err = chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.EncryptedReportingAddr(server.URL()),
		chrome.EnableFeatures(erpserver.MissiveDefaultFeature),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Chrome start failed: ", err)
	}

	// Create the filter for the log message to match.
	containsMessage := func(e *syslog.Entry) bool {
		return strings.Contains(e.Content, "Key failed verification")
	}

	// Check the log reader to see if any message that matches is there.
	if _, err := logReader.Wait(ctx, 10*time.Second, containsMessage); err != nil {
		s.Fatalf("Failed to find key verification failed message in log %s, with error: %v", syslog.MessageFile, err)
	}
}
