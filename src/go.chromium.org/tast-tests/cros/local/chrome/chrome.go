// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package chrome implements a library used for communication with Chrome.
package chrome

import (
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"android.googlesource.com/platform/external/perfetto/protos/perfetto/trace/github.com/google/perfetto/perfetto_proto"
	"github.com/golang/protobuf/proto"

	"go.chromium.org/tast-tests/cros/local/chrome/ash/ashproc"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/internal/cdputil"
	"go.chromium.org/tast-tests/cros/local/chrome/internal/config"
	"go.chromium.org/tast-tests/cros/local/chrome/internal/driver"
	"go.chromium.org/tast-tests/cros/local/chrome/internal/extension"
	"go.chromium.org/tast-tests/cros/local/chrome/internal/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/internal/login"
	"go.chromium.org/tast-tests/cros/local/chrome/internal/setup"
	"go.chromium.org/tast-tests/cros/local/chrome/jslog"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/logsaver"
	"go.chromium.org/tast-tests/cros/local/minidump"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast/core/caller"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

var chromeKeepStateVar = testing.RegisterVarString(
	"chrome.keepState",
	"false",
	"chrome.KeepState decides whether to pass KeepState to Chrome.New by default",
)

var chromeReuseSession = testing.RegisterVarString(
	"chrome.tryReuseSession",
	"false",
	"chrome.tryReuseSession decides whether to pass TryReuseSession to Chrome.New by default",
)

const (
	// LoginTimeout is the maximum amount of time that Chrome is expected to take to perform login.
	// Tests that call New with the default fake login mode should declare a timeout that's at least this long.
	// Tast waits for login by checking when all partitions are mounted and ready. For normal login this takes up most of the time.
	// TODO: Once we are at Go 1.21, change this to
	// max(cryptohome.WaitForUserTimeout, MinLoginTimeout)
	LoginTimeout = MinLoginTimeout

	// GAIALoginTimeout is the maximum amount of the time that Chrome is expected
	// to take to perform actual gaia login. As far as I checked a few samples of
	// actual test runs, most of successful login finishes within ~40secs. Use
	// 40*3=120 seconds for the safety.
	GAIALoginTimeout = LoginTimeout + 40*time.Second

	// GAIALoginChildTimeout is the maximum amount of time that Chrome is expected
	// to take to perform actual gaia login for a child account. Use double the
	// regular GAIALoginTimeout because we check credentials for two users (child and parent).
	GAIALoginChildTimeout = 2 * GAIALoginTimeout

	// ManagedUserLoginTimeout is the maximum amount of time that Chrome is expected to take to perform login for a managed user.
	// Tests that call New with the default fake login mode and a managed user should declare a timeout that's at least this long.
	// TODO(crbug.com/1199705): Find a better value or go back to LoginTimeout.
	ManagedUserLoginTimeout = LoginTimeout + 30*time.Second

	// EnrollmentAndLoginTimeout is the maximum amount of time that Chrome is expected to take to perform both enrollment and login.
	// Tests that call New with both enrollment and the default fake login mode should declare a timeout that's at least this long.
	// TODO(crbug.com/1199705): Find a better value.
	EnrollmentAndLoginTimeout = LoginTimeout + 1*time.Minute

	// MinLoginTimeout is the minimum timeout for the login operation.
	// Login timeout shorter than this amount would be extended
	// to ensure the Chrome has enough time to perform login.
	// See b/269211070 for more information.
	MinLoginTimeout = 4*time.Minute + 10*time.Second

	// tryReuseSessionTimeout is the maximum amount of time that Chrome is expected to take to perform
	// session reuse checking. Chrome will connect to the existing Chrome instance, obtained the
	// existing configuration, and compare with the new session config. This procedure doesn't
	// restart the Chrome UI and should finish fast. If this procdure fails, we still have time for new
	// session login.
	tryReuseSessionTimeout = 10 * time.Second

	// TestExtensionID is an extension ID of the autotest extension. It
	// corresponds to testExtensionKey.
	TestExtensionID = extension.TestExtensionID

	// BlankURL is the URL corresponding to the about:blank page.
	BlankURL = "about:blank"

	// NewTabURL is the URL corresponding to the chrome://newtab/ page.
	// NOTE: A trailing slash is added to the URL in case that it could be passed over to NewConnForTarget.
	// The given URL must match exactly what Chrome ends up associating with the tab.
	// For example, "chrome://newtab/" gets loaded in the tab instead of "chrome://newtab".
	NewTabURL = "chrome://newtab/"

	// VersionURL is the URL corresponding to the chrome://version/ page.
	// NOTE: A trailing slash is added to the URL in case that it could be passed over to NewConnForTarget.
	VersionURL = "chrome://version/"

	// SigninInternalsURL is the URL corresponding to the chrome signin-internals page.
	// NOTE: A trailing slash is added to the URL in case that it could be passed over to NewConnForTarget.
	SigninInternalsURL = "chrome://signin-internals/"

	// persistentDir is a directory to save files that should persist even
	// after Tast finishes. For instance, we save test extensions here so
	// that Chrome does not malfunction on post-test manual inspection.
	// This directory is cleared at the beginning of chrome.New.
	persistentDir = "/usr/local/tmp/tast/chrome_session"
	// extensionsDir is the directory for all chrome session extensions.
	extensionsDir = persistentDir + "/extensions"
	// lacrosExtensionsDir is the directory for browser extensions.
	lacrosExtensionsDir = persistentDir + "/lacros_extensions"
)

// locked is set to true while a precondition is active to prevent tests from calling New or Chrome.Close.
var locked = false

// prePackages lists packages containing preconditions that are allowed to call Lock and Unlock.
var prePackages = []string{
	"go.chromium.org/tast-tests/cros/local/arc",
	"go.chromium.org/tast-tests/cros/local/mgs",
	"go.chromium.org/tast-tests/cros/local/policyutil/pre",
	"go.chromium.org/tast-tests/cros/local/bundles/cros/apps/fixture",
	"go.chromium.org/tast-tests/cros/local/inputs/fixture",
	"go.chromium.org/tast-tests/cros/local/inputs/pre",
	"go.chromium.org/tast-tests-private/crosint/local/bundles/crosint/arc",
	"go.chromium.org/tast-tests/cros/local/bundles/crosint/camera/fixtures",
	"go.chromium.org/tast-tests-private/crosint/local/bundles/crosint/pita/pre",
	"go.chromium.org/tast-tests/cros/local/bundles/pita/pita/pre",
	"go.chromium.org/tast-tests/cros/local/chrome",
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice",
	"go.chromium.org/tast-tests/cros/local/chrome/cuj",
	"go.chromium.org/tast-tests/cros/local/chrome/nearbyshare/nearbyfixture",
	"go.chromium.org/tast-tests/cros/local/chrome/familylink",
	"go.chromium.org/tast-tests/cros/local/chrome/mtp",
	"go.chromium.org/tast-tests/cros/local/chrome/projector",
	"go.chromium.org/tast-tests/cros/local/accountmanager",
	"go.chromium.org/tast-tests/cros/local/crostini",
	"go.chromium.org/tast-tests/cros/local/drivefs",
	"go.chromium.org/tast-tests/cros/local/graphics",
	"go.chromium.org/tast-tests/cros/local/kioskmode/fixtures",
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt",
	"go.chromium.org/tast-tests/cros/local/media/pre",
	"go.chromium.org/tast-tests/cros/local/multivm",
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures",
	"go.chromium.org/tast-tests/cros/local/policyutil/pre",
	"go.chromium.org/tast-tests/cros/local/power/setup",
	"go.chromium.org/tast-tests/cros/local/vdi/fixtures",
	"go.chromium.org/tast-tests/cros/local/wpr",
	"go.chromium.org/tast-tests/cros/local/saveddesks",
}

// Lock prevents from New or Chrome.Close from being called until Unlock is called.
// It can only be called by preconditions and is idempotent.
func Lock() {
	caller.Check(2, prePackages)
	locked = true
}

// Unlock allows New and Chrome.Close to be called after an earlier call to Lock.
// It can only be called by preconditions and is idempotent.
func Unlock() {
	caller.Check(2, prePackages)
	locked = false
}

// Chrome interacts with the currently-running Chrome instance via the
// Chrome DevTools protocol (https://chromedevtools.github.io/devtools-protocol/).
type Chrome struct {
	// cfg contains configurations computed from options given to chrome.New.
	// Its fields must not be altered after its construction.
	cfg config.Config
	// deprecatedExtDirs holds the directories of the test extensions and will
	// only be used by DeprecatedExtDirs().
	deprecatedExtDirs []string

	agg  *jslog.Aggregator
	sess *driver.Session

	logFilename string
	logMarker   *logsaver.Marker

	// The time just before ash-chrome is (re)started. This timestamp marks the
	// earliest time for considering Lacros logs as part of the current test.
	// TODO(andreaorru): support the reuse case.
	logsStartTime time.Time

	loginPending bool // true if login is pending until ContinueLogin is called
}

// HasChrome is an interface for fixture values that contain a Chrome instance. It allows
// retrieval of the underlying Chrome object.
type HasChrome interface {
	Chrome() *Chrome
}

// Chrome returns the Chrome instance.
// It implements the HasChrome interface.
func (c *Chrome) Chrome() *Chrome { return c }

// Browser returns a Browser instance.
func (c *Chrome) Browser() *browser.Browser {
	return browser.New(c.sess, true)
}

// Creds returns credentials used to log into a session.
func (c *Chrome) Creds() Creds { return c.cfg.Creds() }

// VKEnabled returns whether virtual keyboard is enabled.
func (c *Chrome) VKEnabled() bool { return c.cfg.VKEnabled() }

// LoginMode returns the user login mode as string.
func (c *Chrome) LoginMode() string {
	switch c.cfg.LoginMode() {
	case config.FakeLogin:
		return "Fake"
	case config.GAIALogin:
		return "GAIA"
	case config.GuestLogin:
		return "Guest"
	case config.NoLogin:
		return "NoLogin"
	}
	return "Unknown"
}

// User returns the username that was used to log in to Chrome. Note that in almost all cases you actually want NormalizedUser below.
func (c *Chrome) User() string { return c.cfg.Creds().User }

// NormalizedUser returns the normalized (lowercase and striping '.' characters) username that was used to log in to Chrome.
func (c *Chrome) NormalizedUser() string { return c.cfg.NormalizedUser() }

// LacrosExtraArgs returns the extra arguments that should be added to the Lacros command line.
func (c *Chrome) LacrosExtraArgs() []string { return c.cfg.LacrosExtraArgs() }

// DeprecatedExtDirs returns the directories holding the test extensions.
// For reused Chrome session, deprecatedExtDirs is not set and this method will return nil.
//
// DEPRECATED: This method does not handle sign-in profile extensions correctly.
func (c *Chrome) DeprecatedExtDirs() []string {
	return c.deprecatedExtDirs
}

// DebugAddrPort returns the addr:port at which Chrome is listening for DevTools connections,
// e.g. "127.0.0.1:38725". This port should not be accessed from outside of this package,
// but it is exposed so that the port's owner can be easily identified.
func (c *Chrome) DebugAddrPort() string {
	return c.sess.DebugAddrPort()
}

// LogFilename returns the real path of the log file for the Chrome.
func (c *Chrome) LogFilename() string {
	return c.logFilename
}

// New restarts the ui job, tells Chrome to enable testing, and (by default) logs in.
// The NoLogin option can be passed to avoid logging in.
func New(ctx context.Context, opts ...Option) (c *Chrome, retErr error) {
	if locked {
		panic("Cannot create Chrome instance while precondition is being used")
	}

	keepState := chromeKeepStateVar.Value()
	if keepState == "true" {
		opts = append(opts, KeepState())
	}
	shouldTryReuse, err := strconv.ParseBool(chromeReuseSession.Value())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %v(%v) to bool", chromeReuseSession.Name(), chromeReuseSession.Value())
	}
	if shouldTryReuse {
		opts = append(opts, TryReuseSession())
	}

	ctx, st := timing.Start(ctx, "chrome_new")
	defer st.End()

	// Override privacy sandbox dialog feature to hide it (crbug.com/330241089).
	opts = append(opts, EnableFeatures("PrivacySandboxSettings4"))

	opts = append(opts, ExtraArgs("--cryptohome-ignore-cleanup-ownership-for-testing"))

	cfg, err := config.NewConfig(opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to process options")
	}

	// Cap the timeout to be certain length depending on the login mode. Sometimes
	// chrome.New may fail and get stuck on an unexpected screen. Without timeout,
	// it simply runs out the entire timeout. See https://crbug.com/1078873.
	timeout := LoginTimeout
	if cfg.LoginMode() == config.GAIALogin {
		timeout = GAIALoginTimeout
	}
	// Allow a custom timeout to be set.
	if cfg.CustomLoginTimeout() != 0 {
		timeout = cfg.CustomLoginTimeout()
	}
	// b/211032595: Sometimes, vm test requires longer timeout.
	// b/269211070: Sometimes, gaia login requires a longer timeout.
	// Make sure the timeout to be at least 4 minutes and 10 sec.
	if timeout < MinLoginTimeout {
		timeout = MinLoginTimeout
	}
	origCtx := ctx
	ctx, cancel := context.WithTimeout(origCtx, timeout)
	defer cancel()

	// Gaia Profiling: background internet check with timeout.
	if cfg.LoginMode() == config.GAIALogin {
		checkInternetConnectivityInBackground(ctx, 40*time.Second)
	}

	// Check whether ctx is long enough.
	deadline, _ := ctx.Deadline()
	remaining := time.Until(deadline)
	cxtMayBeShort := remaining < timeout

	// In case chrome.New fails for a deadline error, which might be caused
	// by a browser hang, take minidump snapshots for diagnosis.
	defer func(ctx context.Context) {
		/**
		* "context deadline exceeded" is not clear and people may be misled. See 2 examples:
		* - b/267295043
		* - b/257471572
		* I propose that: if retErr contains "context deadline exceeded"
		* and the ctx deadline duration is less than recommendation,
		* a reminder message will be added.
		* Here is a error message sample:
		* "Failed to start Chrome: context deadline duration 14.999971871s exceed,
		* it's better to have at least 4m0s: login failed: failed to finish user login:
		* waiting for cryptohome failed: failed to wait for user mount and validate type:
		* failed to get user home path: failed to call cryptohome-path user: context deadline exceeded"
		 */
		if retErr != nil && strings.Contains(retErr.Error(), "context deadline exceeded") && cxtMayBeShort {
			retErr = errors.Wrapf(retErr, "context deadline duration %v exceed, it's better to have at least %v", remaining, timeout)
		}
		if retErr == nil || ctx.Err() == nil || origCtx.Err() != nil {
			return
		}
		ctx, st := timing.Start(ctx, "save_minidumps")
		defer st.End()
		testing.ContextLog(ctx, "Taking minidump snapshots to diagnose possible browser hang")
		if err := saveMinidumpsWithoutCrash(origCtx); err != nil {
			testing.ContextLog(ctx, "Failed to take minidump snapshots: ", err)
		}
	}(ctx)

	if err := setup.PreflightCheck(ctx, cfg); err != nil {
		return nil, errors.Wrap(err, "pre-flight check failed")
	}

	tryReuse := cfg.TryReuseSession()
	forceReuse := cfg.ForceReuseSession()
	if tryReuse || forceReuse {
		reuseCtx, reuseCancel := context.WithTimeout(ctx, tryReuseSessionTimeout)
		defer reuseCancel()

		reuseSession := tryReuseSession
		if forceReuse {
			reuseSession = forceReuseSession
		}
		cr, err := reuseSession(reuseCtx, cfg)

		if err == nil {
			return cr, nil
		}
		testing.ContextLogf(ctx, "Current session is not reusable: %v; restarting a new session", err)
	}

	if err := os.RemoveAll(persistentDir); err != nil {
		return nil, err
	}

	// Prepare extensions.
	guestModeLogin := extension.GuestModeDisabled
	if cfg.LoginMode() == config.GuestLogin {
		guestModeLogin = extension.GuestModeEnabled
	}
	exts, err := extension.PrepareExtensions(extensionsDir, cfg, guestModeLogin)
	if err != nil {
		return nil, errors.Wrap(err, "failed to prepare extensions for ash-chrome")
	}
	lacrosExts, err := extension.PrepareExtensions(lacrosExtensionsDir, cfg, guestModeLogin)
	if err != nil {
		return nil, errors.Wrap(err, "failed to prepare extensions for lacros-chrome")
	}

	logsStartTime := time.Now().UTC()
	if err := setup.RestartChromeForTesting(ctx, cfg, exts.AshArgs(), lacrosExts.LacrosArgs()); err != nil {
		return nil, errors.Wrap(err, "failed to restart chrome for testing")
	}

	agg := jslog.NewAggregator()
	defer func() {
		if retErr != nil {
			agg.Close()
		}
	}()

	sess, err := driver.NewSession(ctx, ashproc.ExecPath, cdputil.DebuggingPortPath, cdputil.WaitPort, agg)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to establish connection to Chrome Debugging Protocol with debugging port path=%q", cdputil.DebuggingPortPath)
	}
	defer func() {
		if retErr != nil {
			sess.Close(ctx)
		}
	}()

	if cfg.LoginMode() != config.NoLogin && !cfg.KeepState() {
		if err := cryptohome.RemoveUserDir(ctx, cfg.NormalizedUser()); err != nil {
			return nil, errors.Wrapf(err, "failed to remove cryptohome user directory for %s", cfg.NormalizedUser())
		}
	}

	logFilename, err := CurrentLogFile()
	if err != nil {
		return nil, errors.Wrap(err, "failed to find the log filename")
	}
	testing.ContextLogf(ctx, "Log file name: %s", logFilename)

	// In case chrome.New fails for a deadline error, which might be caused
	// by a browser hang, write out the current chrome log.
	defer func(ctx context.Context) {
		if retErr == nil || ctx.Err() == nil || origCtx.Err() != nil {
			return
		}
		testing.ContextLog(ctx, "Saving the current chrome log to the output directory")
		if err := saveChromeLog(origCtx, logFilename); err != nil {
			testing.ContextLog(ctx, "Failed to save chrome log: ", err)
		}
	}(ctx)

	loginPending := false
	if cfg.DeferLogin() {
		loginPending = true
	} else {
		if err := login.LogIn(ctx, cfg, sess); err == login.ErrNeedNewSession {
			// Restart session.
			newSess, err := driver.NewSession(ctx, ashproc.ExecPath, cdputil.DebuggingPortPath, cdputil.WaitPort, agg)
			if err != nil {
				return nil, errors.Wrap(err, "failed to reconnect to restarted session")
			}
			sess.Close(ctx)
			sess = newSess
		} else if err != nil {
			return nil, errors.Wrap(err, "login failed")
		}
	}

	return &Chrome{
		cfg:               *cfg,
		deprecatedExtDirs: exts.DeprecatedDirs(),
		agg:               agg,
		sess:              sess,
		logFilename:       logFilename,
		logMarker:         logsaver.NewMarkerNoOffset(logFilename),
		logsStartTime:     logsStartTime,
		loginPending:      loginPending,
	}, nil
}

