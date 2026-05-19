// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mafredri/cdp/rpcc"
	"golang.org/x/exp/slices"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	uiperf "go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/disk"
	"go.chromium.org/tast-tests/cros/local/graphics/modetest"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/perfutil"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	ashTastBootTimeLogin2                                 = "Ash.Tast.BootTime.Login2"
	ashTastArcUIAvailableAfterLoginDuration               = "Ash.Tast.ArcUiAvailableAfterLogin.Duration"
	arcTastUIAvailableTimeDelta                           = "Arc.Tast.UiAvailable.TimeDelta"
	bootTimeLogin2                                        = "BootTime.Login2"
	bootTimeLogin3                                        = "BootTime.Login3"
	uptimeLogoutToUIStopAfterLogout                       = "Uptime.LogoutToUIStopAfterLogout"
	uptimeUIStopToProcessesTerminatedAfterLogout          = "Uptime.UIStopToProcessesTerminatedAfterLogout"
	uptimeOtherProcessesTerminatedToChromeExecAfterLogout = "Uptime.OtherProcessesTerminatedToChromeExecAfterLogout"
	uptimeChromeExecToLoginPromptVisibleAfterLogout       = "Uptime.ChromeExecToLoginPromptVisibleAfterLogout"
	uptimeLogout                                          = "Uptime.Logout"
	uptimeLoginPromptSetupTimeAfterLogout                 = "Uptime.LoginPromptSetupTimeAfterLogout"
	uptimeLogoutToLoginPromptVisible                      = "Uptime.LogoutToLoginPromptVisible"
	loginPerfTraceConfigFileName                          = "login_perf_trace_config.pbtxt"

	categoryAutoRestore   = "Ash.LoginPerf.AutoRestore."
	categoryManualRestore = "Ash.LoginPerf.ManualRestore."

	metricAllShelfIconsLoaded                = "AllShelfIconsLoaded"
	metricShelfLoginAnimationEnd             = "ShelfLoginAnimationEnd"
	metricTotalDuration                      = "TotalDuration"
	metricPostLoginAnimationDurationPrefix   = "PostLoginAnimation.Duration"
	metricPostLoginAnimationSmoothnessPrefix = "PostLoginAnimation.Smoothness"
	metricPostLoginAnimationJankPrefix       = "PostLoginAnimation.Jank"
	metricDeferredTasksStarted               = "DeferredTasksStarted"

	metricAutoRestoreAllBrowserWindowsCreated   = "Ash.LoginPerf.AutoRestore.AllBrowserWindowsCreated"
	metricAutoRestoreAllBrowserWindowsShown     = "Ash.LoginPerf.AutoRestore.AllBrowserWindowsShown"
	metricAutoRestoreAllBrowserWindowsPresented = "Ash.LoginPerf.AutoRestore.AllBrowserWindowsPresented"

	suffixClamshellMode = ".ClamshellMode"
	suffixTabletMode    = ".TabletMode"
)

const (
	// Supported ARC modes
	noarc      = "noarc"
	arcenabled = "arcenabled"

	// Alias for deferring ARC for readability.
	deferARC = "DeferArcActivationUntilUserSessionStartUpTaskCompletion"
	// Alias for deferring ARC with the parameters which force to defer ARC.
	deferARCForceEnabled = "DeferArcActivationUntilUserSessionStartUpTaskCompletion:history_window/0/history_threshold/1"
)

const loginPerfOptinTimeout = 10 * time.Minute

var disableARCSyncOption = chrome.ExtraArgs(arc.DisableSyncFlags()...)

// NOTE: default set of categories is defined in `loginPerfTraceConfigFileName`.
var cmdlineVarTracingExtraCategories = testing.RegisterVarString(
	"ui.LoginPerf.tracing_extra_categories",
	"",
	"Trace event categories to additionally collect, separated by commas",
)

// Use 3 successful runs instead of 10, to reduce tests time.
var cmdlineVarMinSuccessfulRuns = testing.RegisterVarString(
	"ui.LoginPerf.runs",
	"3",
	"The number of minimum successful runs.",
)

// loginPerfTestParam is a set of parameters for the login perf test.
// The baseline parameters are:
//   - windows: 8
//   - arcMode: arcenabled
//   - tabletMode: false
//   - autoSessionRestore: true
type loginPerfTestParam struct {
	windows            int      // Number of session restored windows.
	arcMode            string   // ARC mode to test.
	tabletMode         bool     // Whether to run the test in tablet mode.
	autoSessionRestore bool     // Whether to restore session automatically.
	disabledFeatures   []string // Controls ash-chrome features to be disabled.
	enabledFeatures    []string // Controls ash-chrome features to be enabled.
}

