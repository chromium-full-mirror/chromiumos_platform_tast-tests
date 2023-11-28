// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/reportingutil"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/policy/dlputil"
	dlp "go.chromium.org/tast-tests/cros/services/cros/dlp"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataLeakPreventionRulesListFilesDrive,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with file Google Drive restriction",
		Contacts: []string{
			"chromeos-dlp@google.com",
			"aidazolic@google.com",
		},
		// ChromeOS > Software > Commercial (Enterprise) > DLP (Data Loss Prevention)
		BugComponent: "b:892101",
		Attr:         []string{"group:data-leak-prevention-dmserver-enrollment-daily"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps: []string{
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.dlp.DataLeakPreventionService",
			"tast.cros.policy.PolicyService",
			"tast.cros.tape.Service",
		},
		Timeout: 7 * time.Minute,
		VarDeps: []string{
			dlputil.RestrictionWarnReportingEnabledUsernameAsh,
			dlputil.RestrictionWarnReportingEnabledPasswordAsh,
		},
		Fixture: fixture.CleanOwnership,
		Data: []string{
			dlputil.DataFile,
			dlputil.HTMLFile,
		},
	})
}

func DataLeakPreventionRulesListFilesDrive(ctx context.Context, s *testing.State) {
	username := s.RequiredVar(dlputil.RestrictionWarnReportingEnabledUsernameAsh)
	password := s.RequiredVar(dlputil.RestrictionWarnReportingEnabledPasswordAsh)

	// Establish RPC connection to the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	// Create client instance of the DataLeakPrevention service.
	service := dlp.NewDataLeakPreventionServiceClient(cl.Conn)

	// Enroll the DUT and login.
	if _, err := service.EnrollAndLogin(ctx, &dlp.EnrollAndLoginRequest{
		Username:           username,
		Password:           password,
		DmserverUrl:        policy.DMServerAlphaURL,
		ReportingServerUrl: reportingutil.ReportingServerURL,
		EnableLacros:       false,
	}); err != nil {
		s.Fatal("Remote call EnrollAndLogin() failed: ", err)
	}
	defer service.StopChrome(ctx, &empty.Empty{})

	// Create a temporary directory on the DUT and make sure it's deleted after test.
	d, err := service.CreateTempDir(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to create a temporary directory: ", err)
	}
	defer service.RemoveTempDir(ctx, &dlp.RemoveTempDirRequest{
		Path: d.Path,
	})

	// Push data files to the DUT.
	dut := s.DUT()
	if _, err := linuxssh.PutFiles(ctx, dut.Conn(), map[string]string{
		s.DataPath(dlputil.HTMLFile): filepath.Join(d.Path, dlputil.RemoteHTMLFile),
		s.DataPath(dlputil.DataFile): filepath.Join(d.Path, dlputil.RemoteDataFile),
	}, linuxssh.DereferenceSymlinks); err != nil {
		s.Fatal("Failed to send data to remote: ", err)
	}

	if _, err := service.TestCopyFileToDrive(ctx, &dlp.TestCopyFileToDriveRequest{
		DataPath: d.Path,
	}); err != nil {
		s.Fatal("Failed to TestCopyFileToDrive: ", err)
	}
}
