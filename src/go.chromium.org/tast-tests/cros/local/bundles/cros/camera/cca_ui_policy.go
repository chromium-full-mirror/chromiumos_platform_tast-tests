// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIPolicy,
		Desc:         "Verifies if CCA is unusable when the camera app is disabled by the Adenterprise policy",
		Contacts:     []string{"chromeos-camera-app-eng@google.com", "wtlee@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:hw_agnostic", "group:mainline", "informational"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		Data:         []string{"video_capture_allowed.html"},
		Timeout:      4 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.SystemFeaturesDisableList{Val: []string{"camera"}}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.VideoCaptureAllowed{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.VideoCaptureAllowedUrls{}, pci.VerifiedFunctionalityJS),
		},
	})
}

// CCAUIPolicy verifies CCA is unusable when enterprise policy disables it.
func CCAUIPolicy(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	outDir := s.OutDir()

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	testVideoCaptureURL := server.URL + "/video_capture_allowed.html"

	subTestTimeout := 30 * time.Second
	for _, tst := range []struct {
		name     string
		testFunc func(context.Context, *chrome.Chrome, string) error
		policy   []policy.Policy
	}{{
		"testNoPolicy",
		testCameraAppWork,
		[]policy.Policy{},
	}, {
		"testBlockCameraApp",
		testCameraAppBlocked,
		[]policy.Policy{&policy.SystemFeaturesDisableList{Val: []string{"camera"}}},
	}, {
		"testVideoCaptureAllowedUnset",
		testCameraAppWork,
		[]policy.Policy{&policy.VideoCaptureAllowed{Stat: policy.StatusUnset}},
	}, {
		"testVideoCaptureAllowedWithAllowedURL",
		func(ctx context.Context, cr *chrome.Chrome, outDir string) error {
			return testVideoCaptureShowPrompt(ctx, cr, outDir, testVideoCaptureURL, false)
		},
		[]policy.Policy{&policy.VideoCaptureAllowedUrls{Val: []string{testVideoCaptureURL}}},
	}, {
		"testVideoCaptureAllowedWithUnallowedURL",
		func(ctx context.Context, cr *chrome.Chrome, outDir string) error {
			return testVideoCaptureShowPrompt(ctx, cr, outDir, testVideoCaptureURL, true)
		},
		[]policy.Policy{&policy.VideoCaptureAllowedUrls{Val: []string{"https://my_corp_site.com/conference.html"}}},
	}, {
		"testVideoCaptureAllowed",
		testCameraAppWork,
		[]policy.Policy{&policy.VideoCaptureAllowed{Val: true}},
	}, {
		"testVideoCaptureBlocked",
		testPreviewNotActive,
		[]policy.Policy{&policy.VideoCaptureAllowed{Val: false}},
	}, {
		"testVideoCaptureBlockedButAllowCCA",
		testCameraAppWork,
		[]policy.Policy{&policy.VideoCaptureAllowed{Val: false}, &policy.VideoCaptureAllowedUrls{Val: []string{"chrome://camera-app/*"}}},
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

// testCameraAppWork tests whether CCA works normally.
func testCameraAppWork(ctx context.Context, cr *chrome.Chrome, outDir string) error {
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

// testCameraAppBlocked tests whether the camera app is blocked and a message
// box "Camera is blocked" will show when launching CCA through the launcher.
func testCameraAppBlocked(ctx context.Context, cr *chrome.Chrome, outDir string) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get test extension connection")
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, outDir, func() bool {
		return retErr != nil
	}, tconn)

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to find keyboard")
	}
	defer kb.Close(cleanupCtx)

	if err := launcher.SearchAndLaunch(tconn, kb, apps.Camera.Name)(ctx); err != nil {
		return errors.Wrap(err, "failed to find camera app in the launcher")
	}

	ui := uiauto.New(tconn).WithTimeout(10 * time.Second)
	blockedWindowFinder := nodewith.Name("Camera is blocked").First()

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

// testPreviewNotActive tests whether the preview will never become active.
func testPreviewNotActive(ctx context.Context, cr *chrome.Chrome, outDir string) error {
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

// testVideoCaptureShowPrompt tests whether a prompt will show up to request for
// user's permission before the video capture starts.
func testVideoCaptureShowPrompt(ctx context.Context, cr *chrome.Chrome, outDir, testURL string, shouldShowPrompt bool) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Open the test website.
	conn, err := cr.NewConn(ctx, testURL)
	if err != nil {
		return errors.Wrap(err, "failed to open website")
	}
	defer conn.Close()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, outDir, func() bool {
		return retErr != nil
	}, tconn)

	ui := uiauto.New(tconn)

	allowButton := nodewith.Name("Allow").Role(role.Button)
	if shouldShowPrompt {
		if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(allowButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to find the video capture prompt dialog")
		}
	} else {
		if err := ui.EnsureGoneFor(allowButton, 10*time.Second)(ctx); err != nil {
			return errors.Wrap(err, "failed to make sure no video capture prompt dialog shows")
		}
	}
	return nil
}