func init() {
	testing.AddTest(&testing.Test{
		Func: LoginPerf,
		Desc: "Measures performance and UI smoothness of ChromeOS login",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"vincentchiang@google.com",
			"oshima@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		SoftwareDeps: []string{"chrome"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
			ui.GaiaPoolDefaultVarName,
		},
		Data:    []string{"animation.html", "animation.js", loginPerfTraceConfigFileName},
		Timeout: loginPerfOptinTimeout + 25*time.Minute,
		Fixture: fixture.GpuRemoteWatcher,
		Params: []testing.Param{{
			ExtraAttr:         []string{"group:cuj", "cuj_loginperf"},
			ExtraSoftwareDeps: []string{"arc"},
			Val: loginPerfTestParam{
				8,          // windows
				arcenabled, // arcMode
				false,      // tabletMode
				true,       // autoSessionRestore
				[]string{}, // disabledFeatures
				[]string{}, // enabledFeatures
			},
		}, {
			Name:      "noarc_2windows",
			ExtraAttr: []string{"group:cuj", "cuj_loginperf"},
			Val: loginPerfTestParam{
				2,          // windows
				noarc,      // arcMode
				false,      // tabletMode
				true,       // autoSessionRestore
				[]string{}, // disabledFeatures
				[]string{}, // enabledFeatures
			},
		}, {
			Name:      "noarc",
			ExtraAttr: []string{"group:cuj", "cuj_loginperf"},
			Val: loginPerfTestParam{
				8,          // windows
				noarc,      // arcMode
				false,      // tabletMode
				true,       // autoSessionRestore
				[]string{}, // disabledFeatures
				[]string{}, // enabledFeatures
			},
		}, {
			Name:              "2windows",
			ExtraAttr:         []string{"group:cuj", "cuj_loginperf"},
			ExtraSoftwareDeps: []string{"arc"},
			Val: loginPerfTestParam{
				2,          // windows
				arcenabled, // arcMode
				false,      // tabletMode
				true,       // autoSessionRestore
				[]string{}, // disabledFeatures
				[]string{}, // enabledFeatures
			},
		}, {
			Name:              "tablet",
			ExtraAttr:         []string{"group:cuj", "cuj_loginperf"},
			ExtraSoftwareDeps: []string{"arc"},
			Val: loginPerfTestParam{
				8,          // windows
				arcenabled, // arcMode
				true,       // tabletMode
				true,       // autoSessionRestore
				[]string{}, // disabledFeatures
				[]string{}, // enabledFeatures
			},
		}, {
			Name:              "manual_restore",
			ExtraAttr:         []string{"group:cuj", "cuj_loginperf"},
			ExtraSoftwareDeps: []string{"arc"},
			Val: loginPerfTestParam{
				8,          // windows
				arcenabled, // arcMode
				false,      // tabletMode
				false,      // autoSessionRestore
				[]string{}, // disabledFeatures
				[]string{}, // enabledFeatures
			},
		}, {
			// TODO(b/418724317): Remove from lab after BSM slows down login is fixed.
			Name: "noarc_2windows_bsm",
			Val: loginPerfTestParam{
				2,          // windows
				noarc,      // arcMode
				false,      // tabletMode
				true,       // autoSessionRestore
				[]string{}, // disabledFeatures
				[]string{"CrosBatterySaver", "CrosBatterySaverAlwaysOn"}, // enabledFeatures
			},
		}, {
			// TODO(b/418724317): Remove from lab after BSM slows down login is fixed.
			Name:              "2windows_bsm",
			ExtraSoftwareDeps: []string{"arc"},
			Val: loginPerfTestParam{
				2,          // windows
				arcenabled, // arcMode
				false,      // tabletMode
				true,       // autoSessionRestore
				[]string{}, // disabledFeatures
				[]string{"CrosBatterySaver", "CrosBatterySaverAlwaysOn"}, // enabledFeatures
			},
		}},
	})
}

// loginPerfStartToLoginScreen starts Chrome to the login screen.
func loginPerfStartToLoginScreen(
	ctx context.Context,
	signinExtManifestKey string,
	param loginPerfTestParam,
) (cr *chrome.Chrome, retErr error) {
	// chrome.NoLogin() and chrome.KeepState() are needed to show the login
	// screen with a user pod (instead of the OOBE login screen).
	options := []chrome.Option{
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.LoadSigninProfileExtension(signinExtManifestKey),
		// Disable OOBE testing API for measurement runs because it can affect the performance. See b/354825581.
		chrome.DisableOOBETestAPI(),
		chrome.EnableRestoreTabs(),
		chrome.SkipForceOnlineSignInForTesting(),
		chrome.EnableWebAppInstall(),
		chrome.HideCrashRestoreBubble(), // Ignore possible incomplete shutdown.
		// Disable whats-new page. See crbug.com/1271436.
		chrome.DisableFeatures("ChromeWhatsNewUI"),
		chrome.ExtraArgs("--disable-sync"),
		chrome.DisableFeatures(param.disabledFeatures...),
		chrome.EnableFeatures(param.enabledFeatures...),
	}
	// Disable the ARC deferring feature, which is enabled by default in production.
	// Even though it can improve login performance, the behavior depends on ARC usage during sessions.
	// To get consistent results, we intentionally disable the feature for measurement runs.
	// But we skip disabling the feature if it's enabled explicitly because disabling a feature precedes enabling it.
	shouldDeferARC := slices.Contains(param.enabledFeatures, deferARC) || slices.Contains(param.enabledFeatures, deferARCForceEnabled)
	if !shouldDeferARC {
		options = append(options, chrome.DisableFeatures(deferARC))
	}

	// Append additional ARC options.
	switch param.arcMode {
	case noarc:
	case arcenabled:
		options = append(options,
			chrome.ARCSupported(),
			chrome.DisableFeatures("ArcExternalStorageAccess"),
			disableARCSyncOption,
		)
	default:
		panic(fmt.Sprintf("Unknown arcMode value=%v", param.arcMode))
	}

	// Drop caches to simulate cold boot.
	if err := disk.DropCaches(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to drop caches")
	}
	testing.ContextLog(ctx, "loginPerfStartToLoginScreen: File caches dropped")

	cr, err := chrome.New(ctx, options...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start chrome")
	}
	defer func() {
		if retErr != nil {
			if cr == nil {
				testing.ContextLog(ctx, "loginPerfStartToLoginScreen: Chrome is unexpectedly nil, will not close")
			} else {
				cr.Close(ctx)
				cr = nil
			}
		}
	}()

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok || outDir == "" {
		return nil, errors.New("failed to get the out directory")
	}

	tLoginConn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "creating login test api connection failed")
	}
	defer faillog.DumpUITreeOnError(ctx, outDir, func() bool { return retErr != nil }, tLoginConn)

	if err := ash.SetTabletModeEnabled(ctx, tLoginConn, param.tabletMode); err != nil {
		return nil, errors.Wrapf(err, "failed to set tablet mode %v", param.tabletMode)
	}

	// Wait for the login screen to be ready for password entry.
	if st, err := lockscreen.WaitState(
		ctx,
		tLoginConn,
		func(st lockscreen.State) bool { return st.ReadyForPassword },
		30*time.Second); err != nil {
		return nil, errors.Wrapf(err, "failed waiting for the login screen to be ready for password entry: last state: %+v", st)
	}

	testing.ContextLog(ctx, "Sleeping for 5 seconds before trying to log in")
	// GoBigSleepLint: This sleep is a test parameter.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		return nil, errors.Wrap(err, "failed to sleep at the login screen for 5 seconds")
	}

	return cr, nil
}

