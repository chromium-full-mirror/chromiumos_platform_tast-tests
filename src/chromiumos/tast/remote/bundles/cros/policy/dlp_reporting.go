// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"chromiumos/tast/common/tape"
	"chromiumos/tast/remote/bundles/cros/policy/dlputil"
	"chromiumos/tast/remote/policyutil"
	"chromiumos/tast/remote/reportingutil"
	"chromiumos/tast/rpc"
	dlp "chromiumos/tast/services/cros/dlp"
	"chromiumos/tast/ssh/linuxssh"
	"chromiumos/tast/testing"
)

const (

	// restrictionReportReportingEnabledUsername is the path to the secret username having report restriction level for all components and reporting enabled.
	restrictionReportReportingEnabledUsername = "dlp.restriction_level_report_reporting_enabled_username"

	// restrictionReportReportingEnabledPassword is the path to the secret password having report restriction level for all components and reporting enabled.
	restrictionReportReportingEnabledPassword = "dlp.restriction_level_report_reporting_enabled_password"
)

// testParams contains parameters for testing different DLP configurations.
type testParams struct {
	Username    string          // username for Chrome enrollment
	Password    string          // password for Chrome enrollment
	BrowserType dlp.BrowserType // which browser the test should use
	Action      dlputil.Action  // which action the test should use
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DlpReporting,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests DLP actions in report mode and check whether the correct events are generated, sent, and received from the server side",
		Contacts: []string{
			"chromeos-dlp@google.com",
		},
		BugComponent: "b:892101",
		Attr:         []string{"group:dmserver-enrollment-daily"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps: []string{
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.dlp.DataLeakPreventionService",
			"tast.cros.policy.PolicyService",
			"tast.cros.tape.Service",
		},
		Timeout: 7 * time.Minute,
		VarDeps: []string{
			restrictionReportReportingEnabledUsername,
			restrictionReportReportingEnabledPassword,
			reportingutil.ManagedChromeCustomerIDPath,
			reportingutil.EventsAPIKeyPath,
			tape.ServiceAccountVar,
		},
		Params: []testing.Param{
			{
				Name: "ash_clipboard_copy_paste",
				Val: testParams{
					Username:    restrictionReportReportingEnabledUsername,
					Password:    restrictionReportReportingEnabledPassword,
					BrowserType: dlp.BrowserType_ASH,
					Action:      dlputil.ClipboardCopyPaste,
				},
			},
			{
				Name: "lacros_clipboard_copy_paste",
				Val: testParams{
					Username:    restrictionReportReportingEnabledUsername,
					Password:    restrictionReportReportingEnabledPassword,
					BrowserType: dlp.BrowserType_LACROS,
					Action:      dlputil.ClipboardCopyPaste,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name: "ash_print",
				Val: testParams{
					Username:    restrictionReportReportingEnabledUsername,
					Password:    restrictionReportReportingEnabledPassword,
					BrowserType: dlp.BrowserType_ASH,
					Action:      dlputil.Printing,
				},
			},
			{
				Name: "lacros_print",
				Val: testParams{
					Username:    restrictionReportReportingEnabledUsername,
					Password:    restrictionReportReportingEnabledPassword,
					BrowserType: dlp.BrowserType_LACROS,
					Action:      dlputil.Printing,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name: "ash_screenshot",
				Val: testParams{
					Username:    restrictionReportReportingEnabledUsername,
					Password:    restrictionReportReportingEnabledPassword,
					BrowserType: dlp.BrowserType_ASH,
					Action:      dlputil.Screenshot,
				},
			},
			{
				Name: "lacros_screenshot",
				Val: testParams{
					Username:    restrictionReportReportingEnabledUsername,
					Password:    restrictionReportReportingEnabledPassword,
					BrowserType: dlp.BrowserType_LACROS,
					Action:      dlputil.Screenshot,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name: "ash_screenshare",
				Val: testParams{
					Username:    restrictionReportReportingEnabledUsername,
					Password:    restrictionReportReportingEnabledPassword,
					BrowserType: dlp.BrowserType_ASH,
					Action:      dlputil.Screenshare,
				},
			},
			{
				Name: "lacros_screenshare",
				Val: testParams{
					Username:    restrictionReportReportingEnabledUsername,
					Password:    restrictionReportReportingEnabledPassword,
					BrowserType: dlp.BrowserType_LACROS,
					Action:      dlputil.Screenshare,
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name: "ash_files",
				Val: testParams{
					Username:    restrictionReportReportingEnabledUsername,
					Password:    restrictionReportReportingEnabledPassword,
					BrowserType: dlp.BrowserType_ASH,
					Action:      dlputil.Files,
				},
			},
			{
				Name: "lacros_files",
				Val: testParams{
					Username:    restrictionReportReportingEnabledUsername,
					Password:    restrictionReportReportingEnabledPassword,
					BrowserType: dlp.BrowserType_LACROS,
					Action:      dlputil.Files,
				},
			},
		},
		Data: []string{
			dlputil.DataFile,
			dlputil.HTMLFile,
		},
	})
}

func DlpReporting(ctx context.Context, s *testing.State) {

	params := s.Param().(testParams)

	username := s.RequiredVar(params.Username)
	password := s.RequiredVar(params.Password)
	customerID := s.RequiredVar(reportingutil.ManagedChromeCustomerIDPath)
	APIKey := s.RequiredVar(reportingutil.EventsAPIKeyPath)
	sa := []byte(s.RequiredVar(tape.ServiceAccountVar))

	// Reset the DUT state once the test is finished.
	defer func(ctx context.Context) {
		if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Error("Failed to reset TPM after test: ", err)
		}
	}(ctx)
	// Reset the device enrollment state making sure the DUT is rebooted so that the reporting daemon works properly.
	// Local reset is not enough since it may not reboot the device.
	if err := policyutil.EnsureTPMAndSystemStateAreResetRemote(ctx, s.DUT()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}

	// Establish RPC connection to the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)
	defer reportingutil.Deprovision(ctx, cl.Conn, sa, customerID)

	// Create client instance of the DataLeakPrevention service.
	service := dlp.NewDataLeakPreventionServiceClient(cl.Conn)

	// Use the service to enroll the DUT and login.
	if _, err := service.EnrollAndLogin(ctx, &dlp.EnrollAndLoginRequest{
		Username:           username,
		Password:           password,
		DmserverUrl:        reportingutil.DmServerURL,
		ReportingServerUrl: reportingutil.ReportingServerURL,
		EnableLacros:       params.BrowserType == dlp.BrowserType_LACROS,
		EnabledFeatures:    "EncryptedReportingPipeline",
	}); err != nil {
		s.Fatal("Remote call EnrollAndLogin() failed: ", err)
	}
	defer service.StopChrome(ctx, &empty.Empty{})

	c, err := service.ClientID(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to grab client ID from device: ", err)
	}

	// We are going to filter the events also based on the test time.
	testStartTime := time.Now()

	switch params.Action {
	case dlputil.ClipboardCopyPaste:
		service.ClipboardCopyPaste(ctx, &dlp.ActionRequest{
			BrowserType: params.BrowserType,
		})
	case dlputil.Printing:
		service.Print(ctx, &dlp.ActionRequest{
			BrowserType: params.BrowserType,
		})
	case dlputil.Screenshot:
		service.Screenshot(ctx, &dlp.ActionRequest{
			BrowserType: params.BrowserType,
		})
	case dlputil.Screenshare:
		service.Screenshare(ctx, &dlp.ActionRequest{
			BrowserType: params.BrowserType,
		})
	case dlputil.Files:
		// Create a temporary directory on the DUT.
		d, err := service.CreateTempDir(ctx, &empty.Empty{})
		if err != nil {
			s.Fatal("Failed to create a temporary directory: ", err)
		}

		// Push data files to the DUT and make sure they're deleted after the test.
		dut := s.DUT()
		defer service.RemoveTempDir(ctx, &dlp.RemoveTempDirRequest{
			Path: d.Path,
		})
		if _, err := linuxssh.PutFiles(ctx, dut.Conn(), map[string]string{
			s.DataPath(dlputil.HTMLFile): filepath.Join(d.Path, dlputil.RemoteHTMLFile),
			s.DataPath(dlputil.DataFile): filepath.Join(d.Path, dlputil.RemoteDataFile),
		}, linuxssh.DereferenceSymlinks); err != nil {
			s.Fatal("Failed to send data to remote: ", err)
		}

		if _, err := service.FilesDriveCopyPaste(ctx, &dlp.ActionRequest{
			BrowserType: params.BrowserType,
			DataPath:    d.Path,
		}); err != nil {
			s.Fatal("Failed to test FilesCopyPaste: ", err)
		}
	}

	s.Log("Waiting 60 seconds to make sure events reach the server and are processed")
	if err := testing.Sleep(ctx, 60*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	events, err := dlputil.RetrieveEvents(ctx, customerID, APIKey, c.ClientId, testStartTime)
	if err != nil {
		s.Fatal("Failed to retrieve events: ", err)
	}

	if err := dlputil.ValidateReportEvents(ctx, params.Action, events); err != nil {
		s.Fatal("Failed to validate events: ", err)
	}

}