// Close disconnects from Chrome and cleans up standard extensions.
// To avoid delays between tests, the ui job (and by extension, Chrome) is not restarted,
// so the current user (if any) remains logged in.
func (c *Chrome) Close(ctx context.Context) error {
	if locked {
		panic("Do not call Close while precondition is being used")
	}

	if c.sess != nil {
		c.sess.Close(ctx)
		c.sess = nil
	}

	dir, dirOk := testing.ContextOutDir(ctx)
	defer c.agg.Close()

	if dirOk {
		return c.saveLogs(ctx, dir)
	}

	testing.ContextLog(ctx, "No output directory exists, not saving log file")
	return nil
}

func (c *Chrome) saveLogs(ctx context.Context, outDir string) error {
	c.agg.Save(filepath.Join(outDir, "jslog.txt"))

	if err := lacros.SaveLogsAfter(ctx, outDir, c.logsStartTime); err != nil {
		testing.ContextLog(ctx, "Failed to store per-test Lacros log data: ", err)
	}

	if err := c.logMarker.Save(filepath.Join(outDir, filepath.Base(c.logFilename))); err != nil {
		testing.ContextLog(ctx, "Failed to save the entire log: ", err)
		return err
	}
	return nil
}

// SaveLogsOnError saves jsLog.txt, chrome_$date-$time and lacros_$date-$time.log in the outDir when hasError returns true.
func (c *Chrome) SaveLogsOnError(ctx context.Context, outDir string, hasError func() bool) error {
	if hasError() {
		return c.saveLogs(ctx, outDir)
	}
	return nil
}