// loginPerfDoLogin logs in and waits for animations to finish.
func loginPerfDoLogin(
	ctx context.Context,
	cr *chrome.Chrome,
	credentials chrome.Creds,
) (retErr error) {
	outdir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("no output directory exists")
	}
	tLoginConn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "creating login test API connection failed")
	}
	defer faillog.DumpUITreeOnError(ctx, outdir, func() bool { return retErr != nil }, tLoginConn)

	// TODO(crbug/1109381): the password field isn't actually ready just yet when WaitState returns.
	// This causes it to miss some of the keyboard input, so the password will be wrong.
	// We can check in the UI for the password field to exist, which seems to be a good enough indicator that
	// the field is ready for keyboard input.
	if err := lockscreen.WaitForPasswordField(ctx, tLoginConn, credentials.User, 15*time.Second); err != nil {
		return errors.Wrap(err, "password text field did not appear in the ui")
	}

	// Reset automation as we no longer need it. Otherwise, automation requires extra work (e.g.
	// updating accessibility tree) and makes it hard to investigate real performance issues.
	if err := tLoginConn.ResetAutomation(ctx); err != nil {
		return errors.Wrap(err, "failed to reset automation feature")
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer kb.Close(ctx)

	if err := kb.Type(ctx, credentials.Pass+"\n"); err != nil {
		return errors.Wrap(err, "entering password failed")
	}

	// Check if the login was successful using the API.
	if st, err := lockscreen.WaitState(
		ctx,
		tLoginConn,
		func(st lockscreen.State) bool { return st.LoggedIn },
		30*time.Second,
	); err != nil {
		return errors.Wrapf(err, "failed waiting to log in: last state: %+v", st)
	}

	return nil
}

// waitForLoginAnimationEnd waits until the post login animation is complete.
func waitForLoginAnimationEnd(ctx context.Context, tconn *chrome.TestConn) error {
	if err := tconn.Call(ctx, nil, "tast.promisify(chrome.autotestPrivate.waitForLoginAnimationEnd)"); err != nil {
		return errors.Wrap(err, "failed to call waitForLoginAnimationEnd")
	}
	return nil
}

// maxHistogramValue calculates the estimated maximum of the histogram values.
// At is an error when there are no data points.
func maxHistogramValue(h *histogram.Histogram) (float64, error) {
	if h.TotalCount() == 0 {
		return 0, errors.New("no histogram data")
	}
	var max int64 = math.MinInt64
	for _, b := range h.Buckets {
		if b.Count > 0 && max < b.Max {
			max = b.Max
		}
	}
	return float64(max), nil
}

func reportMaxHistogramValue(
	ctx context.Context,
	hist *histogram.Histogram,
	unit string,
	pv *perfutil.Values,
) error {
	value, err := maxHistogramValue(hist)
	if err != nil {
		return errors.Wrapf(err, "failed to get %s data", hist.Name)
	}
	pv.Append(perf.Metric{
		Name:      hist.Name,
		Unit:      unit,
		Direction: perf.SmallerIsBetter,
	}, value)
	return nil
}

// logout is a proxy to chrome.autotestPrivate.logout
func logout(ctx context.Context, cr *chrome.Chrome) error {
	if cr == nil {
		testing.ContextLog(ctx, "Sign out: skipped (no chrome)")
		return nil
	}
	testing.ContextLog(ctx, "Sign out: started")

	// Limit sign-out attempt to 1 minute (so that we could retry early).
	cleanupContext := ctx
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to test api")
	}
	sm, err := session.NewSessionManager(ctx)
	if err != nil {
		return err
	}
	sw, err := sm.WatchSessionStateChanged(ctx, "stopped")
	if err != nil {
		return errors.Wrap(err, "failed to watch for D-Bus signals")
	}
	defer sw.Close(ctx)

	if err := tconn.Call(ctx, nil, "chrome.autotestPrivate.logout"); err != nil {
		if errors.Is(err, rpcc.ErrConnClosing) {
			testing.ContextLog(ctx, "WARNING: chrome.autotestPrivate.logout failed with: ", err)
		} else {
			return errors.Wrap(err, "failed to run chrome.autotestPrivate.logout()")
		}
	}

	select {
	case <-sw.Signals:
		testing.ContextLog(cleanupContext, "Got SessionStateChanged signal")
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "didn't get SessionStateChanged signal")
	}
	testing.ContextLog(cleanupContext, "Sign out: done")
	return nil
}

