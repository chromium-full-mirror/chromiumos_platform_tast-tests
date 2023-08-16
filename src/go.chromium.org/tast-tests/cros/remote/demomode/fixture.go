// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package demomode

import (
	"context"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/demomode/constants"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	ps "go.chromium.org/tast-tests/cros/services/cros/demomode"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
)

const (
	setUpTimeout    = 350 * time.Second
	tearDownTimeout = 25 * time.Second
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.PostDemoModeOOBEAlpha,
		Desc: "Has proceeded through Demo Mode setup flow from OOBE",
		Contacts: []string{
			"cros-demo-mode-eng@google.com",
			"jacksontadie@google.com",
		},
		Impl: &fixtureImpl{
			// This user has infinite idle time-out value for demo mode, thus will not end demo mode session in middle of test.
			enrollmentUser: "admin-tast",
			dmServerURL:    constants.DMServerAlphaURL,
		},
		SetUpTimeout:    setUpTimeout,
		TearDownTimeout: tearDownTimeout,
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey"},
		ServiceDeps: []string{
			"tast.cros.demomode.DemoModeService",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.ui.ChromeUIService",
			"tast.cros.browser.ChromeService"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: fixture.PostDemoModeOOBEProd,
		Desc: "Has proceeded through Demo Mode setup flow from OOBE with Cloud Gaming customizations configured",
		Contacts: []string{
			"jacksontadie@google.com",
			"cros-demo-mode-eng@google.com",
		},
		Impl: &fixtureImpl{
			// This user has infinite idle time-out value for demo mode, thus will not end demo mode session in middle of test.
			enrollmentUser: "admin-tast",
			dmServerURL:    constants.DMServerProdURL,
		},
		SetUpTimeout:    setUpTimeout,
		TearDownTimeout: tearDownTimeout,
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey"},
		ServiceDeps: []string{
			"tast.cros.demomode.DemoModeService",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.ui.ChromeUIService",
			"tast.cros.browser.ChromeService"},
	})
	testing.AddFixture(&testing.Fixture{
		Name: fixture.PostDemoModeOOBECloudGaming,
		Desc: "Has proceeded through Demo Mode setup flow from OOBE with Cloud Gaming customizations configured",
		Contacts: []string{
			"jacksontadie@google.com",
			"cros-demo-mode-eng@google.com",
		},
		Impl: &fixtureImpl{
			// This user has infinite idle time-out value for demo mode, thus will not end demo mode session in middle of test.
			enrollmentUser:  "admin-tast",
			enabledFeatures: []string{"CloudGamingDevice"},
			dmServerURL:     constants.DMServerAlphaURL,
		},
		SetUpTimeout:    setUpTimeout,
		TearDownTimeout: tearDownTimeout,
		Vars:            []string{"ui.signinProfileTestExtensionManifestKey"},
		ServiceDeps: []string{
			"tast.cros.demomode.DemoModeService",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.ui.ChromeUIService",
			"tast.cros.browser.ChromeService"},
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
	if err := policyutil.EnsureTPMAndSystemStateAreResetLocal(ctx, s.DUT(), s.RPCHint()); err != nil {
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
	if err := policyutil.EnsureTPMAndSystemStateAreResetLocal(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM after tests: ", err)
	}
}
