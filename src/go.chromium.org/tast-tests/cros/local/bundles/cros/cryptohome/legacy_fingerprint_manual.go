// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/cryptohome/internal"
	"go.chromium.org/tast-tests/cros/local/chrome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LegacyFingerprintManual,
		Desc:         "Checks that legacy fingerprint functions correctly through manual interaction with FP sensor",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts: []string{
			"cryptohome-core@google.com",
			"chromeos-fingerprint@google.com",
			"hcyang@google.com",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		SoftwareDeps: []string{"biometrics_daemon", "chrome"},
		HardwareDeps: hwdep.D(hwdep.Fingerprint()),
		Timeout:      5 * time.Minute,
	})
}

func LegacyFingerprintManual(ctx context.Context, s *testing.State) {
	const (
		userName           = "foo@bar.baz"
		userPassword       = "secret"
		indexFingerLabel   = "index-finger"
		indexFingerName    = "index finger"
		middleFingerLabel  = "middle-finger"
		middleFingerName   = "middle finger"
		thumbName          = "thumb"
		maxFailureAttempts = 3
	)

	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)
	biodClient := hwsec.NewBiodClient(cmdRunner)

	cr, err := chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: userName, Pass: userPassword}))
	if err != nil {
		s.Fatal("Failed to start Chrome at login screen: ", err)
	}
	defer client.UnmountAndRemoveVault(ctxForCleanUp, userName)
	defer cr.Close(ctxForCleanUp)

	sanitizedUsername, err := client.GetSanitizedUsername(ctx, userName, false)
	if err != nil {
		s.Fatalf("Failed to get sanitized username for %s: %v", userName, err)
	}

	// Step 1: Enroll the index finger.
	internal.PromptFingerEnroll(ctx, indexFingerName)
	if err := biodClient.Enroll(ctx, sanitizedUsername, indexFingerLabel); err != nil {
		s.Fatal("Enroll failed: ", err)
	}
	internal.PromptFingerLift(ctx)

	// Step 2: Enroll the middle finger.
	internal.PromptFingerEnroll(ctx, middleFingerName)
	if err := biodClient.Enroll(ctx, sanitizedUsername, middleFingerLabel); err != nil {
		s.Fatal("Enroll failed: ", err)
	}
	internal.PromptFingerLift(ctx)

	// Step 3: Ensure index finger authenticates successfully within 3 touches.
	internal.PromptFingerMatch(ctx, indexFingerName)
	if err := biodClient.MatchExpectSuccess(ctx, maxFailureAttempts); err != nil {
		s.Fatal("Match failed: ", err)
	}
	internal.PromptFingerLift(ctx)

	// Step 4: Ensure middle finger authenticates successfully within 3 touches.
	internal.PromptFingerMatch(ctx, middleFingerName)
	if err := biodClient.MatchExpectSuccess(ctx, maxFailureAttempts); err != nil {
		s.Fatal("Match failed: ", err)
	}
	internal.PromptFingerLift(ctx)

	// Step 5: Ensure thumb can't match successfully.
	internal.PromptFingerMatch(ctx, thumbName)
	if err := biodClient.MatchExpectFailure(ctx); err != nil {
		s.Fatal("Failed to expect a match failure: ", err)
	}
}