// setSessionRestoreSetting opens OS settings and sets the 'Always restore' setting or
// 'Ask every time' depending on `autoRestore`. In order to avoid possible noise when collecting the
// browser login time performance at restoring time, this function also makes sure to close the OS
// settings app before returning.
func setSessionRestoreSetting(ctx context.Context, tconn *chrome.TestConn, autoRestore bool) error {
	settings, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.SystemPreferences)
	if err != nil {
		return errors.Wrap(err, "failed to launch system preferences page")
	}

	restoreButtonReg := regexp.MustCompile("(Restore session on startup|Welcome Recap)")
	restoreButtonNode := nodewith.NameRegex(restoreButtonReg).Role(role.ComboBoxSelect)

	alwaysRestoreOptionNode := nodewith.NameRegex(regexp.MustCompile("Always (restore|open)")).Role(role.MenuListOption)
	askEveryTimeOptionNode := nodewith.NameRegex(regexp.MustCompile("Ask every time")).Role(role.MenuListOption)

	optionNode := alwaysRestoreOptionNode
	if !autoRestore {
		optionNode = askEveryTimeOptionNode
	}

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("set session restore setting in the settings app",
		ui.WaitUntilExists(restoreButtonNode),
		ui.DoDefault(restoreButtonNode),
		ui.WaitUntilExists(optionNode),
		ui.DoDefault(optionNode),
	)(ctx); err != nil {
		return err
	}

	if err := settings.Close(ctx); err != nil {
		return err
	}

	// TODO(crbug.com/1314785)
	// According to the PRD of Full Restore go/chrome-os-full-restore-dd,
	// it uses a throttle of 2.5s to save the app launching and window
	// state information to the backend. Therefore, sleep 3 seconds here.
	// GoBigSleepLint: crbug.com/1314785
	return testing.Sleep(ctx, 3*time.Second)
}

func cancelSessionRestoreOnWelcomeRecap(ctx context.Context, tconn *chrome.TestConn) error {
	noThanksButtonNode := nodewith.ClassName("PillButton").Name("No thanks")

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("click \"No thanks\" option on the Welcome Recap screen",
		ui.WithTimeout(5*time.Second).WaitUntilExists(noThanksButtonNode),
		ui.DoDefault(noThanksButtonNode),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"No thanks\" option on Welcome Recap screen")
	}

	return nil
}

func restoreSessionOnWelcomeRecap(ctx context.Context, tconn *chrome.TestConn) error {
	openButtonNode := nodewith.ClassName("PillButton").Name("Open")

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("click \"Open\" option on the Welcome Recap screen",
		ui.WithTimeout(5*time.Second).WaitUntilExists(openButtonNode),
		ui.DoDefault(openButtonNode),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Open\" option on Welcome Recap screen")
	}

	return nil
}