// shouldCloseOnReset filters out targets should be closed in resetting Chrome state.
// it tries to close all "normal" pages, apps and dialog boxes and exclude exemptions.
func shouldCloseOnReset(t *Target) bool {
	// Chrome OS Virtual Keyboard is permanently cached in Chrome Session to speed up loading.
	if t.Type == "other" && (t.Title == "Chrome OS Virtual Keyboard" ||
		strings.HasPrefix(t.URL, "chrome-extension://fgoepimhcoialccpbmpnnblemnepkkao") || // Don't close ChromeOS xkb extension.
		strings.HasPrefix(t.URL, "chrome-extension://jkghodnilhceideoidjikpgommlajknk") || // Don't close input methods extension.
		strings.HasPrefix(t.URL, "chrome-extension://mndnfokpggljbaajbnioimlmbfngpief") || // Don't close ChromeVox extension.
		strings.HasPrefix(t.URL, "chrome://tab-strip.top-chrome/") || // Don't close the tab strip.
		// Don't close the print preview (it will be closed automatically with the parent page).
		// The comparison between URL and Title is needed so that the chrome://print/ opened on a new page can be closed normally.
		// See more details in b/268483323.
		(strings.HasPrefix(t.URL, "chrome://print/") && t.URL != t.Title)) {
		return false
	}
	return t.Type == "page" || t.Type == "app" || t.Type == "other"
}

