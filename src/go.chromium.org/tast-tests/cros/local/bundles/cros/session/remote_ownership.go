// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package session

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast-tests/cros/local/session/ownership"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RemoteOwnership,
		Desc:         "Verifies that Ownership API can be used to set device policies (as an enterprise might do)",
		BugComponent: "b:1331478", // ChromeOS > Software > Core > SessionManager
		Contacts: []string{
			"chromeos-session-manager@google.com",
			"hidehiko@chromium.org",
		},
		Data: []string{"testcert.p12"},
		Attr: []string{"group:mainline", "group:hw_agnostic"},
	})
}

func RemoteOwnership(ctx context.Context, s *testing.State) {
	if err := session.SetUpDevice(ctx); err != nil {
		s.Fatal("Failed to reset device ownership: ", err)
	}

	privKey, err := session.ExtractPrivKey(s.DataPath("testcert.p12"))
	if err != nil {
		s.Fatal("Failed to parse PKCS #12 file: ", err)
	}

	sm, err := session.NewSessionManager(ctx)
	if err != nil {
		s.Fatal("Failed to create session_manager binding: ", err)
	}
	if err := session.PrepareChromeForPolicyTesting(ctx, sm); err != nil {
		s.Fatal("Failed to prepare Chrome for testing: ", err)
	}

	const (
		testUser = "test@foo.com"
		testPass = "test_password"
	)

	// 1. Initial policy set up (no session).
	settings := ownership.BuildTestSettings(testUser)
	if err := session.StoreSettings(ctx, sm, testUser, privKey, nil, settings); err != nil {
		s.Fatal("Failed to store settings: ", err)
	}
	if retrieved, err := session.RetrieveSettings(ctx, sm); err != nil {
		s.Fatal("Failed to retrieve settings: ", err)
	} else if diff := cmp.Diff(settings, retrieved, protocmp.Transform()); diff != "" {
		const diffName = "diff.txt"
		if err = os.WriteFile(filepath.Join(s.OutDir(), diffName), []byte(diff), 0644); err != nil {
			s.Error("Failed to write diff: ", err)
		}
		s.Fatal("Unexpected settings were retrieved. Diff is found in ", diffName)
	}

	// 2. Rotate key gracefully on the login screen (no session).
	// This should succeed because graceful rotation (with oldKey) is always allowed.
	newPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		s.Fatal("Failed to generate RSA key: ", err)
	}
	if err := session.StoreSettings(ctx, sm, testUser, newPrivKey, privKey, settings); err != nil {
		s.Fatal("Failed to rotate key gracefully on login screen: ", err)
	}
	if retrieved, err := session.RetrieveSettings(ctx, sm); err != nil {
		s.Fatal("Failed to retrieve rotated settings: ", err)
	} else if diff := cmp.Diff(settings, retrieved, protocmp.Transform()); diff != "" {
		const diffName = "diff-rotated.txt"
		if err = os.WriteFile(filepath.Join(s.OutDir(), diffName), []byte(diff), 0644); err != nil {
			s.Error("Failed to write diff: ", err)
		}
		s.Fatal("Unexpected rotated settings were retrieved. Diff is found in ", diffName)
	}

	// 3. Force re-key (clobber) in session (owner signed in).
	// Create clean vault for the test user.
	if err = cryptohome.RemoveVault(ctx, testUser); err != nil {
		s.Fatal("Failed to remove vault: ", err)
	}
	if err = cryptohome.CreateVault(ctx, testUser, testPass); err != nil {
		s.Fatal("Failed to create vault: ", err)
	}
	// Start a session for the owner user.
	if err = sm.StartSession(ctx, testUser, ""); err != nil {
		s.Fatal("Failed to start session: ", err)
	}

	// Force re-key the device (clobber).
	// This should succeed now because the owner is signed in.
	clobberPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		s.Fatal("Failed to generate RSA key: ", err)
	}
	if err := session.StoreSettings(ctx, sm, testUser, clobberPrivKey, nil, settings); err != nil {
		s.Fatal("Failed to store clobbered settings with owner signed in: ", err)
	}
	if retrieved, err := session.RetrieveSettings(ctx, sm); err != nil {
		s.Fatal("Failed to retrieve clobbered settings: ", err)
	} else if diff := cmp.Diff(settings, retrieved, protocmp.Transform()); diff != "" {
		const diffName = "diff-clobbered.txt"
		if err = os.WriteFile(filepath.Join(s.OutDir(), diffName), []byte(diff), 0644); err != nil {
			s.Error("Failed to write diff: ", err)
		}
		s.Fatal("Unexpected clobbered settings were retrieved. Diff is found in ", diffName)
	}
}
