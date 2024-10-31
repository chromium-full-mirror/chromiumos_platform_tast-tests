// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/flex"
	"go.chromium.org/tast-tests/cros/local/oobe"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	enrollmentTokenVarRevoked = "policy.TokenBasedEnrollmentRevoked.enrollment_token"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TokenBasedEnrollmentRevoked,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that token-based enrollment with a revoked token does not enroll",
		Contacts: []string{
			"cros-onboarding-team@google.com",
			"jacksontadie@google.com",
		},
		BugComponent: "b:1271043", // Chrome OS Server Projects > Enterprise Management >> Chrome Commercial Backend >> Onboarding >> Enterprise Enrollment
		Fixture:      fixture.CleanOwnership,
		Attr:         []string{"group:dmserver-enrollment-daily"},
		SoftwareDeps: []string{"flex_device", "chrome"},
		VarDeps: []string{
			enrollmentTokenVarRevoked,
			"ui.signinProfileTestExtensionManifestKey",
		},
	})
}

func TokenBasedEnrollmentRevoked(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	defer func() {
		s.Log("Cleaning up Flex config dirs")
		if err := flex.CleanUpFlexConfigDirs(cleanupCtx); err != nil {
			s.Error("Failed to clean up Flex config dirs: ", err)
		}
	}()

	s.Log("Enrollment token: " + s.RequiredVar(enrollmentTokenVarRevoked))

	oobeConfig := fmt.Sprintf(" { \"enrollmentToken\": \"%s\" }", s.RequiredVar(enrollmentTokenVarRevoked))
	if err := flex.WriteFlexOobeConfigToDisk(oobeConfig); err != nil {
		s.Fatal("Failed to seed Flex OOBE config data: ", err)
	}

	// Have to restart job after creating flex_config files as upstart config
	// conditionally bind-mounts the flex_config dir only if it's present.
	if err := upstart.RestartJob(ctx, "oobe_config_restore"); err != nil {
		s.Fatal("Failed to restart oobe_config_restore daemon: ", err)
	}

	cr, err := chrome.New(ctx, chrome.NoLogin(), chrome.DMSPolicy(policy.DMServerAlphaURL))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to create OOBE connection: ", err)
	}

	if err := oobe.WaitForWelcomeScreenVisible(ctx, oobeConn); err != nil {
		s.Fatal("Failed to wait for Welcome screen to be visible: ", err)
	}
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.WelcomeScreen.clickNext()", nil); err != nil {
		s.Fatal("Failed to click welcome page next button: ", err)
	}
	if err := oobe.ProceedThroughNetworkScreen(ctx, oobeConn); err != nil {
		s.Fatal("Failed to proceed through network screen: ", err)
	}
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.UserCreationScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the user creation screen to be visible: ", err)
	}
}
