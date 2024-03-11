// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/reportingutil"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/policy/dlputil"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	dlp "go.chromium.org/tast-tests/cros/services/cros/dlp"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

// testParams contains parameters for testing different DLP configurations.
type testParams struct {
	Username    string               // username for Chrome enrollment
	Password    string               // password for Chrome enrollment
	BrowserType dlp.BrowserType      // which browser the test should use
	Action      dlputil.Action       // which action the test should use
	Counts      dlputil.EventsCounts // which and how many report events to expect
}

// eventsCountsToMode returns the mode associated with the given events counts.
func eventsCountsToMode(count *dlputil.EventsCounts) dlp.Mode {
	if count.Block > 0 {
		return dlp.Mode_BLOCK
	}
	if count.Report > 0 {
		return dlp.Mode_REPORT
	}
	if count.WarnProceed > 0 {
		return dlp.Mode_WARN_PROCEED
	}
	if count.Warn > 0 {
		return dlp.Mode_WARN_CANCEL
	}
	return dlp.Mode_ALLOW
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
		Attr:         []string{"group:data-leak-prevention-dmserver-enrollment-daily", "group:enterprise-reporting"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps: []string{
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.dlp.DataLeakPreventionService",
			"tast.cros.policy.PolicyService",
			"tast.cros.tape.Service",
		},
		Timeout: 10 * time.Minute,
		VarDeps: []string{
			dlputil.RestrictionReportReportingEnabledUsernameAsh,
			dlputil.RestrictionReportReportingEnabledPasswordAsh,
			dlputil.RestrictionReportReportingEnabledUsernameLacros,
			dlputil.RestrictionReportReportingEnabledPasswordLacros,
			dlputil.RestrictionBlockReportingEnabledUsernameAsh,
			dlputil.RestrictionBlockReportingEnabledPasswordAsh,
			dlputil.RestrictionWarnReportingEnabledUsernameAsh,
			dlputil.RestrictionWarnReportingEnabledPasswordAsh,
			reportingutil.ManagedChromeCustomerIDPath,
			reportingutil.EventsAPIKeyPath,
			tape.ServiceAccountVar,
		},
		Params: []testing.Param{
			{
				Name: "ash_clipboard_copy_paste",
				Val: testParams{
					Username:    dlputil.RestrictionReportReportingEnabledUsernameAsh,
					Password:    dlputil.RestrictionReportReportingEnabledPasswordAsh,
					BrowserType: dlp.BrowserType_ASH,
					Action:      dlputil.ClipboardCopyPaste,
					Counts:      dlputil.EventsCounts{Report: 1},
				},
			},
			{
				Name: "lacros_clipboard_copy_paste",
				Val: testParams{
					Username:    dlputil.RestrictionReportReportingEnabledUsernameLacros,
					Password:    dlputil.RestrictionReportReportingEnabledPasswordLacros,
					BrowserType: dlp.BrowserType_LACROS,
					Action:      dlputil.ClipboardCopyPaste,
					Counts:      dlputil.EventsCounts{Report: 1},
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name: "ash_print",
				Val: testParams{
					Username:    dlputil.RestrictionReportReportingEnabledUsernameAsh,
					Password:    dlputil.RestrictionReportReportingEnabledPasswordAsh,
					BrowserType: dlp.BrowserType_ASH,
					Action:      dlputil.Printing,
					Counts:      dlputil.EventsCounts{Report: 1},
				},
			},
			{
				Name: "lacros_print",
				Val: testParams{
					Username:    dlputil.RestrictionReportReportingEnabledUsernameLacros,
					Password:    dlputil.RestrictionReportReportingEnabledPasswordLacros,
					BrowserType: dlp.BrowserType_LACROS,
					Action:      dlputil.Printing,
					Counts:      dlputil.EventsCounts{Report: 1},
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name: "ash_screenshot",
				Val: testParams{
					Username:    dlputil.RestrictionReportReportingEnabledUsernameAsh,
					Password:    dlputil.RestrictionReportReportingEnabledPasswordAsh,
					BrowserType: dlp.BrowserType_ASH,
					Action:      dlputil.Screenshot,
					Counts:      dlputil.EventsCounts{Report: 1},
				},
			},
			{
				Name: "lacros_screenshot",
				Val: testParams{
					Username:    dlputil.RestrictionReportReportingEnabledUsernameLacros,
					Password:    dlputil.RestrictionReportReportingEnabledPasswordLacros,
					BrowserType: dlp.BrowserType_LACROS,
					Action:      dlputil.Screenshot,
					Counts:      dlputil.EventsCounts{Report: 1},
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name: "ash_screenshare",
				Val: testParams{
					Username:    dlputil.RestrictionReportReportingEnabledUsernameAsh,
					Password:    dlputil.RestrictionReportReportingEnabledPasswordAsh,
					BrowserType: dlp.BrowserType_ASH,
					Action:      dlputil.Screenshare,
					Counts:      dlputil.EventsCounts{Report: 1},
				},
			},
			{
				Name: "lacros_screenshare",
				Val: testParams{
					Username:    dlputil.RestrictionReportReportingEnabledUsernameLacros,
					Password:    dlputil.RestrictionReportReportingEnabledPasswordLacros,
					BrowserType: dlp.BrowserType_LACROS,
					Action:      dlputil.Screenshare,
					Counts:      dlputil.EventsCounts{Report: 1},
				},
				ExtraSoftwareDeps: []string{"lacros"},
			},
			{
				Name: "ash_files",
				Val: testParams{
					Username:    dlputil.RestrictionReportReportingEnabledUsernameAsh,
					Password:    dlputil.RestrictionReportReportingEnabledPasswordAsh,
					BrowserType: dlp.BrowserType_ASH,
					Action:      dlputil.Files,
					Counts:      dlputil.EventsCounts{Report: 1},
				},
			},
			{
				Name: "lacros_files",
				Val: testParams{
					Username:    dlputil.RestrictionReportReportingEnabledUsernameLacros,
					Password:    dlputil.RestrictionReportReportingEnabledPasswordLacros,
					BrowserType: dlp.BrowserType_LACROS,
					Action:      dlputil.Files,
					Counts:      dlputil.EventsCounts{Report: 1},
				},
			},
			{
				Name: "ash_block_files",
				Val: testParams{
					Username:    dlputil.RestrictionBlockReportingEnabledUsernameAsh,
					Password:    dlputil.RestrictionBlockReportingEnabledPasswordAsh,
					BrowserType: dlp.BrowserType_ASH,
					Action:      dlputil.Files,
					Counts:      dlputil.EventsCounts{Block: 1},
				},
			},
			{
				Name: "ash_warn_files",
				Val: testParams{
					Username:    dlputil.RestrictionWarnReportingEnabledUsernameAsh,
					Password:    dlputil.RestrictionWarnReportingEnabledPasswordAsh,
					BrowserType: dlp.BrowserType_ASH,
					Action:      dlputil.Files,
					Counts:      dlputil.EventsCounts{Warn: 1},
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

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

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
	}(cleanupCtx)
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
	defer cl.Close(cleanupCtx)
	defer reportingutil.Deprovision(cleanupCtx, cl.Conn, sa, customerID)

	// Create client instance of the DataLeakPrevention service.
	service := dlp.NewDataLeakPreventionServiceClient(cl.Conn)

	// Use the service to enroll the DUT and login.
	if _, err := service.EnrollAndLogin(ctx, &dlp.EnrollAndLoginRequest{
		Username:           username,
		Password:           password,
		DmserverUrl:        policy.DMServerAlphaURL,
		ReportingServerUrl: reportingutil.ReportingServerURL,
		EnableLacros:       params.BrowserType == dlp.BrowserType_LACROS,
		EnabledFeatures:    "EncryptedReportingPipeline, ClientAutomatedTest",
	}); err != nil {
		s.Fatal("Remote call EnrollAndLogin() failed: ", err)
	}
	defer service.StopChrome(cleanupCtx, &empty.Empty{})

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
		// Create a temporary directory on the DUT and make sure it's deleted after test.
		d, err := service.CreateTempDir(ctx, &empty.Empty{})
		if err != nil {
			s.Fatal("Failed to create a temporary directory: ", err)
		}
		defer service.RemoveTempDir(cleanupCtx, &dlp.RemoveTempDirRequest{
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

		if _, err := service.FilesDriveCopyPaste(ctx, &dlp.ActionRequest{
			BrowserType: params.BrowserType,
			DataPath:    d.Path,
			Mode:        eventsCountsToMode(&params.Counts),
		}); err != nil {
			s.Fatal("Failed to test FilesCopyPaste: ", err)
		}
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		events, err := dlputil.RetrieveEvents(ctx, customerID, APIKey, c.ClientId, testStartTime)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to retrieve events"))
		}

		if events.Size() < params.Counts.Sum() {
			// Events may not have reached the server yet. Poll again.
			return errors.New("cannot find the expected number of events")
		}

		if err := dlputil.ValidateReportEvents(ctx, params.Action, events, &params.Counts); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to validate events"))
		}

		return nil
	}, &testing.PollOptions{
		Timeout:  4 * time.Minute,
		Interval: 20 * time.Second,
	}); err != nil {
		s.Errorf("Failed to validate DLP events: %v:", err)
	}

}
