// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policyutil

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.TAPEEnrolled,
		Desc: "Enrolls using real DMServer by using a TAPE account",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com",
		},
		BugComponent:    "b:1111632", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Enrollment
		Impl:            &tapeEnrolledFixt{},
		SetUpTimeout:    enrollmentSetupTimeout,
		TearDownTimeout: 5 * time.Minute,
		ResetTimeout:    15 * time.Second,
		PostTestTimeout: 15 * time.Second,
		Parent:          fixture.TAPEAccount,
		ServiceDeps: []string{
			"tast.cros.policy.PolicyService",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.baserpc.FileSystem",
			"tast.cros.tape.Service",
			"tast.cros.graphics.ScreenshotService",
		},
		Vars: []string{
			tape.ServiceAccountVar,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: fixture.EnterpriseConnectorsMGSAshWebProtectDisabledEnrolled,
		Desc: "Enrolls using real DMServer by using a TAPE account with Autolaunch MGS and pre-applied policies for Ash web protect disabled",
		Contacts: []string{
			"cros-enterprise-connectors@google.com",
			"webprotect-eng@google.com",
			"mohamedaomar@google.com",
		},
		BugComponent:    "b:1111632", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Enrollment
		Impl:            &tapeEnrolledFixt{},
		SetUpTimeout:    enrollmentSetupTimeout,
		TearDownTimeout: 5 * time.Minute,
		ResetTimeout:    15 * time.Second,
		PostTestTimeout: 15 * time.Second,
		Parent:          fixture.EnterpriseConnectorsMGSAshWebProtectDisabledAccount,
		ServiceDeps: []string{
			"tast.cros.policy.PolicyService",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.baserpc.FileSystem",
			"tast.cros.tape.Service",
			"tast.cros.graphics.ScreenshotService",
		},
		Vars: []string{
			tape.ServiceAccountVar,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: fixture.EnterpriseConnectorsMGSAshWebProtectEnabledAllowEnrolled,
		Desc: "Enrolls using real DMServer by using a TAPE account with Autolaunch MGS and pre-applied policies for Ash web protect enabled and allowed",
		Contacts: []string{
			"cros-enterprise-connectors@google.com",
			"webprotect-eng@google.com",
			"mohamedaomar@google.com",
		},
		BugComponent:    "b:1111632", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Enrollment
		Impl:            &tapeEnrolledFixt{},
		SetUpTimeout:    enrollmentSetupTimeout,
		TearDownTimeout: 5 * time.Minute,
		ResetTimeout:    15 * time.Second,
		PostTestTimeout: 15 * time.Second,
		Parent:          fixture.EnterpriseConnectorsMGSAshWebProtectEnabledAllowAccount,
		ServiceDeps: []string{
			"tast.cros.policy.PolicyService",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.baserpc.FileSystem",
			"tast.cros.tape.Service",
			"tast.cros.graphics.ScreenshotService",
		},
		Vars: []string{
			tape.ServiceAccountVar,
		},
	})
	testing.AddFixture(&testing.Fixture{
		Name: fixture.EnterpriseConnectorsMGSAshWebProtectEnabledBlockEnrolled,
		Desc: "Enrolls using real DMServer by using a TAPE account with Autolaunch MGS and pre-applied policies for Ash web protect enabled and blocked",
		Contacts: []string{
			"cros-enterprise-connectors@google.com",
			"webprotect-eng@google.com",
			"mohamedaomar@google.com",
		},
		BugComponent:    "b:1111632", // ChromeOS > Software > Commercial (Enterprise) > Remote Management > Enrollment
		Impl:            &tapeEnrolledFixt{},
		SetUpTimeout:    enrollmentSetupTimeout,
		TearDownTimeout: 5 * time.Minute,
		ResetTimeout:    15 * time.Second,
		PostTestTimeout: 15 * time.Second,
		Parent:          fixture.EnterpriseConnectorsMGSAshWebProtectEnabledBlockAccount,
		ServiceDeps: []string{
			"tast.cros.policy.PolicyService",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.baserpc.FileSystem",
			"tast.cros.tape.Service",
			"tast.cros.graphics.ScreenshotService",
		},
		Vars: []string{
			tape.ServiceAccountVar,
		},
	})
}

