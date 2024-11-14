// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentd tests security event reporting to missive.
package secagentd

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"

	fe "go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/fileeventsimpl"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/fixture"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	pb "go.chromium.org/tast-tests/cros/services/cros/secagentd"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FileEvents,
		Desc: "Checks that XDR network events are correctly being reported",
		Contacts: []string{
			"cros-enterprise-security@google.com",
			"aashay@google.com",
			"jasonling@google.com", // Author
		},
		// ChromeOS > Security > ChromeOS Enterprise Security
		BugComponent: "b:1208373",
		Attr:         []string{},
		Timeout:      3 * time.Minute,
		SoftwareDeps: []string{"bpf", "chrome", "shipping_kernel"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Fixture:      fixture.LoggedInWithFileEventsEnabled,
		Params: []testing.Param{{
			Name:      "user_fs",
			Val:       pb.TestCase_USER_FILES,
			ExtraAttr: []string{"group:mainline", "group:criticalstaging", "informational"},
		}, {
			Name:      "user_credential",
			Val:       pb.TestCase_USER_CREDENTIAL,
			ExtraAttr: []string{"group:mainline", "group:criticalstaging", "informational"},
		}, {
			Name:      "cookies",
			Val:       pb.TestCase_COOKIES,
			ExtraAttr: []string{"group:mainline", "group:criticalstaging", "informational"},
		}, {
			Name:      "tpm_key",
			Val:       pb.TestCase_TPM_KEY,
			ExtraAttr: []string{"group:mainline", "group:criticalstaging", "informational"},
		}, {
			Name:      "auth_factors",
			Val:       pb.TestCase_AUTH_FACTORS,
			ExtraAttr: []string{"group:mainline", "group:criticalstaging", "informational"},
		}, {
			Name:      "system_passwords",
			Val:       pb.TestCase_SYSTEM_PASSWORD,
			ExtraAttr: []string{"group:mainline", "group:criticalstaging", "informational"},
		},
		},
	})
}

// FileEvents triggers various file events in different monitored sensitive areas and verifies that the
// correct events are sent over dbus.
func FileEvents(ctx context.Context, s *testing.State) {
	felogic := fe.CreateForLocalTest(s)
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	normalizedUser := cr.NormalizedUser()
	downloadsPath, _ := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	systemPath, _ := cryptohome.SystemPath(ctx, normalizedUser)
	userPath, _ := cryptohome.UserPath(ctx, normalizedUser)
	mountedVaultPath, _ := cryptohome.MountedVaultPath(ctx, normalizedUser)
	hashedUser, _ := cryptohome.UserHash(ctx, normalizedUser)
	s.Log("chrome normalized user:", normalizedUser)
	s.Log("cryptohome systemspath:", systemPath)
	s.Log("cryptohome downloadspath:", downloadsPath)
	s.Log("cryptohome userpath:", userPath)
	s.Log("cryptohome mountedVaultPath:", mountedVaultPath)
	s.Log("cryptohome hashed user:", hashedUser)
	testCase, err := fe.GetFileEventDetails(ctx, s.Param().(pb.TestCase), cr)
	if err != nil {
		s.Fatal("Invalid test case: ", err)
	}
	felogic.DoTest(ctx, testCase)
}
