// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package kioskmode provides ways to set policies for local device accounts
// in a Kiosk mode.
package kioskmode

import (
	"context"
	"encoding/hex"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash/ashproc"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/local/policyutil/fixtures"
	"chromiumos/tast/local/procutil"
	"chromiumos/tast/local/syslog"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

var (
	// KioskAppAccountID identifier of the Kiosk application.
	KioskAppAccountID   = "arbitrary_id_store_app_2@managedchrome.com"
	kioskAppAccountType = policy.AccountTypeKioskApp
	// KioskAppID pointing to the Printtest app - not listed in the WebStore.
	KioskAppID = "aajgmlihcokkalfjbangebcffdoanjfo"
	// KioskAppBtnName is the name of the Printest app which shows up in the Apps
	// menu on the sign-in screen.
	KioskAppBtnName = "Simple Printest"
	// KioskAppAccountInfo can be included in DeviceLocalAccounts to enable KioskApp
	KioskAppAccountInfo = policy.DeviceLocalAccountInfo{
		AccountID:   &KioskAppAccountID,
		AccountType: &kioskAppAccountType,
		KioskAppInfo: &policy.KioskAppInfo{
			AppId: &KioskAppID,
		}}
	cancelLaunchText = nodewith.Name("Press Ctrl + Alt + S to switch to ChromeOS").Role("staticText")
)

const (
	// kioskStartingLog is reported by Chrome once Kiosk is starting.
	kioskStartingLog = "Starting kiosk mode"
	// kioskStartingDuration is the time estimate to emit a kioskStartingLog after Kiosk launch has
	// started (either manual or auto launch).
	kioskStartingDuration = 30 * time.Second
	// kioskReadyToLaunchLog is reported by Chrome once the Kiosk app is ready to launch.
	kioskReadyToLaunchLog = "Kiosk app is ready to launch."
	// kioskReadyToLaunchDuration is the time estimate to emit a kioskReadyToLaunchLog after
	// kioskStartingLog was emitted.
	kioskReadyToLaunchDuration = 90 * time.Second
	// kioskLaunchSucceededLog is reported by Chrome once Kiosk launched successfully.
	kioskLaunchSucceededLog = "Kiosk launch succeeded"
	// kioskLaunchSucceededDuration is the time estimate to emit a kioskLaunchSucceededLog after
	// kioskReadyToLaunchLog was emitted.
	kioskLaunchSucceededDuration = 60 * time.Second
	// kioskClosingSplashScreenLog is reported by Chrome once the splash screen is closing.
	kioskClosingSplashScreenLog = "App window created, closing splash screen."

	// setPolicyDuration is the time estimate to set policies in Kiosk with setPolicies or
	// setPolicyBlob.
	setPolicyDuration = 60 * time.Second
	// SetupDuration is the time estimate to set up a Kiosk session with kioskmode.New. This does not
	// include time to launch the session.
	SetupDuration = setPolicyDuration
	// LaunchDuration is the time estimate to launch a Kiosk session.
	LaunchDuration = kioskStartingDuration + kioskReadyToLaunchDuration + kioskLaunchSucceededDuration
	// CleanupDuration is the time estimate to clean up a Kiosk session with kiosk.Close.
	CleanupDuration = setPolicyDuration
)

// Kiosk structure holds necessary references and provides a way to safely
// close Kiosk mode.
type Kiosk struct {
	ctx           context.Context
	cr            *chrome.Chrome
	fdms          *fakedms.FakeDMS
	localAccounts *policy.DeviceLocalAccounts
	httpServer    *httptest.Server
	// TODO(b/280555587) remove this field when kiosk.DeprecatedClose is removed.
	autostart bool
}

// Close cleans up resources used by the Kiosk struct and resets policies to an empty slice.
//
// Calls to kioskmode.New should be paired with a call to Close. This is important as Close clears
// Kiosk policies before the next tests.
//
// The error returned from Close must be checked, and tests should fail if Close returns an error.
//
// Tests should reserve time for Close using CleanupDuration, for example:
//
//	// Store initial context in cleanupCtx and shorten ctx.
//	cleanupCtx := ctx
//	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
//	defer cancel()
//
//	// From now on use ctx in the rest of the test as usual.
//	kiosk, cr, err := kioskmode.New(ctx, ...)
//	if err != nil { ... }
//
//	// Pass in cleanupCtx when calling kiosk.Close.
//	defer func(ctx context.Context) {
//		// Fail the test if kiosk.Close fails.
//		if err := kiosk.Close(ctx); err != nil {
//			s.Error("Failed to close kiosk: ", err)
//		}
//	}(cleanupCtx)
func (k *Kiosk) Close(ctx context.Context, signinTestExtensionManifestKey string) (retErr error) {
	if ctxutil.DeadlineBefore(ctx, time.Now().Add(CleanupDuration)) {
		testing.ContextLog(ctx, "Deadline too short for kiosk.Close, did you reserve cleanupCtx?")
		retErr = errors.New("potentially insufficient time remaining for kiosk.Close")
	}

	if k.httpServer != nil {
		k.httpServer.Close()
	}

	// Proceed with cleanup if Chrome is already closed or fails to close, because clearPolicies will
	// start a new Chrome instance.
	if k.cr != nil {
		if err := k.cr.Close(ctx); err != nil {
			if retErr == nil {
				retErr = errors.Wrap(err, "failed to close chrome")
			} else {
				testing.ContextLog(ctx, "Failed to close chrome: ", err)
			}
		}
	}

	if err := clearPolicies(ctx, k.fdms, signinTestExtensionManifestKey); err != nil {
		if retErr == nil {
			retErr = errors.Wrap(err, "failed to clean up Kiosk policies, this may impact next test")
		} else {
			testing.ContextLog(ctx, "Failed to clean up Kiosk policies, this may impact next test: ", err)
		}
	}
	return retErr
}

// clearPolicies sets policies to an empty policy slice.
func clearPolicies(ctx context.Context, fdms *fakedms.FakeDMS, signinTestExtensionManifestKey string) error {
	return setPolicies(ctx, fdms, signinTestExtensionManifestKey, []policy.Policy{})
}

// DeprecatedClose clears policies, but keeps serving device local accounts
// then closes Chrome. Ideally we would serve an empty policies slice however,
// that makes Chrome crashes when AutoLaunch() option was used.
//
// Deprecated: Prefer using Close, as it clears Kiosk policies correctly between
// tests.
func (k *Kiosk) DeprecatedClose(ctx context.Context) (retErr error) {
	// If Chrome fails to start in RestartChromeWithOptions it has already been
	// cleaned up by startChromeClearPolicies.
	if k.cr == nil {
		return errors.New("Skipping kiosk.Close() because Chrome is nil")
	}

	// Using defer to make sure Chrome is always closed.
	defer func(ctx context.Context) {
		if err := k.cr.Close(ctx); err != nil {
			// Chrome error supersedes previous error if any.
			retErr = errors.Wrap(err, "could not close Chrome while closing Kiosk session")
		}
	}(ctx)

	if k.httpServer != nil {
		k.httpServer.Close()
	}

	var policies []policy.Policy
	// When AutoLaunch() option was used, then the corresponding policy has to
	// be removed before starting a new Chrome session. Otherwise Kiosk will
	// start again. When applying an empty policies slice, Chrome crashes.
	// Hence the safest way is to apply local accounts again. That way Chrome
	// will load them but will start normally. If the next tests want to use
	// policy they will override them.
	if k.autostart {
		policies = append(policies, k.localAccounts)
	}

	var serveAndRefreshErr error

	defer func(ctx context.Context) {
		if serveAndRefreshErr == nil {
			return
		}

		// If `policyutil.ServeAndRefresh` is failed, we might be on the login screen
		// if test interrupted kiosk autolaunch or manual kiosk launch failed.
		//
		// So we try to refresh policies from login screen using signin profile test extension.
		//
		// Test has to use next kiosk option to load signin profile test extension:
		//
		//   kioskmode.ExtraChromeOptions(
		//	   chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")))

		// TODO(b/278071203): Figure out more robust way to cleanup autolaunch kiosk.

		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		if err := policyutil.ServeAndRefreshOnLoginScreen(ctx, k.fdms, k.cr, policies); err != nil {
			testing.ContextLog(ctx, "Could not serve and refresh policies on login screen. If kioskmode.AutoLaunch() option was used it may impact next test : ", err)
			retErr = serveAndRefreshErr
		}
	}(ctx)

	defer func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		if err := policyutil.ServeAndRefresh(ctx, k.fdms, k.cr, policies); err != nil {
			testing.ContextLog(ctx, "Could not serve and refresh policies. If kioskmode.AutoLaunch() option was used it may impact next test : ", err)
			serveAndRefreshErr = errors.Wrap(err, "could not clear policies")
		}
	}(ctx)

	return nil
}