type tapeEnrolledFixt struct {
	account   *fixture.TAPEAccountData
	rpcClient *rpc.Client
}

func (e *tapeEnrolledFixt) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	e.account = s.ParentValue().(*fixture.TAPEAccountData)

	ok := false

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	// Make sure to clean up on error.
	defer func(ctx context.Context) {
		if !ok {
			if err := EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
				s.Error("Failed to reset TPM after setup failure: ", err)
			}
		}
	}(cleanupCtx)

	if err := EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}

	rpcClient, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer rpcClient.Close(cleanupCtx)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	defer func(ctx context.Context) {
		if !ok {
			if err := tapeClient.DeprovisionHelper(ctx, rpcClient, e.account.OrgUnitPath); err != nil {
				s.Fatal("Failed to deprovision device: ", err)
			}
		}
	}(cleanupCtx)

	// Always dump the logs.
	defer func(ctx context.Context) {
		screenshotService := graphics.NewScreenshotServiceClient(rpcClient.Conn)
		if _, err := screenshotService.CaptureScreenshot(ctx,
			&graphics.CaptureScreenshotRequest{FilePrefix: "enrollment"},
		); err != nil {
			testing.ContextLog(ctx, "Failed to capture screenshot: ", err)
		}

		chromeDir := path.Join(s.OutDir(), "Chrome")
		if err := os.Mkdir(chromeDir, 0777); err != nil {
			testing.ContextLog(ctx, "Failed to create Chrome dir: ", err)
		}

		if err := linuxssh.GetFile(ctx, s.DUT().Conn(), "/var/log/chrome/chrome", filepath.Join(chromeDir, "chrome.log"), linuxssh.DereferenceSymlinks); err != nil {
			testing.ContextLog(ctx, "Failed to dump Chrome log: ", err)
		}

		uiDir := "/var/log/ui"
		uiDirHost := path.Join(s.OutDir(), "ui")
		if err := os.Mkdir(uiDirHost, 0777); err != nil {
			testing.ContextLog(ctx, "Failed to create ui dir: ", err)
		}

		if err := linuxssh.GetFile(ctx, s.DUT().Conn(), uiDir, uiDirHost, linuxssh.DereferenceSymlinks); err != nil {
			testing.ContextLog(ctx, "Failed to dump ui dir: ", err)
		}
	}(cleanupCtx)

	policyClient := pspb.NewPolicyServiceClient(rpcClient.Conn)
	if _, err := policyClient.GAIAEnrollUsingChrome(ctx, &pspb.GAIAEnrollUsingChromeRequest{
		Username:    e.account.Username,
		Password:    e.account.Password,
		DmserverURL: policy.DMServerAlphaURL,
	}); err != nil {
		s.Fatal("Failed to enroll using Chrome: ", err)
	}

	if _, err := policyClient.StopChrome(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to stop Chrome: ", err)
	}

	e.rpcClient, err = rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Error("Failed to connect to DUT: ", err)
	}

	ok = true

	return e.account
}

func (e *tapeEnrolledFixt) TearDown(ctx context.Context, s *testing.FixtState) {
	defer func(ctx context.Context) {
		if err := EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Fatal("Failed to reset TPM: ", err)
		}
	}(ctx)

	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	if e.rpcClient != nil {
		e.rpcClient.Close(ctx)
	}

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	if err := tapeClient.DeprovisionHelper(ctx, cl, e.account.OrgUnitPath); err != nil {
		s.Fatal("Failed to deprovision device: ", err)
	}
}

// Check if device state has been lost.
func (e *tapeEnrolledFixt) Reset(ctx context.Context) error {
	return checkEnrollment(ctx, e.rpcClient)
}

func (*tapeEnrolledFixt) PreTest(ctx context.Context, s *testing.FixtTestState) {}

// Tests need to make sure not to clear enrollment. Report a test error if it happens.
func (e *tapeEnrolledFixt) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if err := checkEnrollment(ctx, e.rpcClient); err != nil {
		s.Error("Enrollement lost: ", err)
	}
}
