// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	ps "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	flexConfigDirPath  = "/mnt/stateful_partition/unencrypted/flex_config"
	flexConfigFilePath = "/mnt/stateful_partition/unencrypted/flex_config/config.json"

	enrollmentTokenVarCEU = "policy.TokenBasedEnrollment.enrollment_token"

	oobeConfigRestoreUID = "20121"
	oobeConfigRestoreGID = "20121"

	enrollmentTimeout = 5 * time.Minute
)

type tokenEnrollmentParams struct {
	// Variable name of enrollment token used to enroll. Different tokens may
	// be in different OUs, may be revoked, or may be associated with different
	// upgrade types.
	enrollmentTokenVar string
	// URL defining which DMServer environment to talk to during enrollment.
	dmServerURL string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         TokenBasedEnrollment,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify success of token-based enrollment on ChromeOS Flex",
		Contacts: []string{
			"cros-onboarding-team@google.com",
			"jacksontadie@google.com",
		},
		BugComponent: "b:1271043", // Chrome OS Server Projects > Enterprise Management >> Chrome Commercial Backend >> Onboarding >> Enterprise Enrollment
		Fixture:      fixture.CleanOwnership,
		Attr:         []string{"group:dmserver-enrollment-daily"},
		SoftwareDeps: []string{"reven_oobe_config", "chrome"},
		ServiceDeps: []string{
			"tast.cros.policy.PolicyService",
			dutfs.ServiceName,
			"tast.cros.tape.Service",
		},
		VarDeps: []string{enrollmentTokenVarCEU, tape.ServiceAccountVar, "ui.signinProfileTestExtensionManifestKey"},
		Params: []testing.Param{
			{
				Name: "autopush",
				Val: tokenEnrollmentParams{
					dmServerURL:        policy.DMServerAlphaURL,
					enrollmentTokenVar: enrollmentTokenVarCEU,
				},
			},
		},
	})
}

func TokenBasedEnrollment(ctx context.Context, s *testing.State) {
	params := s.Param().(tokenEnrollmentParams)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 20*time.Second)
	defer cancel()

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	fs := dutfs.NewClient(cl.Conn)
	defer cleanUpFlexConfigDir(cleanupCtx, s, fs)

	// Create Flex config dir and write enrollmentToken JSON to config file within.
	if err := fs.MkDir(ctx, flexConfigDirPath, 0740); err != nil {
		s.Fatalf("Failed to create %s directory: %v", flexConfigDirPath, err)
	}
	enrollmentToken := s.RequiredVar(params.enrollmentTokenVar)
	oobeConfigJSON := fmt.Sprintf("{ \"enrollmentToken\": \"%s\" }", enrollmentToken)
	if err := fs.WriteFile(ctx, flexConfigFilePath, []byte(oobeConfigJSON), 0640); err != nil {
		s.Fatalf("Failed to create %s file: %v", flexConfigFilePath, err)
	}

	// Chown Flex config dir so oobe_config_restore daemon can read/write it.
	chownArgs := []string{"-R", oobeConfigRestoreUID + ":" + oobeConfigRestoreGID, flexConfigDirPath}
	if err := s.DUT().Conn().CommandContext(ctx, "chown", chownArgs...).Run(testexec.DumpLogOnError); err != nil {
		s.Fatalf("Failed to chown %s and its contents: %v", flexConfigDirPath, err)
	}

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}
	defer func(ctx context.Context) {
		if err := tapeClient.DeprovisionHelper(ctx, cl, "unused"); err != nil {
			s.Fatal("Failed to deprovision device: ", err)
		}
	}(cleanupCtx)

	policyClient := ps.NewPolicyServiceClient(cl.Conn)

	if _, err := policyClient.TokenBasedEnrollUsingChrome(ctx, &ps.TokenBasedEnrollUsingChromeRequest{
		DmserverURL: params.dmServerURL,
		ManifestKey: s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
	}); err != nil {
		s.Fatal("Failed to enroll using enrollment token: ", err)
	}

	_, err = os.Stat(flexConfigFilePath)
	if os.IsExist(err) {
		s.Fatal("Flex config has not been cleaned up after enrollment")
	}
	if err != nil && !os.IsNotExist(err) {
		s.Fatal("Unexpected error when trying to stat Flex config file: ", err)
	}
}

func cleanUpFlexConfigDir(ctx context.Context, s *testing.State, fs *dutfs.Client) {
	s.Log("Cleaning up Flex config dir")
	if err := fs.RemoveAll(ctx, flexConfigDirPath); err != nil {
		s.Fatal("Failed to delete Flex config dir: ", err)
	}
}