// initializeLoginPerfTest initializes user session state that will be restored
// in subsequent test runs.
func initializeLoginPerfTest(ctx context.Context,
	signinExtManifestKey string,
	param loginPerfTestParam,
	animationPageURL string,
) (
	retCreds chrome.Creds,
	retErr error,
) {
	// Reserve some time to dump ARC state if apps fail to install.
	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	loginPool := dma.CredsFromPool(ui.GaiaPoolDefaultVarName)
	options := []chrome.Option{
		chrome.GAIALoginPool(loginPool),
		chrome.EnableRestoreTabs(),
		chrome.SkipForceOnlineSignInForTesting(),
		chrome.EnableWebAppInstall(),
		// Disable whats-new page. See crbug.com/1271436.
		chrome.DisableFeatures("ChromeWhatsNewUI"),
		// Disable deferring ARC.
		chrome.DisableFeatures(deferARC),
		// --disable-sync disables test account info sync, eg. Wi-Fi credentials,
		// so that each test run does not remember info from last test run.
		chrome.ExtraArgs("--disable-sync"),
	}

	isARCEnabled := arc.Supported() && param.arcMode == arcenabled
	// Only enable arc if it's supported and enabled.
	if isARCEnabled {
		// We enable ARC initially to fully initialize it.
		options = append(options, chrome.ARCSupported(), disableARCSyncOption)
	}

	cr, err := chrome.New(ctx, options...)
	if err != nil {
		return chrome.Creds{}, errors.Wrap(err, "chrome login failed")
	}
	defer func() {
		// cr is not valid after logout.
		if cr == nil {
			testing.ContextLog(cleanupContext, "initializeLoginPerfTest: Chrome is nil, will not close")
		} else {
			cr.Close(cleanupContext)
			cr = nil
		}
	}()

	creds := cr.Creds()

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok || outDir == "" {
		return chrome.Creds{}, errors.New("failed to get the out directory")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return chrome.Creds{}, errors.Wrap(err, "failed to connect to test api")
	}
	defer faillog.DumpUITreeOnError(ctx, outDir, func() bool { return retErr != nil }, tconn)

	testing.ContextLog(ctx, "Opting into Play Store")
	if isARCEnabled {
		if err := optin.PerformWithTimeout(ctx, cr, tconn, loginPerfOptinTimeout); err != nil {
			return chrome.Creds{}, errors.Wrap(err, "failed to optin to Play Store")
		}
		testing.ContextLog(ctx, "Optin finished")

		// Wait for ARC++ aps to download and initialize.
		testing.ContextLog(ctx, "Initialize: Waiting for arc to install initial apps")
		histogram, metricsErr := metrics.WaitForHistogram(
			ctx,
			tconn,
			"Ash.ArcAppInitialAppsInstallDuration",
			10*time.Minute,
		)
		// While waiting we could hit the |ctx| timeout so we are
		// using the |cleanupContext| to avoid the |ctx|.
		if metricsErr != nil {
			if err := optin.DumpLogCat(cleanupContext, "error"); err != nil {
				testing.ContextLog(cleanupContext,
					"WARNING: Failed to dump logcat: ", err)
			}
			return chrome.Creds{}, errors.Wrap(metricsErr,
				"failed to wait until ARC initial "+
					"apps installed")
		}
		testing.ContextLog(ctx, "Initialize: "+
			"Ash.ArcAppInitialAppsInstallDuration histogram=",
			histogram)

		testing.ContextLog(ctx, "Initialize: Waiting 30 seconds to allow time for session to fully initialize")
		// GoBigSleepLint: Give session time to settle.
		if err := testing.Sleep(ctx, 30*time.Second); err != nil {
			return chrome.Creds{}, errors.Wrap(err, "failed to run initial wait time")
		}
	} else {
		testing.ContextLog(ctx, "ARC++ is not supported. Running test without ARC")

		testing.ContextLog(ctx, "Initialize: Waiting 1 minute to allow time for session to fully initialize")
		// GoBigSleepLint: Give session time to settle.
		if err := testing.Sleep(ctx, time.Minute); err != nil {
			return chrome.Creds{}, errors.Wrap(err, "failed to run initial wait time")
		}
	}

	if err := setSessionRestoreSetting(ctx, tconn, param.autoSessionRestore); err != nil {
		return chrome.Creds{}, errors.Wrap(err, "failed to adjust always restore settings")
	}

	if err := logout(ctx, cr); err != nil {
		return creds, errors.Wrap(err, "failed to log out")
	}
	cr = nil

	testing.ContextLog(ctx, "Create new windows: Sign in to create new windows")
	// Log in and log out to create a user pod on the login screen and required number of windows in session.
	err = func() error {
		// We do not need ARC to create Chrome windows.
		modifiedParam := param
		modifiedParam.arcMode = noarc
		modifiedParam.tabletMode = false
		cr, err := loginPerfStartToLoginScreen(ctx, signinExtManifestKey, modifiedParam)
		if err != nil {
			return err
		}
		defer func() {
			// cr is not valid after logout.
			if cr == nil {
				testing.ContextLog(ctx, "create windows: Chrome is nil, will not close")
			} else {
				cr.Close(ctx)
				cr = nil
			}
		}()

		err = loginPerfDoLogin(ctx, cr, creds)
		if err != nil {
			return err
		}

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to connect to test api")
		}

		// Wait until the session settles.
		if err := waitForLoginAnimationEnd(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to await login animation")
		}

		// If session restore is manual, explicitly discard the previous session to start new one.
		if !param.autoSessionRestore {
			if err := cancelSessionRestoreOnWelcomeRecap(ctx, tconn); err != nil {
				return errors.Wrap(err, "failed to cancel session restore")
			}
		}

		visible, err := ash.CountVisibleWindows(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to count browser windows")
		}
		if visible != 0 && visible != 1 {
			return errors.Errorf("unexpected number of visible windows before creating new ones: expected %d, found %d", 0, visible)
		}
		testing.ContextLogf(ctx, "Before creating windows: visible=%d", visible)
		if err := ash.CreateWindows(ctx, tconn, cr, animationPageURL, param.windows); err != nil {
			return errors.Wrap(err, "failed to create browser windows")
		}

		testing.ContextLog(ctx, "Sign out: sleep for 20 seconds to let session settle")
		// GoBigSleepLint: Give session time to settle.
		if err := testing.Sleep(ctx, 20*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep for 20 seconds")
		}
		if err := logout(ctx, cr); err != nil {
			return errors.Wrap(err, "failed to log out")
		}
		cr = nil
		return nil
	}()
	if err != nil {
		return creds, errors.Wrap(err, "failed to create new browser windows")
	}

	return creds, nil
}

type loginPerfTracingConfig struct {
	configFilePath  string
	extraCategories []string
	resultOutDir    string
	resultFileName  string
}

