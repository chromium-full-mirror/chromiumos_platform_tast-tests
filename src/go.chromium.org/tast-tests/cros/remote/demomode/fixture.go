// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package demomode

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	ps "go.chromium.org/tast-tests/cros/services/cros/demomode"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
)

const (
	// Set long enough timeout for SetUp() as Demo Mode OOBE flow need to download
	// Demo Mode components which could be slow depending on network latency.
	setUpTimeout    = 8 * time.Minute
	tearDownTimeout = 60 * time.Second
)

var serviceDeps = []string{
	"tast.cros.demomode.DemoModeService",
	"tast.cros.hwsec.OwnershipService",
	"tast.cros.ui.ChromeUIService",
	"tast.cros.browser.ChromeService",
	"tast.cros.tape.Service",
	"tast.cros.policy.PolicyService"}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.PostDemoModeOOBEAlpha,
		Desc: "Has proceeded through Demo Mode setup flow from OOBE",
		Contacts: []string{
			"cros-demo-mode-eng@google.com",
			"llin@chromium.org",
		},
		BugComponent: "b:812312", // ChromeOS > Software > Consumer > Demo Mode
		Impl: &fixtureImpl{
			// This user has infinite idle time-out value for demo mode, thus will not end demo mode session in middle of test.
			enrollmentUser: "admin-tast",
			dmServerURL:    policy.DMServerAlphaURL,
		},
		SetUpTimeout:    setUpTimeout,
		TearDownTimeout: tearDownTimeout,
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey", tape.ServiceAccountVar},
		ServiceDeps:     serviceDeps,
	})
	testing.AddFixture(&testing.Fixture{
		Name: fixture.PostDemoModeOOBEProd,
		Desc: "Has proceeded through Demo Mode setup flow from OOBE with Cloud Gaming customizations configured",
		Contacts: []string{
			"cros-demo-mode-eng@google.com",
			"llin@chromium.org",
		},
		BugComponent: "b:812312", // ChromeOS > Software > Consumer > Demo Mode
		Impl: &fixtureImpl{
			// This user has infinite idle time-out value for demo mode, thus will not end demo mode session in middle of test.
			enrollmentUser: "admin-tast",
			dmServerURL:    policy.DMServerProdURL,
		},
		SetUpTimeout:    setUpTimeout,
		TearDownTimeout: tearDownTimeout,
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey", tape.ServiceAccountVar},
		ServiceDeps:     serviceDeps,
	})
	testing.AddFixture(&testing.Fixture{
		Name: fixture.PostDemoModeOOBECloudGaming,
		Desc: "Has proceeded through Demo Mode setup flow from OOBE with Cloud Gaming customizations configured",
		Contacts: []string{
			"cros-demo-mode-eng@google.com",
			"llin@chromium.org",
		},
		BugComponent: "b:812312", // ChromeOS > Software > Consumer > Demo Mode
		Impl: &fixtureImpl{
			// This user has infinite idle time-out value for demo mode, thus will not end demo mode session in middle of test.
			enrollmentUser:  "admin-tast",
			enabledFeatures: []string{"CloudGamingDevice"},
			dmServerURL:     policy.DMServerAlphaURL,
		},
		SetUpTimeout:    setUpTimeout,
		TearDownTimeout: tearDownTimeout,
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey", tape.ServiceAccountVar},
		ServiceDeps:     serviceDeps,
	})
}

// fixtureImpl implements testing.FixtureImpl.
type fixtureImpl struct {
	// The user that the device enrolls into Demo Mode with. This allows us to
	// control which Organizational Unit the device enrolls into, thus the policies.
	enrollmentUser string
	// Additional features that should be enabled during Demo Mode setup.
	enabledFeatures []string
	// URL defining which DMServer environment to talk to during Demo Mode enrollment.
	dmServerURL string
}

var _ testing.FixtureImpl = &fixtureImpl{}

