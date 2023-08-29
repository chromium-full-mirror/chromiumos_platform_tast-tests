// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package floatingworkspace

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/ash/ashproc"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/procutil"
	"go.chromium.org/tast-tests/cros/local/saveddesks"
	"go.chromium.org/tast-tests/cros/services/cros/floatingworkspace"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var appsList = []apps.App{apps.Chrome, apps.FilesSWA}

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			floatingworkspace.RegisterTemplateSyncServiceServer(srv, &TemplateSyncService{s: s})
		},
	})
}

// TemplateSyncService implements tast.cros.floatingworkspace.TemplateSyncService.
type TemplateSyncService struct {
	s *testing.ServiceState

	cr         *chrome.Chrome
	tconn      *chrome.TestConn
	deviceName string
	username   string
}

// NewChromeLogin logs into Chrome with DeskTemplateSync and FloatingWorkspaceV2 flags enabled.
func (tss *TemplateSyncService) NewChromeLogin(ctx context.Context, req *floatingworkspace.CrOSLoginRequest) (*empty.Empty, error) {
	if tss.cr != nil {
		return nil, errors.New("Chrome already available")
	}
	floatingworkspaceOpts := []chrome.Option{
		chrome.EnableFeatures("DeskTemplateSync", "FloatingWorkspaceV2"),
	}

	tss.username = chrome.DefaultUser
	if req.Username != "" {
		tss.username = req.Username
		floatingworkspaceOpts = append(floatingworkspaceOpts, chrome.GAIALogin(chrome.Creds{User: req.Username, Pass: req.Password}))
	}
	if req.KeepState {
		floatingworkspaceOpts = append(floatingworkspaceOpts, chrome.KeepState())
	}

	testing.ContextLog(ctx, req.EnabledFlags)
	for _, flag := range req.EnabledFlags {
		floatingworkspaceOpts = append(floatingworkspaceOpts, chrome.EnableFeatures(flag))
	}
	testing.ContextLog(ctx, req.DisabledFlags)
	for _, flag := range req.DisabledFlags {
		floatingworkspaceOpts = append(floatingworkspaceOpts, chrome.DisableFeatures(flag))
	}

	cr, err := chrome.New(ctx, floatingworkspaceOpts...)
	if err != nil {
		testing.ContextLog(ctx, "Failed to start Chrome")
		return nil, err
	}
	tss.cr = cr
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get a connection to the Test Extension")
		return nil, err
	}
	tss.tconn = tconn
	return &empty.Empty{}, nil
}

// CloseChrome closes all surfaces and Chrome.
// This will likely be called in a defer in remote tests instead of called explicitly. So log everything that fails to aid debugging later.
func (tss *TemplateSyncService) CloseChrome(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	tconn := tss.tconn
	ac := uiauto.New(tconn)
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		return nil, err
	}
	ash.CleanUpDesks(cleanupCtx, tconn)
	// Delete all existing saved desks.
	if err := saveddesks.DeleteSavedDesks(cleanupCtx, tconn, ac); err != nil {
		return nil, err
	}

	cleanup(cleanupCtx)

	if tss.cr == nil {
		testing.ContextLog(ctx, "Chrome not available")
		return nil, errors.New("Chrome not available")
	}
	err = tss.cr.Close(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to close Chrome in Floating Workspace service: ", err)
	} else {
		testing.ContextLog(ctx, "Floating Workspace service closed successfully for: ", tss.deviceName)
	}
	tss.cr = nil
	return &empty.Empty{}, err
}

// WaitForFloatingWorkspaceCapture captures a desk template and upload it to chrome sync.
func (tss *TemplateSyncService) WaitForFloatingWorkspaceCapture(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	tconn := tss.tconn
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		return nil, err
	}

	defer cleanup(cleanupCtx)

	ac := uiauto.New(tconn)
	// Wait for saved desk sync.
	ash.WaitForSavedDeskSync(ctx, ac)
	// Close all existing windows.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		return nil, err
	}
	// Open Chrome and Files.
	if err := saveddesks.OpenApps(ctx, tconn, ac, appsList); err != nil {
		return nil, err
	}

	// Wait for 35 seconds for floating workspace templates to capture and sync.
	// GoBigSleepLint: There is a periodic job running in the background every 30s
	// to capture a desk for upload. There is no confirmation that the upload has
	// occurred through the UI, so the best chance to make sure we don't miss a
	// upload is to wait 30s + some buffer.
	if err := testing.Sleep(ctx, 35*time.Second); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}

// Signout signs user out of of current session with keyboard shortcut.
func (tss *TemplateSyncService) Signout(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	oldProc, err := ashproc.Root()
	if err != nil {
		return nil, err
	}

	// Sign out with keyboard shortcut.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, err
	}
	// Press the signout shortcut twice, one for bringing up the signout
	// confirmation, second for confirming the signout operation.
	if err := kb.Accel(ctx, "Ctrl+Shift+Q"); err != nil {
		return nil, err
	}
	if err := kb.Accel(ctx, "Ctrl+Shift+Q"); err != nil {
		return nil, err
	}
	// Wait for Chrome restart
	if err := procutil.WaitForTerminated(ctx, oldProc, 30*time.Second); err != nil {
		return nil, err
	}
	if _, err := ashproc.WaitForRoot(ctx, 30*time.Second); err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// SignBackIn signs user back in after user signs out from previous session.
func (tss *TemplateSyncService) SignBackIn(ctx context.Context, req *floatingworkspace.CrOSLoginRequest) (*empty.Empty, error) {
	creds := tss.cr.Creds()
	floatingworkspaceOpts := []chrome.Option{
		chrome.EnableFeatures("DeskTemplateSync", "FloatingWorkspaceV2"),
		chrome.KeepState(), chrome.LoadSigninProfileExtension(req.SignInOption), chrome.NoLogin(),
	}
	cr, err := chrome.New(ctx, floatingworkspaceOpts...)
	if err != nil {
		testing.ContextLog(ctx, "Failed to start Chrome")
		return nil, err
	}
	tss.cr = cr
	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get a connection to the Test Extension")
		return nil, err
	}
	tss.tconn = tconn
	// Wait for the login screen to be ready for password entry.
	if _, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.ReadyForPassword }, 30*time.Second); err != nil {
		return nil, err
	}
	if err := lockscreen.WaitForPasswordField(ctx, tconn, creds.User, 5*time.Second); err != nil {
		return nil, err
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, err
	}
	defer kb.Close(ctx)

	if err := kb.Type(ctx, creds.Pass+"\n"); err != nil {
		return nil, err
	}

	// Check if the login was successful using the API and also by looking for the shelf in the UI.
	if err := lockscreen.WaitForLoggedIn(ctx, tconn, chrome.LoginTimeout); err != nil {
		return nil, err
	}

	if err := ash.WaitForShelf(ctx, tconn, 30*time.Second); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}

// VerifyFloatingWorkspace verifies a floating workspace template is synced to another device.
func (tss *TemplateSyncService) VerifyFloatingWorkspace(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	tconn := tss.tconn
	ac := uiauto.New(tconn)

	defer ash.CleanUpDesks(cleanupCtx, tconn)

	// Wait for apps to launch.
	if err := saveddesks.WaitforAppsToLaunch(ctx, tconn, ac, appsList); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}