// measureLoginPerformance is the actual test flow that could executed multiple times to get average data or generate tracing. Tracing is disabled if `tracingConfig` is nil.
func measureLoginPerformance(
	ctx context.Context,
	signinExtManifestKey string,
	creds chrome.Creds,
	param loginPerfTestParam,
	tracingConfig *loginPerfTracingConfig,
) (
	*chrome.Chrome,
	[]*histogram.Histogram,
	map[perf.Metric][]float64,
	error,
) {
	var expectedHistograms []string
	{
		displCount, err := modetest.NumberOfOutputsConnected(ctx)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "failed to get connected display count")
		}
		expectedHistograms = constructExpectedHistograms(param, displCount > 0)
	}

	cr, err := loginPerfStartToLoginScreen(ctx, signinExtManifestKey, param)
	if err != nil {
		return cr, nil, nil, errors.Wrap(err, "failed to start to login screen")
	}

	// The actual test function
	testFunc := func(ctx context.Context, stopTracing func(ctx context.Context) error) error {
		// Limit each test run to 2 minutes (so that we could retry early).
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		out, err := exec.Command("ps", "aux").Output()
		testing.ContextLog(ctx, "ps aux result:")
		testing.ContextLog(ctx, string(out))
		if err != nil {
			return errors.Wrap(err, "ps aux failed")
		}

		err = loginPerfDoLogin(ctx, cr, creds)
		if err != nil {
			return errors.Wrap(err, "failed to log in")
		}

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to connect to test api")
		}

		if err := waitForLoginAnimationEnd(ctx, tconn); err != nil {
			return errors.Wrap(err, "failed to await login animation")
		}

		sleepTime := 10 * time.Second
		if stopTracing != nil {
			// Stopping tracing before the full 10 seconds have
			// elapsed reduces the trace file size by approximately
			// 20% (from ~10MB to ~8MB) per file.
			testing.ContextLog(ctx, "Sleep for 5 seconds to wait for last metrics before stopping tracing")
			// GoBigSleepLint: Controls the tracing time.
			if err := testing.Sleep(ctx, 5*time.Second); err != nil {
				return errors.Wrap(err, "failed to sleep for 5 seconds")
			}
			if err := stopTracing(ctx); err != nil {
				return errors.Wrap(err, "failed to stop tracing")
			}
			sleepTime = 5 * time.Second
		}
		testing.ContextLogf(ctx, "Sleep for %f seconds to let session settle and save restore data", sleepTime.Seconds())
		// GoBigSleepLint: Give session time to settle and save restore data.
		if err := testing.Sleep(ctx, sleepTime); err != nil {
			return errors.Wrapf(err, "failed to sleep for %f seconds", sleepTime.Seconds())
		}
		// boot metrics have likely been already reported and wiped
		// away while we were waiting for the cpu to cool down before
		// running login. Force them to be reported again.
		if err := testexec.CommandContext(
			ctx,
			"sh",
			"-c",
			"touch /tmp/stats-logout-started.written && /usr/share/cros/init/send-uptime-metrics",
		).Run(testexec.DumpLogOnError); err != nil {
			return errors.Wrap(err, "failed to force send-uptime-metrics to be reported again")
		}

		// If session restore is manual, restore the previous session.
		if !param.autoSessionRestore {
			if err := restoreSessionOnWelcomeRecap(ctx, tconn); err != nil {
				return errors.Wrap(err, "failed to restore previous session")
			}
		}

		// Ash.LoginAnimation.Duration.* are reported only a few frames
		// after the animation end. Trigger next system UI animation
		// to push the metrics through.
		if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
			return errors.Wrap(err, "failed to enter the overview mode")
		}
		return nil
	}

	// Full test run, instantiate recorders.
	tLoginConn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return cr, nil, nil, errors.Wrap(err, "creating login test api connection failed")
	}
	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Initialize CUJ recording.
	cujRecorder, err := cujrecorder.NewRecorder(
		ctx,
		tLoginConn,
		cr,
		nil,
		cujrecorder.RecorderOptions{RecordLoginEvents: true},
	)
	if err != nil {
		return cr, nil, nil, errors.Wrap(err, "failed to create a CUJ recorder")
	}
	defer cujRecorder.Close(closeCtx)

	for _, metricConfig := range [][]cujrecorder.MetricConfig{
		cujrecorder.AshCommonMetricConfigs(),
		cujrecorder.BrowserCommonMetricConfigs(),
		cujrecorder.AnyChromeCommonMetricConfigs(),
	} {
		if err := cujRecorder.AddCollectedMetrics(metricConfig...); err != nil {
			return cr, nil, nil, errors.Wrap(err, "failed to add recorded metrics")
		}
	}

	var histograms []*histogram.Histogram

	// CUJ TPS metrics recording wrapper
	cujFunc := func(ctx context.Context) error {
		// Sync filesystem to make sure that we do not have pending data to save.
		if err := testexec.CommandContext(ctx, "sync").Run(testexec.DumpLogOnError); err != nil {
			return errors.Wrap(err, "failed to sync DUT")
		}
		var stopTracingCallback func(ctx context.Context) error
		if tracingConfig != nil {
			// See go/trace-in-cuj-tests about rules for tracing.
			if err := cujRecorder.StartTracingWithExtraCategories(ctx,
				tracingConfig.resultOutDir,
				tracingConfig.resultFileName,
				tracingConfig.configFilePath,
				tracingConfig.extraCategories...); err != nil {
				return errors.Wrap(err, "failed to start tracing")
			}
			stopTracingCallback = cujRecorder.StopTracing
		}

		var err error
		histograms, err = metrics.RunAndWaitAll(
			ctx,
			tLoginConn,
			4*time.Minute,
			func(ctx context.Context) error {
				return testFunc(ctx, stopTracingCallback)
			},
			expectedHistograms...,
		)
		if err != nil {
			return err
		}

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to connect to Ash test api")
		}
		visible, err := ash.CountVisibleWindows(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to count browser windows")
		}
		expected := param.windows
		if visible != expected && visible != expected+1 {
			return errors.Errorf("unexpected number of visible windows: expected %d, found %d", expected, visible)
		}

		return nil
	}
	if err := cujRecorder.Run(ctx, cujFunc); err != nil {
		return cr, nil, nil, errors.Wrap(err, "failed to run the test scenario")
	}
	tpsValues := perf.NewValues()
	if err := cujRecorder.Record(ctx, tpsValues); err != nil {
		return cr, nil, nil, errors.Wrap(err, "failed to collect the data from the recorder")
	}
	if err := cujRecorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}

	return cr, histograms, tpsValues.GetValues(), err
}

