// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package kioskmode provides ways to set policies for local device accounts
// in a Kiosk mode.
package kioskmode

import (
	"context"
	"encoding/hex"
	"fmt"
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
	// cancelLaunchText is a text shown in the launch screen with instructions to cancel launch.
	cancelLaunchText = nodewith.Name("Press Ctrl + Alt + S to switch to ChromeOS").Role("staticText")
	// chromeAppWindow is the Kiosk app window in Chrome app deployments.
	chromeAppWindow = nodewith.ClassName("NativeAppWindowViews").Role("window")
	// webAppWindow is the Kiosk app window in web deployments.
	webAppWindow = nodewith.ClassName("BrowserFrame").Role("window")
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
	kioskReadyToLaunchDuration = 3 * time.Minute
	// kioskLaunchSucceededLog is reported by Chrome once Kiosk launched successfully.
	kioskLaunchSucceededLog = "Kiosk launch succeeded"
	// kioskLaunchSucceededDuration is the time estimate to emit a kioskLaunchSucceededLog after
	// kioskReadyToLaunchLog was emitted.
	kioskLaunchSucceededDuration = 60 * time.Second
	// kioskClosingSplashScreenLog is reported by Chrome once the splash screen is closing.
	kioskClosingSplashScreenLog = "App window created, closing splash screen."

	// policyPersistDuration is the time estimate for Chrome to store policies after a refresh.
	policyPersistDuration = 15 * time.Second
	// setPolicyDuration is the time estimate to set policies in Kiosk with setPolicyBlob or
	// clearPolicies.
	setPolicyDuration = 60*time.Second + policyPersistDuration
	// SetupDuration is the time estimate to set up a Kiosk session with kioskmode.New. This does not
	// include time to launch the session.
	SetupDuration = setPolicyDuration + CleanupDuration
	// LaunchDuration is the time estimate to launch a Kiosk session.
	LaunchDuration = kioskStartingDuration + kioskReadyToLaunchDuration + kioskLaunchSucceededDuration
	// CleanupDuration is the time estimate to clean up a Kiosk session with kiosk.Close.
	CleanupDuration = setPolicyDuration
)

// Kiosk structure holds necessary references and provides a way to safely
// close Kiosk mode.
type Kiosk struct {
	cr            *chrome.Chrome
	fdms          *fakedms.FakeDMS
	localAccounts *policy.DeviceLocalAccounts
	httpServer    *httptest.Server
	// reader for Chrome syslog messages from this Kiosk session. Used to wait for Kiosk logs.
	reader                         *syslog.Reader
	signinTestExtensionManifestKey string
	// TODO(b/280555587) remove this field when kiosk.DeprecatedClose is removed.
	autostart bool
}

