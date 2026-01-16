// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"os"
	"reflect"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DevicePolicyKeyRotation,
		BugComponent: "b:1111617",
		Fixture:      fixture.FakeDMSEnrolled,
		Desc:         "Verifies that when the device private policy key is rotated on the server, the public half is correctly stored on the device",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com", // Team
			"vsavu@google.com", // Test owner
		},
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceAutoUpdateDisabled{}, pci.Served),
		},
		Params: []testing.Param{{
			Name: "sha256_enabled",
			Val:  true,
		}, {
			Name: "sha256_disabled",
			Val:  false,
		}},
	})
}

func DevicePolicyKeyRotation(ctx context.Context, s *testing.State) {
	sha256Enabled := s.Param().(bool)
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Start a Chrome instance that will fetch policies from the FakeDMS.
	opts := []chrome.Option{
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment(),
	}
	if sha256Enabled {
		opts = append(opts, chrome.EnableFeatures("PolicyFetchWithSha256"))
	} else {
		opts = append(opts, chrome.DisableFeatures("PolicyFetchWithSha256"))
	}
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Set the policy key to the first version on the server and on the device.
	// Assumes ChromePolicyLoggedIn refreshes policies with the default blob.
	// This ensures the current key version stored on disk is 1.
	pb := policy.NewBlob()
	pb.CurrentKeyIdx = 1
	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
		s.Fatal("Failed to set initial key: ", err)
	}
	defer func() {
		// Reset the key on the server to the default value.
		if err := fdms.WritePolicyBlob(policy.NewBlob()); err != nil {
			s.Fatal("Failed to reset FakeDMS' key: ", err)
		}
	}()

	// Read the first version of the public half of the key.
	const keyFile = "/var/lib/devicesettings/owner.key"
	originalKey, err := os.ReadFile(keyFile)
	if err != nil {
		s.Fatalf("Could not read key from %s: %v", keyFile, err)
	}

	// Perform the key rotation.
	pb.CurrentKeyIdx = 2
	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
		s.Fatal("Failed to perform key rotation: ", err)
	}

	// Verify that the device rotates the public key by comparing keyFile with the old contents.
	// TODO(b/262529043): For a stronger guarantee compare the key contents
	// with the actual key on the server.
	keyFileContents, err := os.ReadFile(keyFile)
	if err != nil {
		s.Fatalf("Could not read key from %s: %v", keyFile, err)
	}
	if reflect.DeepEqual(originalKey, keyFileContents) {
		s.Fatal("Key rotation failed: key should be updated")
	}
	originalKey = keyFileContents

	// Verify that the new public key works by checking that the new policies are
	// decoded properly and keyFile stays the same.
	ps := []policy.Policy{&policy.DeviceAutoUpdateDisabled{Val: false}}
	pb.AddPolicies(ps)
	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
		s.Fatal("Failed to fetch policies with new public key: ", err)
	}
	if err := policyutil.Verify(ctx, tconn, ps); err != nil {
		s.Error("Failed to verify policies: ", err)
	}

	keyFileContents, err = os.ReadFile(keyFile)
	if err != nil {
		s.Fatalf("Could not read key from %s: %v", keyFile, err)
	}
	if !reflect.DeepEqual(originalKey, keyFileContents) {
		s.Error("Key rotation failed: key should not be updated")
	}
}
