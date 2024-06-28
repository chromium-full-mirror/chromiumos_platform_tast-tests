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
	"go.chromium.org/tast-tests/cros/services/cros/platform"
	ps "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	flexConfigInitialDirPath  = "/mnt/stateful_partition/unencrypted/flex_config"
	flexConfigInitialFilePath = "/mnt/stateful_partition/unencrypted/flex_config/config.json"
	// Path that flex_config will be in after moved to encrypted stateful partition (ESP)
	// on oobe_config_restore startup.
	flexConfigESPDirPath  = "/var/lib/oobe_config_restore/flex_config"
	flexConfigESPFilePath = "/var/lib/oobe_config_restore/flex_config/config.json"

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
			"tast.cros.platform.UpstartService",
		},
		VarDeps: []string{enrollmentTokenVarCEU, tape.ServiceAccountVar, "ui.signinProfileTestExtensionManifestKey"},
		Params: []testing.Param{
			{
				Name: "autopush",
				Val: tokenEnrollmentParams{
					dmServerURL:        policy.DMServerAlphaURL,
					enrollmentTokenVar: enrollmentTokenVarCEU,
				},
				// TODO b/346725308 Refactor to use utility and known dependency list.
				ExtraSearchFlags: []*testing.StringPair{{
					Key: "external_dependency", Value: "DMServerAlpha",
				}},
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
	defer func() {
		s.Log("Cleaning up Flex config dirs")
		if err := cleanUpFlexConfigDirs(cleanupCtx, fs); err != nil {
			s.Error("Failed to clean up Flex config dirs: ", err)
		}
	}()

	// Create Flex config dir and write enrollmentToken JSON to config file within.
	if err := fs.MkDir(ctx, flexConfigInitialDirPath, 0740); err != nil {
		s.Fatalf("Failed to create %s directory: %v", flexConfigInitialDirPath, err)
	}
	enrollmentToken := s.RequiredVar(params.enrollmentTokenVar)
	oobeConfigJSON := fmt.Sprintf("{ \"enrollmentToken\": \"%s\" }", enrollmentToken)
	if err := fs.WriteFile(ctx, flexConfigInitialFilePath, []byte(oobeConfigJSON), 0640); err != nil {
		s.Fatalf("Failed to create %s file: %v", flexConfigInitialFilePath, err)
	}

	// Chown Flex config dir so oobe_config_restore daemon can read/write it.
	chownArgs := []string{"-R", oobeConfigRestoreUID + ":" + oobeConfigRestoreGID, flexConfigInitialDirPath}
	if err := s.DUT().Conn().CommandContext(ctx, "chown", chownArgs...).Run(testexec.DumpLogOnError); err != nil {
		s.Fatalf("Failed to chown %s and its contents: %v", flexConfigInitialDirPath, err)
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

	// Have to restart job after creating flex_config files as upstart config conditionally
	// bind-mounts the flex_config dir only if it's present.
	upstartService := platform.NewUpstartServiceClient(cl.Conn)
	if _, err := upstartService.RestartJob(ctx, &platform.RestartJobRequest{JobName: "oobe_config_restore"}); err != nil {
		s.Fatal("Failed to restart oobe_config_restore daemon: ", err)
	}

	// Verify flex_config has been migrated to encrypted stateful partition after
	// oobe_config_restore restart.
	exists, err := pathExists(ctx, fs, flexConfigInitialFilePath)
	if err != nil {
		s.Fatal("Failed to check existence of Flex config file in unencrypted stateful partition: ", err)
	}
	if exists {
		s.Fatal("Flex config has not been deleted from unencrypted stateful partition")
	}
	exists, err = pathExists(ctx, fs, flexConfigESPFilePath)
	if err != nil {
		s.Fatal("Failed to check existence of Flex config file in encrypted stateful partition: ", err)
	}
	if !exists {
		s.Fatal("Flex config has not been moved to encrypted stateful partition")
	}

	policyClient := ps.NewPolicyServiceClient(cl.Conn)
	if _, err := policyClient.TokenBasedEnrollUsingChrome(ctx, &ps.TokenBasedEnrollUsingChromeRequest{
		DmserverURL: params.dmServerURL,
		ManifestKey: s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
	}); err != nil {
		s.Fatal("Failed to enroll using enrollment token: ", err)
	}

	exists, err = pathExists(ctx, fs, flexConfigESPFilePath)
	if err != nil {
		s.Fatal("Failed to check existence of Flex config file in encrypted stateful partition: ", err)
	}
	if exists {
		s.Fatal("Flex config was not cleaned up after enrollment")
	}
}

func cleanUpFlexConfigDirs(ctx context.Context, fs *dutfs.Client) error {
	if err := fs.RemoveAll(ctx, flexConfigInitialDirPath); err != nil {
		return errors.Wrapf(err, "failed to delete %s directory", flexConfigInitialDirPath)
	}
	if err := fs.RemoveAll(ctx, flexConfigESPDirPath); err != nil {
		return errors.Wrapf(err, "failed to delete %s directory", flexConfigESPDirPath)
	}
	return nil
}

func pathExists(ctx context.Context, fs *dutfs.Client, path string) (bool, error) {
	_, err := fs.Stat(ctx, path)
	if err != nil && !os.IsNotExist(err) {
		return false, errors.Wrapf(err, "unexpected error when trying to stat %s", path)
	}
	return err == nil, nil
}