func (f *fixtureImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if err := policyutil.EnsureTPMAndSystemStateAreResetRemote(ctx, s.DUT()); err != nil {
		s.Fatal("Failed to reset TPM before tests: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC Service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	chromeExtraArgs := []string{
		"--demo-mode-enrolling-username=" + f.enrollmentUser,
		"--arc-start-mode=always-start",
		// Download test version of components (most importantly demo-mode-resources and demo-mode-app),
		// to catch issues before they reach prod
		// TODO(b/263269444): Consider running a version of these tests against the prod components as well
		"--component-updater=test-request",
		"--device-management-url=" + f.dmServerURL}

	defer func(ctx context.Context) {
		if err := uploadTapeDeprovisioningIDs(ctx, cl, s.DUT(), []byte(s.RequiredVar(tape.ServiceAccountVar))); err != nil {
			s.Error("Failed to upload deprovisioning ids: ", err)
		}
	}(cleanupCtx)

	chromeService := ui.NewChromeServiceClient(cl.Conn)
	if _, err := chromeService.New(ctx, &ui.NewRequest{
		LoginMode:                    ui.LoginMode_LOGIN_MODE_NO_LOGIN,
		ArcMode:                      ui.ArcMode_ARC_MODE_SUPPORTED,
		EnableFeatures:               f.enabledFeatures,
		ExtraArgs:                    chromeExtraArgs,
		SigninProfileTestExtensionId: s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
		DontSkipOobeAfterLogin:       true,
	}); err != nil {
		s.Fatal("Failed to create new Chrome at OOBE: ", err)
	}

	dumpUITreeWithScreenshotOnFailure := func(errMsg string) {
		if s.HasError() {
			faillog := ui.NewChromeUIServiceClient(cl.Conn)
			faillog.DumpUITreeWithScreenshotToFile(ctx, &ui.DumpUITreeWithScreenshotToFileRequest{
				FilePrefix: "demo_mode_oobe",
			})
		}
	}
	s.AttachErrorHandlers(dumpUITreeWithScreenshotOnFailure, dumpUITreeWithScreenshotOnFailure)

	dmsc := ps.NewDemoModeServiceClient(cl.Conn)

	if _, err = dmsc.SetUpDemoMode(ctx, &ps.SetUpDemoModeRequest{}); err != nil {
		s.Fatal("Failed to set up Demo Mode: ", err)
	}

	return nil
}

func (f *fixtureImpl) Reset(ctx context.Context) error {
	return nil
}

func (f *fixtureImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *fixtureImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *fixtureImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC Service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	stableDeviceSecret, err := getStableDeviceSecret(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get device stable secret: ", err)
	}
	s.Log("Deprovisioning device")

	if err := tapeClient.DeprovisionAndVerify(ctx, tape.WithStableDeviceSecret(stableDeviceSecret)); err != nil {
		s.Fatal("Failed to deprovision device: ", err)
	}

	if err := policyutil.EnsureTPMAndSystemStateAreResetRemote(ctx, s.DUT()); err != nil {
		s.Fatal("Failed to reset TPM after tests: ", err)
	}
}

func uploadTapeDeprovisioningIDs(ctx context.Context, rpcClient *rpc.Client, dut *dut.DUT, tapeServiceAccount []byte) error {
	policyClient := pspb.NewPolicyServiceClient(rpcClient.Conn)
	deviceAndCustomerIDResponse, err := policyClient.DeviceAndCustomerID(ctx, &empty.Empty{})
	if err != nil {
		return errors.Wrap(err, "failed to get device and customer id")
	}
	deviceID := deviceAndCustomerIDResponse.DeviceID
	customerID := deviceAndCustomerIDResponse.CustomerID

	stableDeviceSecret, err := getStableDeviceSecret(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "failed to get stable device secret")
	}

	tapeClient, err := tape.NewClient(ctx, tapeServiceAccount)
	if err != nil {
		return errors.Wrap(err, "failed to create tape client")
	}

	testing.ContextLog(ctx, "Uploading Tape deprovisioning IDs, customer ID: "+customerID+" - device ID: "+deviceID)
	if err := tapeClient.StoreDeprovisioningIDs(ctx, deviceID, customerID, stableDeviceSecret); err != nil {
		return errors.Wrap(err, "failed to store IDs in TAPE")
	}
	return nil
}

func getStableDeviceSecret(ctx context.Context, dut *dut.DUT) (string, error) {
	const stableDeviceSecretFileName = "/sys/firmware/vpd/ro/stable_device_secret_DO_NOT_SHARE"
	data, err := linuxssh.ReadFile(ctx, dut.Conn(), stableDeviceSecretFileName)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read %s", stableDeviceSecretFileName)
	}
	return string(data), nil
}
