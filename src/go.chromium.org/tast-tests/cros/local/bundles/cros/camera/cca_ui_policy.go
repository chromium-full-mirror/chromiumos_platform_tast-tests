// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIPolicy,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies if CCA is unusable when the camera app is disabled by the Adenterprise policy",
		Contacts:     []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Fixture:      "chromePolicyLoggedIn",
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.SystemFeaturesDisableList{Val: []string{"camera"}}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.VideoCaptureAllowed{Val: false}, pci.VerifiedFunctionalityJS),
		},
	})
}

// CCAUIPolicy verifies CCA is unusable when enterprise policy disables it.
func CCAUIPolicy(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	outDir := s.OutDir()

	subTestTimeout := 30 * time.Second
	for _, tst := range []struct {
		name     string
		testFunc func(context.Context, *chrome.Chrome, string) error
		policy   []policy.Policy
	}{{
		"testNoPolicy",
		testNoPolicy,
		[]policy.Policy{},
	}, {
		"testBlockCameraFeature",
		testBlockCameraFeature,
		[]policy.Policy{&policy.SystemFeaturesDisableList{Val: []string{"camera"}}},
	}, {
		"testBlockVideoCapture",
		testBlockVideoCapture,
		[]policy.Policy{&policy.VideoCaptureAllowed{Val: false}},
	}} {
		subTestCtx, cancel := context.WithTimeout(ctx, subTestTimeout)
		s.Run(subTestCtx, tst.name, func(ctx context.Context, s *testing.State) {
			if err := cca.ClearSavedDir(ctx, cr); err != nil {
				s.Fatal("Failed to clear saved directory: ", err)
			}

			if err := servePolicy(ctx, fdms, cr, tst.policy); err != nil {
				s.Fatal("Failed to serve policy: ", err)
			}

			if err := tst.testFunc(ctx, cr, outDir); err != nil {
				s.Fatalf("Failed to run subtest %v: %v", tst.name, err)
			}
		})
		cancel()
	}
}

func servePolicy(ctx context.Context, fdms *fakedms.FakeDMS, cr *chrome.Chrome, ps []policy.Policy) (retErr error) {
	if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
		return errors.Wrap(err, "failed to reset Chrome")
	}

	if err := policyutil.ServeAndVerify(ctx, fdms, cr, ps); err != nil {
		return errors.Wrap(err, "failed to serve policy")
	}
	return nil
}

// testNoPolicy tests without any policy and expects CCA works fine.
func testNoPolicy(ctx context.Context, cr *chrome.Chrome, outDir string) error {
	tb, err := testutil.NewTestBridge(ctx, cr, testutil.UseFakeHALCamera)
	if err != nil {
		return errors.Wrap(err, "failed to construct test bridge")
	}
	defer tb.TearDown(ctx)

	app, err := cca.New(ctx, cr, outDir, tb)
	if err != nil {
		return errors.Wrap(err, "failed to start CCA with no policy")
	}
	return app.Close(ctx)
}

// testBlockCameraFeature tries to block camera feature and expects a message
// box "Camera is blocked" will show when launching CCA through the launcher.
func testBlockCameraFeature(ctx context.Context, cr *chrome.Chrome, outDir string) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get test extension connection")
	}
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to find keyboard")
	}
	defer kb.Close(ctx)
	if err := launcher.SearchAndLaunch(tconn, kb, apps.Camera.Name)(ctx); err != nil {
		return errors.Wrap(err, "failed to find camera app in the launcher")
	}

	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)
	blockedWindowFinder := nodewith.Role(role.Window).Name("Camera is blocked")

	if err = ui.WaitUntilExists(blockedWindowFinder)(ctx); err != nil {
		return errors.Wrap(err, "failed to check and close blocked window")
	}

	if err = kb.Accel(ctx, "Enter"); err != nil {
		return errors.Wrap(err, "failed to press Enter to close camera warning dialog")
	}

	if err = ui.WaitUntilGone(blockedWindowFinder)(ctx); err != nil {
		return errors.Wrap(err, "failed to close camera warning dialog. This might potentially effect later tests")
	}

	return nil
}

// testBlockVideoCapture tries to block video capture and expects CCA fails to
// initialize since the preview won't show.
func testBlockVideoCapture(ctx context.Context, cr *chrome.Chrome, outDir string) error {
	tb, err := testutil.NewTestBridge(ctx, cr, testutil.UseFakeHALCamera)
	if err != nil {
		return errors.Wrap(err, "failed to construct test bridge")
	}
	defer tb.TearDown(ctx)

	app, err := cca.New(ctx, cr, outDir, tb)

	if err == nil {
		if err := app.WaitForVisibleState(ctx, cca.WarningMessage, true); err != nil {
			return errors.Wrap(err, "failed to detect warning message")
		}
		var errJS *cca.ErrJS
		if err := app.Close(ctx); err != nil && !errors.As(err, &errJS) {
			// It is acceptable that there are errors in CCA since the video
			// capture is blocked. Reports if the error is not JS error.
			testing.ContextLog(ctx, "Failed to close app: ", err)
		}
		return errors.New("failed to block video capture by policy")
	} else if !strings.Contains(err.Error(), cca.ErrVideoNotActive) {
		return errors.Wrap(err, "unexpected error when blocking video capture")
	}
	return nil
}