// WaitLaunchLogs uses reader to look for logs that confirm Kiosk mode launched successfully.
//
// reader is expected to process syslogs filtered for Chrome and to include messages since before
// the session was launched. As in:
//
//	reader, err := syslog.NewReader(ctx, syslog.Program("chrome"))
//	...
//	kiosk, cr, err := kioskmode.New(ctx, ...)  // Reader was created before Kiosk launches.
//	...
//	err := kioskmode.WaitLaunchLogs(ctx, reader)
//
// This is necessary because syslog.NewReader only contains logs from the moment it was created.
//
// Tests using WaitLaunchLogs should have a long enough Timeout to account for
// kioskmode.LaunchDuration.
func WaitLaunchLogs(ctx context.Context, reader *syslog.Reader) error {
	if ctxutil.DeadlineBefore(ctx, time.Now().Add(LaunchDuration)) {
		return errors.New("potentially insufficient time remaining to wait for Kiosk launch")
	}

	if err := waitLog(ctx, reader, kioskStartingLog, kioskStartingDuration); err != nil {
		return errors.Wrap(err, "failed to verify Kiosk is starting")
	}

	if err := waitLog(ctx, reader, kioskReadyToLaunchLog, kioskReadyToLaunchDuration); err != nil {
		return errors.Wrap(err, "failed to verify Kiosk is ready to launch")
	}

	if err := waitLog(ctx, reader, kioskLaunchSucceededLog, kioskLaunchSucceededDuration); err != nil {
		return errors.Wrap(err, "failed to verify Kiosk launch succeeded")
	}

	return nil
}