// CloseTargets closes all targets matched by TargetMatcher.
func (c *Chrome) CloseTargets(ctx context.Context, tm cdputil.TargetMatcher) error {
	targets, err := c.FindTargets(ctx, tm)
	if err != nil {
		return errors.Wrap(err, "failed to get targets")
	}
	var closingTargets []*Target
	if len(targets) > 0 {
		testing.ContextLogf(ctx, "Closing %d target(s)", len(targets))
		for _, t := range targets {
			if err := c.CloseTarget(ctx, t.TargetID); err != nil {
				testing.ContextLogf(ctx, "Failed to close %v: %v", t.URL, err)
			} else {
				// Record all targets that have promised to close
				closingTargets = append(closingTargets, t)
			}
		}
	}
	// Wait for the targets to finish closing
	return testing.Poll(ctx, func(ctx context.Context) error {
		targets, err := c.FindTargets(ctx, tm)
		if err != nil {
			return errors.Wrap(err, "failed to get targets")
		}
		var stillClosingCount int
		for _, ct := range closingTargets {
			for _, t := range targets {
				if ct.TargetID == t.TargetID {
					stillClosingCount++
					break
				}
			}
		}
		if stillClosingCount > 0 {
			return errors.Errorf("%d target(s) still open", stillClosingCount)
		}
		return nil
	}, &testing.PollOptions{Interval: 10 * time.Millisecond, Timeout: 15 * time.Second})
}