// New sets up Chrome for a Kiosk session using policies based on the given options.
//
// Callers must clean up the resulting Kiosk struct with kiosk.Close.
//
// If auto launch was configured in opts, the app should eventually launch automatically. Otherwise,
// callers can use kiosmode.LaunchAppManually.
//
// Note New does not wait for Kiosk launch. Callers should use kiosk.WaitLaunchLogs.
//
// Tests using New should have a long enough Timeout to account for kioskmode.SetupDuration.
func New(ctx context.Context, fdms *fakedms.FakeDMS, signinTestExtensionManifestKey string, opts ...Option) (k *Kiosk, c *chrome.Chrome, retErr error) {
	// Make sure the context deadline is long enough before starting.
	if ctxutil.DeadlineBefore(ctx, time.Now().Add(SetupDuration)) {
		return nil, nil, errors.New("Insufficient time remaining for kioskmode.New")
	}

	// Parse necessary structs from test provided options.
	cfg, deviceLocalAccounts, httpServer, policyBlob, err := parseOptions(ctx, opts)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse Kiosk configuration from options")
	}
	defer func() {
		if retErr != nil && httpServer != nil {
			httpServer.Close()
		}
	}()

	// If an error occurs after the SetPolicyBlob call below, the device may or may not have Kiosk
	// policies set. From now on always clear Kiosk policies on error.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, CleanupDuration)
	defer cancel()
	defer func(ctx context.Context) {
		if retErr != nil {
			if err := clearPolicies(ctx, fdms, signinTestExtensionManifestKey); err != nil {
				testing.ContextLog(ctx, "Failed to clean up Kiosk policies, this may impact next test: ", err)
			}
		}
	}(cleanupCtx)

	// Apply Kiosk policies.
	testing.ContextLog(ctx, "Kiosk mode: Starting Chrome to set Kiosk policies")
	if err := setPolicyBlob(ctx, fdms, signinTestExtensionManifestKey, policyBlob, deviceLocalAccounts.Val); err != nil {
		return nil, nil, errors.Wrap(err, "failed to set Kiosk policy blob")
	}

	// Create a syslog.Reader before the new Chrome instance to capture Kiosk launch messages.
	reader, err := syslog.NewReader(ctx, syslog.Program("chrome"))
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to start Chrome syslog reader")
	}

	// Start a new Chrome instance now that Kiosk policies are in place.
	testing.ContextLog(ctx, "Kiosk mode: Starting Chrome after Kiosk policies were set")
	crOpts := []chrome.Option{
		chrome.NoLogin(),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment(),
	}
	crOpts = append(crOpts, cfg.m.ExtraChromeOptions...)
	cr, err := chrome.New(ctx, crOpts...)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to start chrome after Kiosk policies were set")
	}

	testing.ContextLog(ctx, "Kiosk mode: Setup succeeded")
	return &Kiosk{
		cr:                             cr,
		fdms:                           fdms,
		localAccounts:                  deviceLocalAccounts,
		httpServer:                     httpServer,
		reader:                         reader,
		signinTestExtensionManifestKey: signinTestExtensionManifestKey,
		autostart:                      cfg.m.AutoLaunch,
	}, cr, nil
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
func (k *Kiosk) Close(ctx context.Context) (retErr error) {
	if ctxutil.DeadlineBefore(ctx, time.Now().Add(CleanupDuration)) {
		testing.ContextLog(ctx, "Deadline too short for kiosk.Close, did you reserve cleanupCtx?")
		retErr = errors.New("potentially insufficient time remaining for kiosk.Close")
	}

	if k.httpServer != nil {
		k.httpServer.Close()
	}

	if k.reader != nil {
		if err := k.reader.Close(); err != nil {
			if retErr == nil {
				retErr = errors.Wrap(err, "failed to close Chrome syslog reader for Kiosk session")
			} else {
				testing.ContextLog(ctx, "Failed to close Chrome syslog reader for Kiosk session: ", err)
			}
		}
	}

	// Proceed with cleanup if Chrome is already closed or fails to close, because clearPolicies will
	// start a new Chrome instance.
	if k.cr != nil {
		if err := k.cr.Close(ctx); err != nil {
			if retErr == nil {
				retErr = errors.Wrap(err, "failed to close Chrome")
			} else {
				testing.ContextLog(ctx, "Failed to close Chrome: ", err)
			}
		}
	}

	if err := clearPolicies(ctx, k.fdms, k.signinTestExtensionManifestKey); err != nil {
		if retErr == nil {
			retErr = errors.Wrap(err, "failed to clean up Kiosk policies, this may impact next test")
		} else {
			testing.ContextLog(ctx, "Failed to clean up Kiosk policies, this may impact next test: ", err)
		}
	}
	return retErr
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

// WaitLaunchLogs is the same as the top level WaitLaunchLogs below, but uses the reader stored in
// this Kiosk struct.
//
// This avoids the caveats of creating the reader at the right time, and should be preferred.
func (k *Kiosk) WaitLaunchLogs(ctx context.Context) error {
	return WaitLaunchLogs(ctx, k.reader)
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
//
// TODO(b/280555587) consider removing this when callers migrate to kiosk.WaitLaunchLogs.
func WaitLaunchLogs(ctx context.Context, reader *syslog.Reader) error {
	if ctxutil.DeadlineBefore(ctx, time.Now().Add(LaunchDuration)) {
		// TODO(b/279900827): make this an error after callers are migrated.
		testing.ContextLog(ctx, "Potentially insufficient time remaining to wait for Kiosk launch")
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

	return &Kiosk{cr: cr, fdms: fdms, localAccounts: deviceLocalAccounts, httpServer: httpServer, autostart: cfg.m.AutoLaunch}, cr, nil
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

// LaunchAppManually starts the Kiosk app with given name from the Apps menu on the sign-in screen,
// simulating a manual launch. The given tconn should be a sign in profile test connection.
//
// It doesn't wait for a successful launch so that the launch can be cancelled by pressing
// Ctrl+Alt+S.
//
// See kiosk.WaitLaunchLogs to wait for launch.
//
// TODO(b/230840565): Extract and extend this function to support MGS.
func LaunchAppManually(ctx context.Context, tconn *chrome.TestConn, name string) error {
	testing.ContextLogf(ctx, "Kiosk mode: Starting Kiosk app from signin screen %q", name)
	ui := uiauto.New(tconn)
	localAccountsBtn := nodewith.Name("Apps").HasClass("MenuButton")
	if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(localAccountsBtn)(ctx); err != nil {
		return errors.Wrap(err, "failed to find 'Apps' button")
	}

	// There's a known issue with the "Apps" button, where clicking it too fast has no response, the
	// apps menu does not open, and the test hangs. As a result all manual launch Kiosk tests needed
	// to have this sleep before calling LaunchAppManually.
	//
	// GoBigSleepLint: TODO(b/280952514) "Apps" button in sign in screen needs some time.
	if err := testing.Sleep(ctx, 3*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep after finding 'Apps' button")
	}

	kioskAppBtn := nodewith.Name(name).HasClass("MenuItemView")
	if err := uiauto.Combine("launch Kiosk app from menu",
		ui.LeftClick(localAccountsBtn),
		ui.WaitUntilExists(kioskAppBtn),
		ui.LeftClick(kioskAppBtn),
		// Wait until the launch screen appears, or until it's gone and the Kiosk app is launched.
		ui.WaitUntilAnyExists(cancelLaunchText, chromeAppWindow, webAppWindow),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to start Kiosk application from apps menu")
	}
	return nil
}

// CancelKioskLaunch cancels the current Kiosk launch by pressing Ctrl+Alt+S.
//
// A new Chrome instance will be started with given opts.
func (k *Kiosk) CancelKioskLaunch(ctx context.Context, opts ...chrome.Option) (retCr *chrome.Chrome, retErr error) {
	// Make sure to clean up Chrome on error.
	defer func() {
		if retErr != nil && retCr != nil {
			if err := retCr.Close(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to close Chrome after cancel launch error: ", err)
			}
		}
	}()

	testing.ContextLog(ctx, "Kiosk mode: Cancelling Kiosk launch via Ctrl+Alt+S")
	if err := chrome.PrepareForRestart(); err != nil {
		return nil, errors.Wrap(err, "failed to remove old dev tools port file")
	}

	if err := waitSplashScreen(ctx, k.cr); err != nil {
		return nil, errors.Wrap(err, "failed to wait for Kiosk splash screen")
	}

	// Create the flag file so session_manager does not restart Chrome automatically. We will restart
	// it ourselves.
	clearFlag, err := setupDisableChromeRestartFlagFile()
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup flag file")
	}
	defer func() {
		if err := clearFlag(); err != nil {
			if retErr == nil {
				retErr = errors.Wrap(err, "failed to clear flag file")
			} else {
				testing.ContextLog(ctx, "Failed to clear flag file: ", err)
			}
		}
	}()

	// Find the current Chrome process to wait for it to shut down later.
	oldCr, err := ashproc.WaitForRoot(ctx, time.Minute)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find the browser process")
	}

	if err := pressCancelLaunchAccelerator(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to cancel launch")
	}

	if err := procutil.WaitForTerminated(ctx, oldCr, 10*time.Second); err != nil {
		return nil, errors.Wrap(err, "browser process didn't terminate")
	}

	// Clean up flag file we created.
	if err := clearFlag(); err != nil {
		return nil, errors.Wrap(err, "failed to remove flag file")
	}

	// Restart Chrome without closing since the current Chrome process already terminated.
	cr, err := k.restartChromeNoCloseWithOptions(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to restart Chrome")
	}
	return cr, nil
}

// setupDisableChromeRestartFlagFile creates a flag file to disable Chrome restart.
//
// The cleanup function returned can be used to later delete the file. It is safe and idempotent to
// run the cleanup function multiple times.
func setupDisableChromeRestartFlagFile() (func() error, error) {
	const disableChromeRestartFile = "/run/disable_chrome_restart"
	_, err := os.Create(disableChromeRestartFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Chrome flag file to disable restart")
	}
	didClear := false
	var clearErr error = nil
	clearFunc := func() error {
		if !didClear {
			if err := os.RemoveAll(disableChromeRestartFile); err != nil && !os.IsNotExist(err) {
				clearErr = errors.Wrap(err, "failed to remove Chrome flag file to reenable restart")
			}
		}
		didClear = true
		return clearErr
	}
	return clearFunc, nil
}

// waitSplashScreen waits for the Kiosk splash screen, as identified by the cancel launch message.
func waitSplashScreen(ctx context.Context, cr *chrome.Chrome) error {
	testConn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to signin extension")
	}

	ui := uiauto.New(testConn)
	if err := ui.WaitUntilExists(cancelLaunchText)(ctx); err != nil {
		return errors.Wrap(err, "failed to find splash screen")
	}
	return nil
}

// pressCancelLaunchAccelerator presses the "Ctrl+Alt+S" accelerator to cancel launch.
func pressCancelLaunchAccelerator(ctx context.Context) (retErr error) {
	kw, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create a keyboard")
	}
	if err := kw.Accel(ctx, "Ctrl+Alt+S"); err != nil {
		retErr = errors.Wrap(err, "failed to hit Ctrl+Alt+S and attempt to quit a kiosk app")
	}
	if err := kw.Close(ctx); err != nil {
		if retErr == nil {
			return errors.Wrap(err, "failed to close keyboard writer")
		}
		testing.ContextLog(ctx, "Failed to close keyboard writer: ", err)
	}
	return
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

// clearPolicies sets policies to an empty policy blob.
func clearPolicies(ctx context.Context, fdms *fakedms.FakeDMS, signinTestExtensionManifestKey string) error {
	return setPolicyBlob(ctx, fdms, signinTestExtensionManifestKey, policy.NewBlob(), []policy.DeviceLocalAccountInfo{})
}

// setPolicyBlob starts a new Chrome instance to set the given policy blob.
//
// Note that once this function returns Chrome will be closed and the device will be in the login
// screen.
//
// Callers are expected to start a new Chrome themselves after this function. This is required even
// if Kiosk auto launch policies were configured, as Kiosk cannot auto launch if no Chrome instance
// is running.
func setPolicyBlob(ctx context.Context, fdms *fakedms.FakeDMS, signinTestExtensionManifestKey string, pb *policy.Blob, accs []policy.DeviceLocalAccountInfo) (retErr error) {
	testing.ContextLog(ctx, "Kiosk mode: Starting Chrome in signin screen to set policies")
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

	if err := waitPoliciesPersisted(ctx, accs); err != nil {
		return errors.Wrap(err, "could not verify device local account policies persisted")
	}

	return nil
}

// waitPoliciesPersisted polls for files in /var/lib/device_local_accounts/<account>/policy/policy
// until the number of files matches the expected number of deviceLocalAccounts.
//
// This is neeced because policyutil.Refresh returns too early, before policies are stored in disk.
//
// TODO(b/282959122): Consider removing this function if policyutil.Refresh solves this.
func waitPoliciesPersisted(ctx context.Context, deviceLocalAccounts []policy.DeviceLocalAccountInfo) error {
	const policyDir = "/var/lib/device_local_accounts"

	checkPolicyFilesExist := func(_ context.Context) error {
		accountDirs, err := os.ReadDir(policyDir)
		if err != nil {
			return errors.Wrapf(err, "failed to read directory %q", policyDir)
		}
		// We can't match accountDirs to deviceLocalAccounts, so just check the number of policy files
		// is what we expect.
		if len(accountDirs) != len(deviceLocalAccounts) {
			return errors.Errorf("found %q entries in %q, expected %q", len(accountDirs), policyDir, len(deviceLocalAccounts))
		}
		for _, accountDir := range accountDirs {
			file := fmt.Sprintf("%v/%v/policy/policy", policyDir, accountDir.Name())
			if _, err := os.Stat(file); err != nil {
				return errors.Wrapf(err, "failed to stat %q, does it exist?", file)
			}
		}
		return nil
	}

	if err := testing.Poll(ctx, checkPolicyFilesExist, &testing.PollOptions{Timeout: policyPersistDuration}); err != nil {
		return errors.Wrap(err, "failed to find device local account policy files")
	}
	return nil
}

// parseOptions processes opts and returns relevant structs as needed for kioskmode.New.
func parseOptions(ctx context.Context, opts []Option) (_ *Config, _ *policy.DeviceLocalAccounts, _ *httptest.Server, _ *policy.Blob, retErr error) {
	cfg, err := NewConfig(opts)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "failed to process options")
	}

	deviceLocalAccounts, httpServer := deviceLocalAccountsForConfig(ctx, cfg)
	defer func() {
		if retErr != nil && httpServer != nil {
			httpServer.Close()
		}
	}()

	pb, err := policyBlobForConfig(cfg, deviceLocalAccounts)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "failed to create Kiosk policy blob for config")
	}
	return cfg, deviceLocalAccounts, httpServer, pb, nil
}