// waitLog waits for the Chrome syslog reader to emit the given Kiosk message.
func waitLog(ctx context.Context, reader *syslog.Reader, message string, timeout time.Duration) error {
	testing.ContextLogf(ctx, "Kiosk mode: waiting log message %q", message)
	containsMessage := func(e *syslog.Entry) bool { return strings.Contains(e.Content, message) }
	if _, err := reader.Wait(ctx, timeout, containsMessage); err != nil {
		return errors.Wrapf(err, "could not find log message %q", message)
	}
	return nil
}

// IsKioskAppStarted searches for existing logs to confirm Kiosk is running.
func IsKioskAppStarted(ctx context.Context) error {
	logContent, err := ioutil.ReadFile(syslog.ChromeLogFile)
	if err != nil {
		return errors.Wrap(err, "failed to read "+syslog.ChromeLogFile)
	}

	if !strings.Contains(string(logContent), kioskClosingSplashScreenLog) {
		return errors.New("failed to verify successful launch of Kiosk mode")
	}
	return nil
}

// DeprecatedNew starts Chrome, sets passed Kiosk related options to policies
// and restarts Chrome. When kioskmode.AutoLaunch() is used, then it auto starts
// given Kiosk application. Alternatively use kioskmode.ExtraChromeOptions()
// passing chrome.LoadSigninProfileExtension(). In that case Chrome is started
// and stays on Signin screen with Kiosk accounts loaded.
// Use defer kiosk.Close(ctx) to clean.
//
// Deprecated: Prefer using New, as it sets Kiosk policies in Chrome using the
// safer --prevent-kiosk-autolaunch-for-testing flag.
func DeprecatedNew(ctx context.Context, fdms *fakedms.FakeDMS, opts ...Option) (k *Kiosk, c *chrome.Chrome, e error) {
	cfg, err := NewConfig(opts)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to process options")
	}

	var deviceLocalAccounts *policy.DeviceLocalAccounts
	var httpServer *httptest.Server

	if cfg.m.UseDefaultLocalAccounts {
		if cfg.m.DeviceLocalAccounts != nil {
			return nil, nil, errors.New("invalid config: DeviceLocalAccounts and UseDefaultLocalAccounts should not be used at the same time")
		}

		httpServer = NewWebKioskAppServer(ctx)
		webKioskAppAccountInfo := WebKioskAppAccountInfo(httpServer.URL, WebKioskAccountID)
		deviceLocalAccounts = &policy.DeviceLocalAccounts{
			Val: []policy.DeviceLocalAccountInfo{KioskAppAccountInfo, webKioskAppAccountInfo}}

		// Close local http server if Kiosk fails to start.
		defer func() {
			if httpServer != nil && k == nil {
				httpServer.Close()
			}
		}()
	} else if cfg.m.DeviceLocalAccounts != nil {
		deviceLocalAccounts = cfg.m.DeviceLocalAccounts
	} else {
		return nil, nil, errors.Wrap(err, "local device accounts were not set")
	}

	err = func(ctx context.Context) error {
		testing.ContextLog(ctx, "Kiosk mode: Starting Chrome to set Kiosk policies")
		cr, err := chrome.New(
			ctx,
			chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}), // Required as refreshing policies require test API.
			chrome.DMSPolicy(fdms.URL),
			chrome.KeepEnrollment(),
		)
		if err != nil {
			return errors.Wrap(err, "failed to start Chrome")
		}

		// Close the previous Chrome instance.
		defer cr.Close(ctx)

		// Set local accounts policy.
		policies := []policy.Policy{
			deviceLocalAccounts,
		}

		// Handle the AutoLaunch setup.
		if cfg.m.AutoLaunch == true {
			policies = append(policies, &policy.DeviceLocalAccountAutoLoginId{
				Val: *cfg.m.AutoLaunchKioskAppID,
			})
		}

		// Handle setting device policies.
		if cfg.m.ExtraPolicies != nil {
			policies = append(policies, cfg.m.ExtraPolicies...)
		}

		pb := policy.NewBlob()
		pb.AddPolicies(policies)
		// Handle public account policies.
		if cfg.m.PublicAccountPolicies != nil {
			for accountID, policies := range cfg.m.PublicAccountPolicies {
				pb.AddPublicAccountPolicies(accountID, policies)
			}
		}
		// Handle custom directory api id.
		if cfg.m.CustomDirectoryAPIID != nil {
			pb.DirectoryAPIID = *cfg.m.CustomDirectoryAPIID
		}
		// Update policies.
		if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
			// In case of AutoLaunch was used we try to override policies with
			// local accounts similarly as in kioskmode.Close().
			if cfg.m.AutoLaunch == true {
				if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{deviceLocalAccounts}); err != nil {
					testing.ContextLog(ctx, "Could not serve and refresh policies. If kioskmode.AutoLaunch() option was used it may impact next test : ", err)
				}
			}
			return errors.Wrap(err, "failed to serve and refresh policies")
		}

		return nil
	}(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed preparing Chrome to start with given Kiosk configuration")
	}

	reader, err := syslog.NewReader(ctx, syslog.Program("chrome"))
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to start log reader")
	}
	defer reader.Close()

	var cr *chrome.Chrome
	if cfg.m.AutoLaunch {
		opts := []chrome.Option{
			chrome.NoLogin(),
			chrome.DMSPolicy(fdms.URL),
			chrome.KeepEnrollment(),
		}
		opts = append(opts, cfg.m.ExtraChromeOptions...)

		testing.ContextLog(ctx, "Kiosk mode: Starting Chrome in Kiosk mode")
		// Restart Chrome. After that Kiosk auto starts.
		cr, err = chrome.New(ctx, opts...)
		if err != nil {
			if err := startChromeClearPolicies(ctx, fdms, fixtures.Username, fixtures.Password); err != nil {
				return nil, nil, errors.Wrap(err, "could not finish cleanup")
			}
			return nil, nil, errors.Wrap(err, "Chrome restart failed")
		}

		if !cfg.m.SkipSuccessfulLaunchCheck {
			// Library waits for Kiosk start sequence to start then it checks
			// that Kiosk is ready for launch, and finally it waits for Kiosk
			// to be launched.
			if err := WaitLaunchLogs(ctx, reader); err != nil {
				if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{deviceLocalAccounts}); err != nil {
					testing.ContextLog(ctx, "Could not serve and refresh policies. If kioskmode.AutoLaunch() option was used it may impact next test: ", err)
				}
				cr.Close(ctx)
				return nil, nil, errors.Wrap(err, "there was a problem while checking chrome logs for Kiosk related entries")
			}
		}
	} else {
		opts := []chrome.Option{
			chrome.DeferLogin(),
			chrome.DMSPolicy(fdms.URL),
			chrome.KeepEnrollment(),
		}
		opts = append(opts, cfg.m.ExtraChromeOptions...)

		testing.ContextLog(ctx, "Kiosk mode: Starting Chrome on Signin screen with set Kiosk apps")
		// Restart Chrome. Chrome stays on Sing-in screen
		cr, err = chrome.New(ctx, opts...)
		if err != nil {
			return nil, nil, errors.Wrap(err, "Chrome restart failed")
		}
	}

	return &Kiosk{ctx: ctx, cr: cr, fdms: fdms, localAccounts: deviceLocalAccounts, httpServer: httpServer, autostart: cfg.m.AutoLaunch}, cr, nil
}