// ResetState attempts to reset Chrome's state (e.g. by closing all pages).
// Tests typically do not need to call this; it is exposed primarily for other packages.
func (c *Chrome) ResetState(ctx context.Context) error {
	testing.ContextLog(ctx, "Resetting Chrome's state")
	ctx, st := timing.Start(ctx, "reset_chrome")
	defer st.End()

	tconn, err := c.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get test API connection")
	}

	// First deal with Lacros, which may or may not be enabled.
	if err := lacros.ResetState(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to reset Lacros's state")
	}

	// Now deal with Ash.
	if err := c.CloseTargets(ctx, shouldCloseOnReset); err != nil {
		return errors.Wrap(err, "not all targets finished closing")
	}

	// If the test case started the tracing but somehow StopTracing isn't called,
	// the tracing should be stopped in ResetState.
	if c.sess.TracingStarted() {
		// As noted in the comment of c.StartTracing, the tracingStarted flag is
		// marked before actually StartTracing request is sent because
		// StartTracing's failure doesn't necessarily mean that tracing isn't
		// started. So at this point, c.StopTracing may fail if StartTracing failed
		// and tracing actually didn't start. Because of that, StopTracing's error
		// wouldn't cause an error of ResetState, but simply reporting the error
		// message.
		if _, err := c.sess.StopTracing(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to stop tracing: ", err)
		}
	}

	// Free all remote JS objects in the test extension.
	if err := driver.PrivateReleaseAllObjects(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to free tast remote JS object group")
	}

	if c.cfg.VKEnabled() {
		// Calling the method directly to avoid vkb/chrome circular imports.
		if err := tconn.Call(ctx, nil, "tast.promisify(chrome.inputMethodPrivate.hideInputView)"); err != nil {
			return errors.Wrap(err, "failed to hide virtual keyboard")
		}

		// Waiting until virtual keyboard disappears from a11y tree.
		var isVKShown bool
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if err := tconn.Eval(ctx, `
				tast.promisify(chrome.automation.getDesktop)().then(
					root => {return !!(root.find({role: 'rootWebArea', name: 'Chrome OS Virtual Keyboard'}))}
				)`, &isVKShown); err != nil {
				return errors.Wrap(err, "failed to hide virtual keyboard")
			}
			if isVKShown {
				return errors.New("virtual keyboard is still visible")
			}
			return nil
		}, &testing.PollOptions{Interval: 3 * time.Second, Timeout: 30 * time.Second}); err != nil {
			return errors.Wrap(err, "failed to wait for virtual keyboard to be invisible")
		}
	}

	// Release the mouse buttons in case a test left them pressed. If a button
	// is already released, releasing it is a no-op. Call the method directly to
	// avoid chrome/mouse circular imports.
	// TODO(crbug.com/1096647): Log when a mouse button is pressed.
	for _, button := range []string{"Left", "Right", "Middle"} {
		if err := tconn.Eval(ctx, fmt.Sprintf(`tast.promisify(chrome.autotestPrivate.mouseRelease)(%q)`, button), nil); err != nil {
			return errors.Wrapf(err, "failed to release %s mouse button", button)
		}
	}

	// Clear all notifications in case a test generated some but did not close them.
	if err := tconn.Eval(ctx, "tast.promisify(chrome.autotestPrivate.removeAllNotifications)()", nil); err != nil {
		return errors.Wrap(err, "failed to clear notifications")
	}

	// Disable the automation feature. Otherwise, automation tree updates and
	// events will come to the test API, and sometimes it causes significant
	// performance drawback on low-end devices. See: https://crbug.com/1096719.
	if err := tconn.ResetAutomation(ctx); err != nil {
		return errors.Wrap(err, "failed to reset the automation feature")
	}

	return nil
}