func constructExpectedHistograms(param loginPerfTestParam, hasDisplay bool) []string {
	suffix := suffixClamshellMode
	if param.tabletMode {
		suffix = suffixTabletMode
	}

	var category string
	if param.autoSessionRestore {
		category = categoryAutoRestore
	} else {
		category = categoryManualRestore
	}

	ret := []string{
		ashTastBootTimeLogin2,
		bootTimeLogin2,
		bootTimeLogin3,
		uptimeLogoutToUIStopAfterLogout,
		uptimeUIStopToProcessesTerminatedAfterLogout,
		uptimeOtherProcessesTerminatedToChromeExecAfterLogout,
		uptimeChromeExecToLoginPromptVisibleAfterLogout,
		uptimeLogout,
		uptimeLoginPromptSetupTimeAfterLogout,
		uptimeLogoutToLoginPromptVisible,

		category + metricAllShelfIconsLoaded,
		category + metricDeferredTasksStarted,
	}
	// Following histograms are only collected when session restore is automatic.
	if param.autoSessionRestore {
		ret = append(ret, metricAutoRestoreAllBrowserWindowsCreated, metricAutoRestoreAllBrowserWindowsShown)
	}
	// Following histograms are only collected when the DUT is connected to the display.
	if hasDisplay {
		ret = append(ret,
			category+metricShelfLoginAnimationEnd,
			category+metricTotalDuration,
			category+metricPostLoginAnimationDurationPrefix+suffix,
			category+metricPostLoginAnimationSmoothnessPrefix+suffix,
			category+metricPostLoginAnimationJankPrefix+suffix,
		)
		// Following histograms are only collected when session restore is automatic.
		if param.autoSessionRestore {
			ret = append(ret, metricAutoRestoreAllBrowserWindowsPresented)
		}
	}
	if param.arcMode != noarc {
		ret = append(ret,
			ashTastArcUIAvailableAfterLoginDuration,
			arcTastUIAvailableTimeDelta,
		)
	}

	return ret
}

// storeHistograms transforms []*histogram.Histogram test results into perf Values to report.
func storeHistograms(
	ctx context.Context,
	hists []*histogram.Histogram,
	pv *perfutil.Values,
) error {
	for _, hist := range hists {
		switch hist.Name {
		case
			ashTastBootTimeLogin2,
			bootTimeLogin2,
			bootTimeLogin3,
			ashTastArcUIAvailableAfterLoginDuration,
			arcTastUIAvailableTimeDelta,
			uptimeLogoutToUIStopAfterLogout,
			uptimeUIStopToProcessesTerminatedAfterLogout,
			uptimeOtherProcessesTerminatedToChromeExecAfterLogout,
			uptimeChromeExecToLoginPromptVisibleAfterLogout,
			uptimeLogout,
			uptimeLoginPromptSetupTimeAfterLogout,
			uptimeLogoutToLoginPromptVisible:
			reportMaxHistogramValue(ctx, hist, "millisecond", pv)

		case
			metricAutoRestoreAllBrowserWindowsCreated,
			metricAutoRestoreAllBrowserWindowsShown,
			metricAutoRestoreAllBrowserWindowsPresented,
			categoryAutoRestore + metricAllShelfIconsLoaded,
			categoryAutoRestore + metricShelfLoginAnimationEnd,
			categoryAutoRestore + metricTotalDuration,
			categoryAutoRestore + metricPostLoginAnimationDurationPrefix + suffixClamshellMode,
			categoryAutoRestore + metricPostLoginAnimationDurationPrefix + suffixTabletMode,
			categoryAutoRestore + metricDeferredTasksStarted,
			categoryManualRestore + metricAllShelfIconsLoaded,
			categoryManualRestore + metricShelfLoginAnimationEnd,
			categoryManualRestore + metricTotalDuration,
			categoryManualRestore + metricPostLoginAnimationDurationPrefix + suffixClamshellMode,
			categoryManualRestore + metricPostLoginAnimationDurationPrefix + suffixTabletMode,
			categoryManualRestore + metricDeferredTasksStarted:
			storeHistogramMeanValue(ctx, hist, "ms", perf.SmallerIsBetter, pv)

		case
			categoryAutoRestore + metricPostLoginAnimationSmoothnessPrefix + suffixClamshellMode,
			categoryAutoRestore + metricPostLoginAnimationSmoothnessPrefix + suffixTabletMode,
			categoryManualRestore + metricPostLoginAnimationSmoothnessPrefix + suffixClamshellMode,
			categoryManualRestore + metricPostLoginAnimationSmoothnessPrefix + suffixTabletMode:
			storeHistogramMeanValue(ctx, hist, "percent", perf.BiggerIsBetter, pv)

		case
			categoryAutoRestore + metricPostLoginAnimationJankPrefix + suffixClamshellMode,
			categoryAutoRestore + metricPostLoginAnimationJankPrefix + suffixTabletMode,
			categoryManualRestore + metricPostLoginAnimationJankPrefix + suffixClamshellMode,
			categoryManualRestore + metricPostLoginAnimationJankPrefix + suffixTabletMode:
			storeHistogramMeanValue(ctx, hist, "percent", perf.SmallerIsBetter, pv)

		default:
			return errors.Errorf("unknown histogram %q", hist.Name)
		}
	}
	return nil
}

func storeHistogramMeanValue(
	ctx context.Context,
	hist *histogram.Histogram,
	unit string,
	direction perf.Direction,
	pv *perfutil.Values,
) error {
	value, err := hist.Mean()
	if err != nil {
		return errors.Wrapf(err, "failed to get the mean of %s", hist.Name)
	}
	pv.Append(perf.Metric{
		Name:      hist.Name,
		Unit:      unit,
		Direction: direction,
	}, value)
	return nil
}