// startChromeClearPolicies is called when Chrome fails to start in autostart
// mode - when kioskmode.AutoLaunch() option was used. We need to start Chrome
// and clean policies to prevent Chrome starting automatically in Kiosk mode
// for next test.
// FIXME: this cleanup doesn't work on some devices (e.g. chell). FakeLogin()
// doesn't work either. Need to figure out some way to fix this.
func startChromeClearPolicies(ctx context.Context, fdms *fakedms.FakeDMS, username, password string) error {
	cr, err := chrome.New(
		ctx,
		chrome.NoLogin(),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment(),
	)
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome for cleanup")
	}
	defer cr.Close(ctx)

	if err := policyutil.ServeAndRefresh(ctx, fdms, cr, []policy.Policy{}); err != nil {
		return errors.Wrap(err, "failed to clear policies")
	}
	return nil
}

// WaitForCrxInCache waits for Kiosk crx to be available in cache.
func WaitForCrxInCache(ctx context.Context, id string) error {
	const crxCachePath = "/home/chronos/kiosk/crx/"
	ctx, st := timing.Start(ctx, "wait_crx_cache")
	defer st.End()

	return testing.Poll(ctx, func(ctx context.Context) error {
		files, err := ioutil.ReadDir(crxCachePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return errors.Wrap(err, "Kiosk crx cache does not exist yet")
			}
			return testing.PollBreak(errors.Wrap(err, "failed to list content of Kiosk cache"))
		}

		for _, file := range files {
			if strings.HasPrefix(file.Name(), id) {
				testing.ContextLog(ctx, "Found crx in cache: "+file.Name())
				return nil
			}
		}

		return errors.Wrap(err, "Kiosk crx cache does not have "+id)
	}, nil)
}