// Reconnect reconnects to the current browser session.
//
// WARNING: You cannot use this method to recover from Chrome crashes you don't
// have full control of. Read on to see why.
//
// Call this method when you know you have to re-establish a connection to the
// browser session, e.g. after suspend/resume. After the session is reconnected,
// all existing connections associated with this chrome.Chrome instance also
// needs to be re-established. For example, you should call
// chrome.TestAPIConn(), chrome.NewConn(), or chrome.NewConnForTarget() to get
// the new connections for your test.
//
// If Chrome browser process restarts (e.g. for crash), its devtools port can
// change, so you cannot simply use this method to reliably reconnect to the new
// Chrome process. If your test intentionally crashes Chrome, call
// PrepareForRestart in advance so that Reconnect doesn't attempt to connect to
// an old port.
func (c *Chrome) Reconnect(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	// Create a new session.
	newSess, err := driver.NewSession(ctx, ashproc.ExecPath, cdputil.DebuggingPortPath, cdputil.WaitPort, c.agg)
	if err != nil {
		return err
	}
	c.sess.Close(ctx)
	c.sess = newSess
	return nil
}

// Conn represents a connection to a web content view, e.g. a tab.
type Conn = driver.Conn

// JSObject is a reference to a JavaScript object.
// JSObjects must be released or they will stop the JavaScript GC from freeing the memory they reference.
type JSObject = driver.JSObject

// NewConn creates a new Chrome renderer and returns a connection to it.
// If url is empty, an empty page (about:blank) is opened. Otherwise, the page
// from the specified URL is opened. You can assume that the page loading has
// been finished when this function returns.
func (c *Chrome) NewConn(ctx context.Context, url string, opts ...cdputil.CreateTargetOption) (*Conn, error) {
	return c.sess.NewConn(ctx, url, opts...)
}

// NewBackgroundConn returns NewConn with cdputil.WithBAckground() option specified.
func (c *Chrome) NewBackgroundConn(ctx context.Context, url string, opts ...cdputil.CreateTargetOption) (*Conn, error) {
	opts = append(opts, cdputil.WithBackground())
	return c.NewConn(ctx, url, opts...)
}

// Target describes a DevTools target.
type Target = driver.Target

// TargetID is an ID assigned to a DevTools target.
type TargetID = driver.TargetID

// TargetMatcher is a caller-provided function that matches targets with specific characteristics.
type TargetMatcher = driver.TargetMatcher

// MatchTargetID returns a TargetMatcher that matches targets with the supplied ID.
func MatchTargetID(id TargetID) TargetMatcher {
	return driver.MatchTargetID(id)
}