// deviceLocalAccountsForConfig sets up a policy.DeviceLocalAccounts slice and the default http
// server if necessary, as configured in the given cfg.
func deviceLocalAccountsForConfig(ctx context.Context, cfg *Config) (*policy.DeviceLocalAccounts, *httptest.Server) {
	if cfg.m.DeviceLocalAccounts == nil {
		httpServer := NewWebKioskAppServer(ctx)
		webKioskAppAccountInfo := WebKioskAppAccountInfo(httpServer.URL, WebKioskAccountID)
		deviceLocalAccounts := &policy.DeviceLocalAccounts{
			Val: []policy.DeviceLocalAccountInfo{KioskAppAccountInfo, webKioskAppAccountInfo}}
		return deviceLocalAccounts, httpServer
	}
	return cfg.m.DeviceLocalAccounts, nil
}

// policyBlobForConfig creates the policy.Blob for the given cfg.
func policyBlobForConfig(cfg *Config, deviceLocalAccounts *policy.DeviceLocalAccounts) (*policy.Blob, error) {
	// Set policies for device local accounts.
	policies := []policy.Policy{deviceLocalAccounts}

	// Set auto launch policies.
	if cfg.m.AutoLaunch {
		policies = append(policies, &policy.DeviceLocalAccountAutoLoginId{Val: *cfg.m.AutoLaunchKioskAppID})
	}

	// Set extra policies provided by the test.
	if cfg.m.ExtraPolicies != nil {
		policies = append(policies, cfg.m.ExtraPolicies...)
	}

	// Add policies to policy blob.
	pb := policy.NewBlob()
	if err := pb.AddPolicies(policies); err != nil {
		return nil, errors.Wrap(err, "failed to add policy slice to policy blob")
	}

	// Set public account policies.
	if cfg.m.PublicAccountPolicies != nil {
		for accountID, policies := range cfg.m.PublicAccountPolicies {
			if err := pb.AddPublicAccountPolicies(accountID, policies); err != nil {
				return nil, errors.Wrap(err, "failed to add public account policies to policy blob")
			}
		}
	}

	// Set custom directory API ID.
	if cfg.m.CustomDirectoryAPIID != nil {
		pb.DirectoryAPIID = *cfg.m.CustomDirectoryAPIID
	}

	return pb, nil
}