// restartChromeNoCloseWithOptions replaces the current Chrome in kiosk instance
// with a new one using custom options without closing the old one. It will be
// closed by Kiosk.Close(). Useful when Chrome already closes itself, for
// example when cancelling a Kiosk launch.
func (k *Kiosk) restartChromeNoCloseWithOptions(ctx context.Context, opts ...chrome.Option) (*chrome.Chrome, error) {
	k.cr = nil

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		if err := startChromeClearPolicies(ctx, k.fdms, fixtures.Username, fixtures.Password); err != nil {
			return nil, errors.Wrap(err, "could not finish cleanup")
		}
		return nil, errors.Wrap(err, "failed to start new Chrome")
	}
	k.cr = cr
	return cr, err
}

// RestartChromeWithOptions replaces the current Chrome in kiosk instance with
// a new one using custom options. It will be closed by Kiosk.Close().
func (k *Kiosk) RestartChromeWithOptions(ctx context.Context, opts ...chrome.Option) (*chrome.Chrome, error) {
	if err := k.cr.Close(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to close Chrome")
	}
	return k.restartChromeNoCloseWithOptions(ctx, opts...)
}

// StartFromSignInScreen starts a Kiosk app from the Apps menu on the sign-in
// screen, simulating a manual launch. It doesn't wait for a successful launch
// so that the launch can be cancelled by pressing Ctrl+Alt+S.
// TODO(b/230840565): Extract and extend this function to support MGS.
func StartFromSignInScreen(ctx context.Context, ui *uiauto.Context, name string) error {
	testing.ContextLog(ctx, "Starting Kiosk app from sign-in screen: "+name)
	localAccountsBtn := nodewith.Name("Apps").HasClass("MenuButton")
	kioskAppBtn := nodewith.Name(name).HasClass("MenuItemView")
	if err := uiauto.Combine("launch Kiosk app from menu",
		ui.WaitUntilExists(localAccountsBtn),
		ui.LeftClick(localAccountsBtn),
		ui.WaitUntilExists(kioskAppBtn),
		ui.LeftClick(kioskAppBtn),
		ui.WaitUntilExists(cancelLaunchText),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to start Kiosk application from apps menu")
	}
	return nil
}