// MatchTargetURL returns a TargetMatcher that matches targets with the supplied URL.
func MatchTargetURL(url string) TargetMatcher {
	return driver.MatchTargetURL(url)
}

// MatchTargetURLPrefix returns a TargetMatcher that matches targets whose URL starts with the
// supplied prefix.
func MatchTargetURLPrefix(prefix string) TargetMatcher {
	return driver.MatchTargetURLPrefix(prefix)
}

// MatchAllPages returns a TargetMatcher that matches every target that is a page.
func MatchAllPages() TargetMatcher {
	return driver.MatchAllPages()
}

// NewConnForTarget iterates through all available targets and returns a connection to the
// first one that is matched by tm. It polls until the target is found or ctx's deadline expires.
// An error is returned if no target is found, tm matches multiple targets, or the connection cannot
// be established.
//
//	f := func(t *Target) bool { return t.URL == "http://example.net/" }
//	conn, err := cr.NewConnForTarget(ctx, f)
func (c *Chrome) NewConnForTarget(ctx context.Context, tm TargetMatcher) (*Conn, error) {
	return c.sess.NewConnForTarget(ctx, tm)
}

// FindTargets returns the info about Targets, which satisfies the given cond condition.
func (c *Chrome) FindTargets(ctx context.Context, tm TargetMatcher) ([]*Target, error) {
	return c.sess.FindTargets(ctx, tm)
}

// CloseTarget closes the target identified by the given id.
func (c *Chrome) CloseTarget(ctx context.Context, id TargetID) error {
	return c.sess.CloseTarget(ctx, id)
}

// TestConn is a connection to the Tast test extension's background page.
// cf) crbug.com/1043590
type TestConn = driver.TestConn

// ErrTestConnUndefinedOut is the error returned by cdputil when the result of
// some javascript is undefined, but you attempt to store it in a value.
var ErrTestConnUndefinedOut = cdputil.ErrUndefinedOut

// TestAPIConn returns a shared connection to the test API extension's
// background page (which can be used to access various APIs). The connection is
// lazily created, and this function will block until the extension is loaded or
// ctx's deadline is reached. The caller should not close the returned
// connection; it will be closed automatically by Close.
func (c *Chrome) TestAPIConn(ctx context.Context) (*TestConn, error) {
	return c.Browser().TestAPIConn(ctx)
}

// SigninProfileTestAPIConn is the same as TestAPIConn, but for the signin
// profile test extension.
func (c *Chrome) SigninProfileTestAPIConn(ctx context.Context) (*TestConn, error) {
	return c.sess.SigninProfileTestAPIConn(ctx)
}

// Responded performs basic checks to verify that Chrome has not crashed.
func (c *Chrome) Responded(ctx context.Context) error {
	ctx, st := timing.Start(ctx, "check_chrome")
	defer st.End()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := c.TestAPIConn(ctx)
	if err != nil {
		return err
	}
	result := false
	if err = conn.Eval(ctx, "true", &result); err != nil {
		return err
	}
	if !result {
		return errors.New("eval 'true' returned false")
	}
	return nil
}

// WaitForOOBEConnection waits for that the OOBE page is shown, then returns
// a connection to the page. The caller must close the returned connection.
func (c *Chrome) WaitForOOBEConnection(ctx context.Context) (*Conn, error) {
	return login.WaitForOOBEConnection(ctx, c.sess)
}

// WaitForRMAConnection waits for the RMA dialog to be shown, then returns
// a connection to the page. The caller must close the returned connection.
func (c *Chrome) WaitForRMAConnection(ctx context.Context) (*Conn, error) {
	return login.WaitForRMAConnection(ctx, c.sess)
}

// WaitForCFMConnection waits for the CFM dialog to be shown, then returns
// a connection to the page. The caller must close the returned connection.
func (c *Chrome) WaitForCFMConnection(ctx context.Context) (*Conn, error) {
	return login.WaitForCFMConnection(ctx, c.sess)
}

// WaitForOOBEConnectionToBeDismissed waits for that the OOBE page to be
// dismissed.
func (c *Chrome) WaitForOOBEConnectionToBeDismissed(ctx context.Context) error {
	return login.WaitForOOBEConnectionToBeDismissed(ctx, c.sess)
}

// WaitForOOBEConnectionWithPrefix waits for the prefix OOBE page to be shown,
// then returns a connection to the page. The caller must close the returned
// connection.
func (c *Chrome) WaitForOOBEConnectionWithPrefix(ctx context.Context, prefix string) (*Conn, error) {
	return login.WaitForOOBEConnectionWithPrefix(ctx, c.sess, prefix)
}