func LoginPerf(ctx context.Context, s *testing.State) {
	cuj.WriteMetadataFile(ctx, s.TestName())

	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	signinExtManifestKey := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")
	param := s.Param().(loginPerfTestParam)

	// Run an http server to serve the test contents for accessing from the chrome browsers.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()
	animationPageURL := server.URL + "/animation.html"

	// Log in and log out to create a user pod on the login screen.
	creds, err := initializeLoginPerfTest(
		ctx,
		signinExtManifestKey,
		param,
		animationPageURL,
	)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}

	r := perfutil.NewRunner(nil,
		perfutil.RunnerOptions{
			IgnoreFirstRun:   false,
			DropMinMaxValues: false,
		},
	)
	minRuns, err := strconv.ParseInt(cmdlineVarMinSuccessfulRuns.Value(), 10, 32)
	if err != nil || minRuns <= 0 {
		s.Fatalf("Invalid value for %s: %v", cmdlineVarMinSuccessfulRuns.Name(), err)
	}
	r.SetRunsNumber(perfutil.RunnerCyclesOptions{MaxRuns: int(minRuns) + 2, MinSuccessfulRuns: int(minRuns)})
	s.Logf("Starting test: %s for  %d windows", param.arcMode, param.windows)

	// |cr| is shared between multiple runs, because Chrome connection must to be
	// closed only after histograms are stored.
	var cr *chrome.Chrome

	testName := s.TestName()
	s.Logf("Starting test: %q", testName)

	// Performance run.
	// Metrics are collected and saved to `pv`.
	var allRunErrors []string
	if allRunErrors, err = r.RunMultiple(
		ctx,
		testName,
		uiperf.Run(
			s,
			func(ctx context.Context, name string) ([]*metrics.Histogram, error) {
				// Tracing is disabled for the performance runs.
				var histograms []*metrics.Histogram
				var tpsValues map[perf.Metric][]float64
				var err error
				// Fill in external 'cr'.
				cr, histograms, tpsValues, err = measureLoginPerformance(ctx, signinExtManifestKey, creds, param, nil /*tracingConfig*/)
				r.Values().MergeWithSuffix("", tpsValues)
				return histograms, err
			}),
		func(ctx context.Context, pv *perfutil.Values, hists []*metrics.Histogram) error {
			// Shorten context a bit to allow for cleanup.
			localCloseCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()
			defer func() {
				// cr is not valid after logout.
				if cr == nil {
					s.Log("Subtest cleanup: Chrome is nil, will not close")
				} else {
					cr.Close(localCloseCtx)
					cr = nil
				}
			}()

			if err := storeHistograms(ctx, hists, pv); err != nil {
				return errors.Wrap(err, "storeHistograms failed")
			}
			if err := logout(ctx, cr); err != nil {
				return errors.Wrap(err, "failed to log out")
			}
			cr = nil
			return nil
		}); err != nil {
		s.Fatalf("Failed to run test scenario %s: %v", testName, err)
	}
	if len(allRunErrors) > 0 {
		s.Logf("WARNING: Some of the %s runs ended with failures. All run errors: %v", testName, allRunErrors)
	}

	// Do a tracing run.
	// Values are not stored to the perf results, but only reported to the test log.
	// Tracing run is different from performance run and we need metrics values from the
	// tracing run to analyze the trace.
	// Tracing run errors are logged but do not fail the test.
	tracingValues := perfutil.NewValues(false /*dropMinMax*/)

	var tracingExtraCategories []string
	if cmdlineVarTracingExtraCategories.Value() != "" {
		tracingExtraCategories = strings.Split(cmdlineVarTracingExtraCategories.Value(), ",")
	}
	tracingConfig := &loginPerfTracingConfig{
		configFilePath:  s.DataPath(loginPerfTraceConfigFileName),
		extraCategories: tracingExtraCategories,
		resultOutDir:    s.OutDir(),
		resultFileName:  fmt.Sprintf("%s-trace.data", testName),
	}

	var tracingHistograms []*metrics.Histogram
	var tpsValues map[perf.Metric][]float64

	cr, tracingHistograms, tpsValues, err = measureLoginPerformance(ctx, signinExtManifestKey, creds, param, tracingConfig)
	defer func() {
		// cr is not valid after logout.
		if cr == nil {
			s.Log("tracing cleanup: Chrome is nil, will not close")
		} else {
			cr.Close(closeCtx)
			cr = nil
		}
	}()

	if err != nil {
		s.Logf("WARNING: Failed to run tracing for the test scenario %s-tracing: %s", testName, err)
	} else if err := storeHistograms(ctx, tracingHistograms, tracingValues); err != nil {
		s.Logf("WARNING: Failed to dump tracing histograms for the test scenario %s-tracing: %v", testName, err)
	} else {
		tracingValues.ForEach(func(name string, value []float64) {
			if len(value) != 1 {
				s.Logf("WARNING: %s-tracing: number of %s values is not equal to one: %v", testName, name, value)
			} else {
				s.Logf("%s-tracing %s: %f", testName, name, value[0])
			}
		})
		for metric, values := range tpsValues {
			s.Logf("%s-tracing %s: %v", testName, metric.Name, values)
		}
	}
	if err := logout(ctx, cr); err != nil {
		s.Logf("WARNING: Failed to sign out from the tracing session %s-tracing: %v", testName, err)
	}
	cr = nil

	if err := r.Values().Save(closeCtx, s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}