// CancelKioskLaunch cancels the current Kiosk launch by pressing Ctrl+Alt+S.
// Must be invoked on the Kiosk splash screen. A new Chrome instance will be
// started with given options. It verifies a successful cancel by checking for
// cancelled message on the screen.
func (k *Kiosk) CancelKioskLaunch(ctx context.Context, opts ...chrome.Option) (*chrome.Chrome, error) {
	const disableChromeRestartFile = "/run/disable_chrome_restart"

	testing.ContextLog(ctx, "Cancelling Kiosk launch via Ctrl+Alt+S")
	kw, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a keyboard")
	}
	defer kw.Close(ctx)

	if err := chrome.PrepareForRestart(); err != nil {
		return nil, errors.Wrap(err, "failed to remove old dev tools port file")
	}

	// Create the flag file to make sure session_manager does not start Chrome
	// again after Chrome exits.
	_, err = os.Create(disableChromeRestartFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Chrome flag file")
	}
	defer func(ctx context.Context) {
		if err := os.RemoveAll(disableChromeRestartFile); err != nil && !os.IsNotExist(err) {
			testing.ContextLog(ctx, "Failed to remove flag file: ", err)
		}
	}(ctx)

	// Find the current Chrome process to wait for it to shut down later.
	old, err := ashproc.WaitForRoot(ctx, time.Minute)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find the browser process")
	}

	if err := kw.Accel(ctx, "Ctrl+Alt+S"); err != nil {
		return nil, errors.Wrap(err, "failed to hit Ctrl+Alt+S and attempt to quit a kiosk app")
	}

	// Wait for the current Chrome to shut down.
	if err := procutil.WaitForTerminated(ctx, old, 10*time.Second); err != nil {
		return nil, errors.Wrap(err, "browser process didn't terminate")
	}

	// Remove flag file so that session_manager will start Chrome after UI task is
	// restarted.
	if err := os.RemoveAll(disableChromeRestartFile); err != nil {
		return nil, errors.Wrap(err, "failed to remove flag file")
	}

	// Restart Chrome without closing since the current Chrome process has already
	// exited itself.
	cr, err := k.restartChromeNoCloseWithOptions(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to restart Chrome")
	}
	return cr, nil
}