// ContinueLogin continues login deferred by DeferLogin option. It is an error to call
// this method when DeferLogin option was not passed to New.
func (c *Chrome) ContinueLogin(ctx context.Context) error {
	if !c.loginPending {
		return errors.New("ContinueLogin can be called once after DeferLogin option is used")
	}
	c.loginPending = false

	if err := login.LogIn(ctx, &c.cfg, c.sess); err == login.ErrNeedNewSession {
		// Restart session.
		if err := c.Reconnect(ctx); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}

// FinishUserLogin handles the necessary steps after a successful login (e.g. during SAML login tests).
func (c *Chrome) FinishUserLogin(ctx context.Context) error {
	return login.FinishUserLogin(ctx, &c.cfg, c.sess, nil)
}

// IsTargetAvailable checks if there is any matched target.
func (c *Chrome) IsTargetAvailable(ctx context.Context, tm TargetMatcher) (bool, error) {
	targets, err := c.FindTargets(ctx, tm)
	if err != nil {
		return false, errors.Wrap(err, "failed to get targets")
	}
	return len(targets) != 0, nil
}

// StartTracing starts trace events collection for the selected categories. Android
// categories must be prefixed with "disabled-by-default-android ", e.g. for the
// gfx category, use "disabled-by-default-android gfx", including the space.
// Note: StopTracing should be called even if StartTracing returns an error.
// Sometimes, the request to start tracing reaches the browser process, but there
// is a timeout while waiting for the reply.
func (c *Chrome) StartTracing(ctx context.Context, categories []string, opts ...cdputil.TraceOption) error {
	return c.sess.StartTracing(ctx, categories, opts...)
}

// StartSystemTracing starts trace events collection from the system tracing
// service using the marshaled binary protobuf trace config.
// Note: StopTracing should be called even if StartTracing returns an error.
// Sometimes, the request to start tracing reaches the browser process, but there
// is a timeout while waiting for the reply.
func (c *Chrome) StartSystemTracing(ctx context.Context, perfettoConfig []byte) error {
	return c.sess.StartSystemTracing(ctx, perfettoConfig)
}

// StopTracing stops trace collection and returns the collected trace events.
func (c *Chrome) StopTracing(ctx context.Context) (*perfetto_proto.Trace, error) {
	return c.sess.StopTracing(ctx)
}

// SaveTraceToFile marshals the given trace into a binary protobuf and saves it
// to a gzip archive at the specified path.
func SaveTraceToFile(ctx context.Context, trace *perfetto_proto.Trace, path string) error {
	data, err := proto.Marshal(trace)
	if err != nil {
		return errors.Wrap(err, "could not marshal trace to binary")
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return errors.Wrap(err, "could not open file")
	}
	defer func() {
		if err := file.Close(); err != nil {
			testing.ContextLog(ctx, "Failed to close file: ", err)
		}
	}()

	writer := gzip.NewWriter(file)
	defer func() {
		if err := writer.Close(); err != nil {
			testing.ContextLog(ctx, "Failed to close gzip writer: ", err)
		}
	}()

	if _, err := writer.Write(data); err != nil {
		return errors.Wrap(err, "could not write the data")
	}

	if err := writer.Flush(); err != nil {
		return errors.Wrap(err, "could not flush the gzip writer")
	}

	return nil
}

// saveMinidumpsWithoutCrash saves minidump snapshots of the browser and its
// related processes to the output directory.
// Minidump snapshots are useful on debugging Chrome hang issues for example.
func saveMinidumpsWithoutCrash(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("output directory unavailable in context")
	}

	dir := filepath.Join(outDir, "chrome_diagnosis")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	matchers := []minidump.Matcher{
		// Login timeout is often caused by TPM slowness.
		minidump.MatchByName("chapsd", "cryptohome", "cryptohomed", "session_manager", "tcsd"),
	}
	if proc, err := ashproc.Root(); err == nil {
		matchers = append(matchers, minidump.MatchByPID(int32(proc.Pid)))
	}

	minidump.SaveWithoutCrash(ctx, dir, matchers...)
	return nil
}

// saveChromeLog writes out the current chrome log to the output directory of the context.
// This should only be necessary if something fails during the creation of the chrome instance.
// Otherwise, Chrome.Close() should handle this.
func saveChromeLog(ctx context.Context, logFilename string) error {
	failLogMarker := logsaver.NewMarkerNoOffset(logFilename)
	if outDir, ok := testing.ContextOutDir(ctx); ok {
		if err := failLogMarker.Save(filepath.Join(outDir, filepath.Base(logFilename))); err != nil {
			testing.ContextLog(ctx, "Failed to save the entire log: ", err)
			return err
		}
	} else {
		testing.ContextLog(ctx, "No output directory exists, not saving log file")
	}
	return nil
}

// checkInternetConnectivityInBackground doing network diagnostics to debug DUT connection issues.
// It using VerifyInternetConnectivity to determine whether the device can access the network.
func checkInternetConnectivityInBackground(ctx context.Context, timeout time.Duration) {
	go func() {
		checkInternetConnectivityStart := time.Now()

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if err := ping.VerifyInternetConnectivity(ctx, timeout); err != nil {
				return errors.Wrap(err, "DUT no internet connectivity")
			}

			duration := time.Now().Sub(checkInternetConnectivityStart)
			testing.ContextLog(ctx, "DUT network verification finished in: ", duration)
			return nil
		}, &testing.PollOptions{Timeout: timeout}); err != nil {
			testing.ContextLog(ctx, "Timeout while checking internet connectivity: ", err)
		}
	}()
}
