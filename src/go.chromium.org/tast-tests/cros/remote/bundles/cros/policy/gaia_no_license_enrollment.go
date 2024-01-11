// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

type testResources struct {
	username string // username for Chrome login
	password string // password to login
	dmserver string // device management server url
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         GAIANoLicenseEnrollment,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Try to GAIA Enroll a device to a domain with no licenses; confirm error happens",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com",
		},
		BugComponent: "b:1111632",
		Attr:         []string{"group:dpanel-end2end", "group:dmserver-enrollment-daily"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps: []string{
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.policy.PolicyService",
			"tast.cros.graphics.ScreenshotService",
		},
		Fixture: fixture.CleanOwnership,
		Timeout: 4 * time.Minute,
		Params: []testing.Param{
			{
				Name: "autopush",
				Val: testResources{
					username: "policy.GAIANoLicenseEnrollment.user_name",
					password: "policy.GAIANoLicenseEnrollment.password",
					dmserver: policy.DMServerAlphaURL,
				},
			},
			{
				Name: "staging",
				Val: testResources{
					username: "policy.GAIANoLicenseEnrollment.user_name",
					password: "policy.GAIANoLicenseEnrollment.password",
					dmserver: policy.DMServerStagingURL,
				},
			},
		},
		Vars: []string{
			"policy.GAIANoLicenseEnrollment.user_name",
			"policy.GAIANoLicenseEnrollment.password",
		},
	})
}

func GAIANoLicenseEnrollment(ctx context.Context, s *testing.State) {
	param := s.Param().(testResources)
	username := s.RequiredVar(param.username)
	password := s.RequiredVar(param.password)
	dmServerURL := param.dmserver

	// Shorten deadline to leave time separately for logging and cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 20*time.Second)
	defer cancel()

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	screenshotService := graphics.NewScreenshotServiceClient(cl.Conn)
	captureScreenshotOnError := func(ctx context.Context, hasError func() bool) {
		if !hasError() {
			return
		}

		screenshotService.CaptureScreenshot(ctx, &graphics.CaptureScreenshotRequest{FilePrefix: "enrollmentError"})
	}
	defer captureScreenshotOnError(cleanupCtx, s.HasError)

	policyClient := pspb.NewPolicyServiceClient(cl.Conn)

	if _, err := policyClient.StartNewChromeReader(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to initialize syslog reader: ", err)
	}

	if _, err := policyClient.GAIAEnrollUsingChrome(ctx, &pspb.GAIAEnrollUsingChromeRequest{
		Username:    username,
		Password:    password,
		DmserverURL: dmServerURL,
	}); err == nil {
		s.Fatal("Enrollment was successful and it shouldn't have been: ", err)
	}

	if _, err := policyClient.WaitForEnrollmentError(ctx, &empty.Empty{}); err != nil {
		s.Error("Failed to get error message: ", err)
	}

}