// WaitForSplashScreenShowing waits for the kiosk splash screen to show up as
// identified by the cancelation message
func (k *Kiosk) WaitForSplashScreenShowing() error {
	testConn, err := k.cr.SigninProfileTestAPIConn(k.ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to signin extension")
	}

	ui := uiauto.New(testConn)
	if err := ui.WaitUntilExists(cancelLaunchText)(k.ctx); err != nil {
		return errors.Wrap(err, "failed to find splash screen")
	}
	return nil
}

// GetLocalAccounts fetches DeviceLocalAccounts policy
func (k *Kiosk) GetLocalAccounts() *policy.DeviceLocalAccounts {
	return k.localAccounts
}

// DeviceLocalAccountUserID calculates the user_id of a DeviceLocalAccount.
// This code is replicated in several places; for example:
// - chrome/browser/ash/policy/core/device_local_account.cc (GenerateDeviceLocalAccountUserId)
// - src/platform2/libbrillo/policy/device_local_account_policy_util.cc
func DeviceLocalAccountUserID(account *policy.DeviceLocalAccountInfo) string {
	user, prefix := "", ""
	if account.AccountID != nil {
		user = hex.EncodeToString([]byte(*account.AccountID))
	}
	if account.AccountType != nil {
		switch *account.AccountType {
		case policy.AccountTypePublicSession:
			prefix = "public-accounts"
		case policy.AccountTypeKioskApp:
			prefix = "kiosk-apps"
		case policy.AccountTypeKioskAndroidApp:
			prefix = "arc-kiosk-apps"
		case policy.AccountTypeSAMLPublicSession:
			prefix = "saml-public-accounts"
		case policy.AccountTypeWebKioskApp:
			prefix = "web-kiosk-apps"
		}
	}
	return user + "@" + prefix + ".device-local.localhost"
}

// setPolicyBlob starts a new Chrome instance to set the given policy blob.
//
// Note that once this function returns Chrome will be closed and the device will be in the login
// screen.
//
// Callers are expected to start a new Chrome themselves after this function. This is required even
// if Kiosk auto launch policies were configured, as Kiosk cannot auto launch if no Chrome instance
// is running.
func setPolicyBlob(ctx context.Context, fdms *fakedms.FakeDMS, signinTestExtensionManifestKey string, pb *policy.Blob) (retErr error) {
	testing.ContextLog(ctx, "Kiosk mode: go to login screen to set policies")
	cr, err := chrome.New(
		ctx,
		chrome.NoLogin(),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment(),
		chrome.LoadSigninProfileExtension(signinTestExtensionManifestKey),
		// Use the test-only command line switch to prevent Kiosk auto launch in case the current test
		// configured it in policies.
		//
		// This is important because Chrome decides to auto launch Kiosk very early at startup. Without
		// this flag Chrome would respect the previous policies and auto launch before the new policy
		// blob applies.
		chrome.ExtraArgs("--prevent-kiosk-autolaunch-for-testing"),
	)
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome to stay on the login screen and prevent Kiosk autolaunch")
	}
	defer func(ctx context.Context) {
		if err := cr.Close(ctx); err != nil {
			if retErr != nil {
				testing.ContextLog(ctx, "Failed to close Chrome started to set policy blob: ", err)
			} else {
				retErr = errors.Wrap(err, "failed to close Chrome ")
			}
		}
	}(ctx)

	if err := policyutil.ServeBlobAndRefreshOnLoginScreen(ctx, fdms, cr, pb); err != nil {
		return errors.Wrap(err, "could not serve and verify empty policies on login screen")

	}
	return nil
}

// setPolicies is the same as setPolicyBlob but takes a []policy.Policy slice instead.
func setPolicies(ctx context.Context, fdms *fakedms.FakeDMS, signinTestExtensionManifestKey string, policies []policy.Policy) error {
	blob := policy.NewBlob()
	if err := blob.AddPolicies(policies); err != nil {
		return errors.Wrap(err, "failed to add policies to policy blob")
	}
	return setPolicyBlob(ctx, fdms, signinTestExtensionManifestKey, blob)
}
