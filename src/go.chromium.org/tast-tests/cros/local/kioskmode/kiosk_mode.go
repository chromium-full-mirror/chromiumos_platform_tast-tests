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

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/syslog"
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

// WaitLaunchLogs is the same as WaitLaunchLogsWithReader below, but uses the reader stored in this
// Kiosk struct.
//
// This avoids the caveats of creating the reader at the right time, and should be preferred.
func (k *Kiosk) WaitLaunchLogs(ctx context.Context) error {
	return WaitLaunchLogsWithReader(ctx, k.reader)
}

// WaitLaunchLogsWithReader uses reader to look for logs that confirm Kiosk mode launched successfully.
//
// reader is expected to process syslogs filtered for Chrome and to include messages since before
// the session was launched. As in:
//
//	reader, err := syslog.NewReader(ctx, syslog.Program("chrome"))
//	...
//	kiosk, cr, err := kioskmode.New(ctx, ...)  // Reader was created before Kiosk launches.
//	...
//	err := kioskmode.WaitLaunchLogsWithReader(ctx, reader)
//
// This is necessary because syslog.NewReader only contains logs from the moment it was created.
// Prefer using kiosk.WaitLaunchLogs, which does not have this problem.
//
// Tests using WaitLaunchLogsWithReader should have a long enough Timeout to account for
// kioskmode.LaunchDuration.
func WaitLaunchLogsWithReader(ctx context.Context, reader *syslog.Reader) error {
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

// RestartChromeWithOptions replaces the current Chrome in kiosk instance with a new one using the
// given opts.
func (k *Kiosk) RestartChromeWithOptions(ctx context.Context, opts ...chrome.Option) (_ *chrome.Chrome, retErr error) {
	if err := k.cr.Close(ctx); err != nil {
		retErr = errors.Wrap(err, "failed to close Chrome")
	}
	k.cr = nil

	if err := k.reader.Close(); err != nil {
		if retErr == nil {
			retErr = errors.Wrap(err, "failed to close Chrome syslog reader for Kiosk session")
		} else {
			testing.ContextLog(ctx, "Failed to close Chrome syslog reader for Kiosk session: ", err)
		}
	}
	k.reader = nil

	if retErr != nil {
		return nil, retErr
	}

	// Create reader before Chrome to make sure it captures all logs from the Kiosk launch.
	reader, err := syslog.NewReader(ctx, syslog.Program("chrome"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome syslog reader")
	}
	defer func() {
		if retErr != nil {
			if err := reader.Close(); err != nil {
				testing.ContextLog(ctx, "Failed to close Chrome syslog reader for Kiosk session after error: ", err)
			}
		}
	}()

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start new Chrome")
	}

	k.cr = cr
	k.reader = reader
	return cr, nil
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
	localAccountsBtn := nodewith.Name("Apps").HasClass("LoginShelfButton")
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
// This is needed because policyutil.Refresh returns too early, before policies are stored in disk.
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
