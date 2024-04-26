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
	"time"

	"github.com/mafredri/cdp/rpcc"

	"go.chromium.org/tast-tests/cros/common/chrome/histogram"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	uiperf "go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/disk"
	"go.chromium.org/tast-tests/cros/local/graphics"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/perfutil"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	ensureWorkVisibleHistogram                            = "GPU.EnsureWorkVisibleDuration"
	ensureWorkVisibleLowResHistogram                      = "GPU.EnsureWorkVisibleDurationLowRes"
	allBrowserWindowsCreated                              = "Ash.LoginSessionRestore.AllBrowserWindowsCreated"
	allBrowserWindowsShown                                = "Ash.LoginSessionRestore.AllBrowserWindowsShown"
	allBrowserWindowsPresented                            = "Ash.LoginSessionRestore.AllBrowserWindowsPresented"
	allShelfIconsLoaded                                   = "Ash.LoginSessionRestore.AllShelfIconsLoaded"
	shelfLoginAnimationEnd                                = "Ash.LoginSessionRestore.ShelfLoginAnimationEnd"
	ashTastBootTimeLogin2                                 = "Ash.Tast.BootTime.Login2"
	ashTastArcUIAvailableAfterLoginDuration               = "Ash.Tast.ArcUiAvailableAfterLogin.Duration"
	arcTastUIAvailableTimeDelta                           = "Arc.Tast.UiAvailable.TimeDelta"
	bootTimeLogin3                                        = "BootTime.Login3"
	uptimeLogoutToUIStopAfterLogout                       = "Uptime.LogoutToUIStopAfterLogout"
	uptimeUIStopToProcessesTerminatedAfterLogout          = "Uptime.UIStopToProcessesTerminatedAfterLogout"
	uptimeOtherProcessesTerminatedToChromeExecAfterLogout = "Uptime.OtherProcessesTerminatedToChromeExecAfterLogout"
	uptimeChromeExecToLoginPromptVisibleAfterLogout       = "Uptime.ChromeExecToLoginPromptVisibleAfterLogout"
	uptimeLogout                                          = "Uptime.Logout"
	uptimeLoginPromptSetupTimeAfterLogout                 = "Uptime.LoginPromptSetupTimeAfterLogout"
	uptimeLogoutToLoginPromptVisible                      = "Uptime.LogoutToLoginPromptVisible"
	loginPerfTraceConfigFileName                          = "login_perf_trace_config.pbtxt"
)

const (
	// Supported ARC modes
	noarc      = "noarc"
	arcenabled = "arcenabled"

	// Alias for deferring ARC for readability.
	deferARC = "DeferArcActivationUntilUserSessionStartUpTaskCompletion"
)

var disableARCSyncOption = chrome.ExtraArgs(arc.DisableSyncFlags()...)

// loginPerfTestParam is a set of parameters for the login perf test.
// The baseline parameters are:
//   - windows: 8
//   - arcMode: arcenabled
//   - tabletMode: false
//   - browserType: browser.TypeAsh
//   - lacrosSelection: lacros.NotSelected (i.e. Ash)
//   - preloadLacros: true
type loginPerfTestParam struct {
	windows          int              // Number of session restored windows.
	arcMode          string           // ARC mode to test.
	tabletMode       bool             // Whether to run the test in tablet mode.
	browserType      browser.Type     // browser.{TypeAsh/TypeLacros}
	lacrosSelection  lacros.Selection // lacros.{Omaha,Rootfs}
	preloadLacros    bool             // Whether to enable LacrosLaunchAtLoginScreen feature.
	disabledFeatures []string         // Controls ash-chrome features to be disabled.
	enabledFeatures  []string         // Controls ash-chrome features to be enabled.
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         LoginPerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Measures performance and UI smoothness of ChromeOS login",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"vincentchiang@google.com",
			"oshima@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		SoftwareDeps: []string{"chrome"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
			"ui.gaiaPoolDefault",
		},
		Data:    []string{"animation.html", "animation.js", loginPerfTraceConfigFileName},
		Timeout: 15 * time.Minute,
		Params: []testing.Param{{
			ExtraAttr:         []string{"group:cuj", "cuj_loginperf"},
			ExtraSoftwareDeps: []string{"arc"},
			Val: loginPerfTestParam{
				8,                  // windows
				arcenabled,         // arcMode
				false,              // tabletMode
				browser.TypeAsh,    // browserType
				lacros.NotSelected, // lacrosSelection
				false,              // preloadLacros
				[]string{deferARC}, // disabledFeatures
				[]string{},         // enabledFeatures
			},
		}, {
			Name:      "noarc_2windows",
			ExtraAttr: []string{"group:cuj", "cuj_loginperf"},
			Val: loginPerfTestParam{
				2,                  // windows
				noarc,              // arcMode
				false,              // tabletMode
				browser.TypeAsh,    // browserType
				lacros.NotSelected, // lacrosSelection
				false,              // preloadLacros
				[]string{deferARC}, // disabledFeatures
				[]string{},         // enabledFeatures
			},
		}, {
			Name:      "noarc",
			ExtraAttr: []string{"group:cuj", "cuj_loginperf"},
			Val: loginPerfTestParam{
				8,                  // windows
				noarc,              // arcMode
				false,              // tabletMode
				browser.TypeAsh,    // browserType
				lacros.NotSelected, // lacrosSelection
				false,              // preloadLacros
				[]string{deferARC}, // disabledFeatures
				[]string{},         // enabledFeatures
			},
		}, {
			Name:              "2windows",
			ExtraAttr:         []string{"group:cuj", "cuj_loginperf"},
			ExtraSoftwareDeps: []string{"arc"},
			Val: loginPerfTestParam{
				2,                  // windows
				arcenabled,         // arcMode
				false,              // tabletMode
				browser.TypeAsh,    // browserType
				lacros.NotSelected, // lacrosSelection
				false,              // preloadLacros
				[]string{deferARC}, // disabledFeatures
				[]string{},         // enabledFeatures
			},
		}, {
			Name:              "lacros_rootfs_noarc_2windows",
			ExtraAttr:         []string{"group:cuj", "cuj_loginperf"},
			ExtraSoftwareDeps: []string{"lacros"},
			Val: loginPerfTestParam{
				2,                  // windows
				noarc,              // arcMode
				false,              // tabletMode
				browser.TypeLacros, // browserType
				lacros.Rootfs,      // lacrosSelection
				true,               // preloadLacros
				[]string{deferARC}, // disabledFeatures
				[]string{},         // enabledFeatures
			},
		}, {
			Name:              "lacros_rootfs_noarc",
			ExtraAttr:         []string{"group:cuj", "cuj_loginperf"},
			ExtraSoftwareDeps: []string{"lacros"},
			Val: loginPerfTestParam{
				8,                  // windows
				noarc,              // arcMode
				false,              // tabletMode
				browser.TypeLacros, // browserType
				lacros.Rootfs,      // lacrosSelection
				true,               // preloadLacros
				[]string{deferARC}, // disabledFeatures
				[]string{},         // enabledFeatures
			},
		}, {
			Name:              "lacros_rootfs_2windows",
			ExtraAttr:         []string{"group:cuj", "cuj_loginperf"},
			ExtraSoftwareDeps: []string{"lacros", "arc"},
			Val: loginPerfTestParam{
				2,                  // windows
				arcenabled,         // arcMode
				false,              // tabletMode
				browser.TypeLacros, // browserType
				lacros.Rootfs,      // lacrosSelection
				true,               // preloadLacros
				[]string{deferARC}, // disabledFeatures
				[]string{},         // enabledFeatures
			},
		}, {
			Name:              "lacros_rootfs",
			ExtraAttr:         []string{"group:cuj", "cuj_loginperf"},
			ExtraSoftwareDeps: []string{"lacros", "arc"},
			Val: loginPerfTestParam{
				8,                  // windows
				arcenabled,         // arcMode
				false,              // tabletMode
				browser.TypeLacros, // browserType
				lacros.Rootfs,      // lacrosSelection
				true,               // preloadLacros
				[]string{deferARC}, // disabledFeatures
				[]string{},         // enabledFeatures
			},
		}, {
			Name:              "lacros_rootfs_nopreload_noarc",
			ExtraAttr:         []string{"group:cuj", "cuj_loginperf"},
			ExtraSoftwareDeps: []string{"lacros"},
			Val: loginPerfTestParam{
				8,                  // windows
				noarc,              // arcMode
				false,              // tabletMode
				browser.TypeLacros, // browserType
				lacros.Rootfs,      // lacrosSelection
				false,              // preloadLacros
				[]string{deferARC}, // disabledFeatures
				[]string{},         // enabledFeatures
			},
		}, {
			Name:              "lacros_rootfs_nopreload",
			ExtraAttr:         []string{"group:cuj", "cuj_loginperf"},
			ExtraSoftwareDeps: []string{"lacros", "arc"},
			Val: loginPerfTestParam{
				8,                  // windows
				arcenabled,         // arcMode
				false,              // tabletMode
				browser.TypeLacros, // browserType
				lacros.Rootfs,      // lacrosSelection
				false,              // preloadLacros
				[]string{deferARC}, // disabledFeatures
				[]string{},         // enabledFeatures
			},
		}, {
			Name:              "tablet",
			ExtraAttr:         []string{"group:cuj", "cuj_loginperf"},
			ExtraSoftwareDeps: []string{"arc"},
			Val: loginPerfTestParam{
				8,                  // windows
				arcenabled,         // arcMode
				true,               // tabletMode
				browser.TypeAsh,    // browserType
				lacros.NotSelected, // lacrosSelection
				false,              // preloadLacros
				[]string{deferARC}, // disabledFeatures
				[]string{},         // enabledFeatures
			},
		}, {
			Name:              "lacros_rootfs_tablet",
			ExtraAttr:         []string{"group:cuj", "cuj_loginperf"},
			ExtraSoftwareDeps: []string{"lacros", "arc"},
			Val: loginPerfTestParam{
				8,                  // windows
				arcenabled,         // arcMode
				true,               // tabletMode
				browser.TypeLacros, // browserType
				lacros.Rootfs,      // lacrosSelection
				true,               // preloadLacros
				[]string{deferARC}, // disabledFeatures
				[]string{},         // enabledFeatures
			},
		}, {
			// Planned configuration for our production.
			Name:              "lacros_chrome_rootfs_prod",
			ExtraAttr:         []string{"group:cuj", "cuj_loginperf"},
			ExtraSoftwareDeps: []string{"lacros", "arc"},
			Val: loginPerfTestParam{
				8,                  // windows
				arcenabled,         // arcMode
				false,              // tabletMode
				browser.TypeLacros, // browserType
				lacros.Rootfs,      // lacrosSelection
				true,               // preloadLacros
				[]string{},         // disabledFeatures
				[]string{deferARC}, // enabledFeatures
			},
		}, {
			Name:              "lacros_chrome_rootfs_prod_2windows",
			ExtraAttr:         []string{"group:cuj", "cuj_loginperf"},
			ExtraSoftwareDeps: []string{"lacros", "arc"},
			Val: loginPerfTestParam{
				2,                  // windows
				arcenabled,         // arcMode
				false,              // tabletMode
				browser.TypeLacros, // browserType
				lacros.Rootfs,      // lacrosSelection
				true,               // preloadLacros
				[]string{},         // disabledFeatures
				[]string{deferARC}, // enabledFeatures
			},
		}},
	})
}

type loginPerfTestConfig struct {
	arcMode              string             // ARC++ mode name.
	arcOpt               []chrome.Option    // Additional Chrome options to control ARC++.
	creds                chrome.Creds       // Test user credentials.
	currentWindows       int                // Test will ensure that at least this number of windows will be restored.
	expectHistograms     []string           // The list of expected histograms. Test will wait for all of them to get reported.
	inTabletMode         bool               // Whether Chrome should run in tablet mode.
	lacrosCfg            *lacrosfixt.Config // Lacros configuration.
	param                loginPerfTestParam // Tast subtest parameters.
	windows              int                // Number of restored windows as configured.
	signinExtManifestKey string             // Key used for login extension.
}

// loginPerfStartToLoginScreen starts Chrome to the login screen.
func loginPerfStartToLoginScreen(
	ctx context.Context,
	testConfig *loginPerfTestConfig,
) (cr *chrome.Chrome, retErr error) {
	// chrome.NoLogin() and chrome.KeepState() are needed to show the login
	// screen with a user pod (instead of the OOBE login screen).
	options := []chrome.Option{
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.LoadSigninProfileExtension(
			testConfig.signinExtManifestKey),
		chrome.EnableFeatures("FullRestore"),
		chrome.EnableRestoreTabs(),
		chrome.SkipForceOnlineSignInForTesting(),
		chrome.EnableWebAppInstall(),
		chrome.HideCrashRestoreBubble(), // Ignore possible incomplete shutdown.
		// Disable whats-new page. See crbug.com/1271436.
		chrome.DisableFeatures("ChromeWhatsNewUI"),
		chrome.DisableFeatures(testConfig.param.disabledFeatures...),
		chrome.EnableFeatures(testConfig.param.enabledFeatures...),
	}
	if testConfig.param.browserType == browser.TypeLacros {
		defaultOpts, err := testConfig.lacrosCfg.Opts()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get default options")
		}
		options = append(options, defaultOpts...)
	} else {
		// For Ash we need to force session restore in another way.
		options = append(options, chrome.ForceLaunchBrowser())
	}
	if testConfig.param.preloadLacros {
		options = append(options, chrome.EnableFeatures("LacrosLaunchAtLoginScreen"))
		options = append(options, chrome.EnableFeatures("LacrosForkZygotesAtLoginScreen"))
		options = append(options, chrome.ExtraArgs("--force-lacros-launch-at-login-screen-for-testing"))
		testing.ContextLog(ctx, "loginPerfStartToLoginScreen: Enabling LacrosLaunchAtLoginScreen feature")
	} else {
		options = append(options, chrome.DisableFeatures("LacrosLaunchAtLoginScreen"))
		testing.ContextLog(ctx, "loginPerfStartToLoginScreen: Disabling LacrosLaunchAtLoginScreen feature")
	}

	// Drop caches to simulate cold boot.
	if err := disk.DropCaches(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to drop caches")
	}
	testing.ContextLog(ctx, "loginPerfStartToLoginScreen: File caches dropped")

	cr, err := chrome.New(
		ctx,
		append(options, testConfig.arcOpt...)...,
	)
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

	if err := ash.SetTabletModeEnabled(ctx, tLoginConn, testConfig.inTabletMode); err != nil {
		return nil, errors.Wrapf(err, "failed to set tablet mode %v", testConfig.inTabletMode)
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
	browserType browser.Type,
) (retL *lacros.Lacros, lacrosConnectTime *time.Duration, retErr error) {
	outdir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return nil, nil, errors.New("no output directory exists")
	}
	tLoginConn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "creating login test API connection failed")
	}
	defer faillog.DumpUITreeOnError(ctx, outdir, func() bool { return retErr != nil }, tLoginConn)

	// TODO(crbug/1109381): the password field isn't actually ready just yet when WaitState returns.
	// This causes it to miss some of the keyboard input, so the password will be wrong.
	// We can check in the UI for the password field to exist, which seems to be a good enough indicator that
	// the field is ready for keyboard input.
	if err := lockscreen.WaitForPasswordField(ctx, tLoginConn, credentials.User, 15*time.Second); err != nil {
		return nil, nil, errors.Wrap(err, "password text field did not appear in the ui")
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get keyboard")
	}
	defer kb.Close(ctx)

	if err := kb.Type(ctx, credentials.Pass+"\n"); err != nil {
		return nil, nil, errors.Wrap(err, "entering password failed")
	}

	// Check if the login was successful using the API and also by looking for the shelf in the UI.
	if st, err := lockscreen.WaitState(
		ctx,
		tLoginConn,
		func(st lockscreen.State) bool { return st.LoggedIn },
		30*time.Second,
	); err != nil {
		return nil, nil, errors.Wrapf(err, "failed waiting to log in: last state: %+v", st)
	}

	if err := ash.WaitForShelf(ctx, tLoginConn, 120*time.Second); err != nil {
		return nil, nil, errors.Wrap(err, "shelf did not appear after logging in")
	}

	if browserType == browser.TypeLacros {
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to connect to test api")
		}
		startTime := time.Now()
		// lacros.Connect() fails if DevTools connection file was already created, but Chrome does not accept connections.
		// Retry for 20 seconds.
		for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
			retL, err = lacros.Connect(ctx, tconn)
			if err == nil {
				duration := time.Since(startTime)
				lacrosConnectTime = &duration
				testing.ContextLogf(ctx, "loginPerfDoLogin: Connecting to lacros took %s", duration.Round(time.Millisecond))
				return retL, lacrosConnectTime, err
			}
			testing.ContextLog(ctx, "loginPerfDoLogin: Connect to lacros failed. Sleeping for 10 milliseconds before retry")
			// GoBigSleepLint: This is sleep between retries.
			if err := testing.Sleep(ctx, 10*time.Millisecond); err != nil {
				return nil, nil, errors.Wrap(err, "failed to wait for lacros-chrome test connection")
			}
		}
		return nil, nil, errors.Wrap(err, "timed out retrying connection to Lacros")
	}
	return nil, nil, nil
}

// loginPerfCreateWindows creates |n| windows and for Ash or Lacros.
func loginPerfCreateWindows(
	ctx context.Context,
	cr *chrome.Chrome,
	l *lacros.Lacros,
	url string,
	n int,
) error {
	if l != nil {
		for i := 0; i < n; i++ {
			conn, err := l.NewConn(ctx, url, browser.WithNewWindow())
			if err != nil {
				return errors.Wrapf(err, "(%d) failed to connect to the %q restore URL", i, url)
			}
			defer conn.Close()
		}
	} else {
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to connect to Ash test api")
		}
		if err := ash.CreateWindows(ctx, tconn, cr, url, n); err != nil {
			return errors.Wrap(err, "failed to create browser windows")
		}
	}
	return nil
}

// countVisibleWindows is a proxy to ash.CountVisibleWindows(...)
func countVisibleWindows(ctx context.Context, cr *chrome.Chrome) (int, error) {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "failed to connect to Ash test api")
	}
	visible, err := ash.CountVisibleWindows(ctx, tconn)
	if err != nil {
		err = errors.Wrap(err, "failed to count browser windows")
	}
	return visible, nil
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
	pv *perfutil.Values,
	hist *histogram.Histogram,
	unit,
	valueName string,
) error {
	value, err := maxHistogramValue(hist)
	if err != nil {
		return errors.Wrapf(err, "failed to get %s data", hist.Name)
	}
	pv.Append(perf.Metric{
		Name:      valueName,
		Unit:      unit,
		Direction: perf.SmallerIsBetter,
	}, value)
	return nil
}

// logout is a proxy to chrome.autotestPrivate.logout
func logout(ctx context.Context, cr *chrome.Chrome, l *lacros.Lacros) error {
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

	if l != nil {
		// TODO(crbug.com/1318180): at the moment for Lacros, we're not
		// getting SetUpWithNewChrome close closure because when used
		// it'd close all resources including targets and wouldn't let
		// the session to properly restore later during
		// performRegularLogin. As a short term workaround we're
		// closing Lacros resources using CloseResources fn instead,
		// though ideally we want to use SetUpWithNewChrome close
		// closure when it's properly implemented.
		l.CloseResources(ctx)
	}

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

// setAlwaysRestoreSettings opens OS settings and sets the 'Always restore' setting. In order to
// avoid possible noise when collecting the browser login time performance at restoring time, this
// function also makes sure to close the OS settings app before returning.
func setAlwaysRestoreSettings(ctx context.Context, tconn *chrome.TestConn) error {
	settings, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.Apps)
	if err != nil {
		return errors.Wrap(err, "failed to launch apps settings page")
	}
	ui := uiauto.New(tconn)
	restoreButtonNode := nodewith.Name("Restore apps on startup").Role(role.ComboBoxSelect)
	if err := uiauto.IfSuccessThen(
		ui.WaitUntilExists(restoreButtonNode),
		ui.DoDefault(restoreButtonNode))(ctx); err != nil {
		return err
	}

	alwaysRestoreOptionNode := nodewith.Name("Always restore").Role(role.ListBoxOption)
	if err := uiauto.IfSuccessThen(
		ui.WaitUntilExists(alwaysRestoreOptionNode),
		ui.DoDefault(alwaysRestoreOptionNode))(ctx); err != nil {
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

// initializeLoginPerfTest initializes user session state that will be restored
// in subsequent test runs.
func initializeLoginPerfTest(ctx context.Context,
	sOutDir string,
	lacrosConfig *lacrosfixt.Config,
	loginPool string,
	param loginPerfTestParam,
	signinExtManifestKey,
	url string,
) (
	retCreds chrome.Creds,
	retErr error,
) {
	// Reserve some time to dump ARC state if apps fail to install.
	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	options := []chrome.Option{
		chrome.GAIALoginPool(loginPool),
		chrome.EnableRestoreTabs(),
		chrome.SkipForceOnlineSignInForTesting(),
		chrome.EnableWebAppInstall(),
		// Disable whats-new page. See crbug.com/1271436.
		// Disable deferring ARC.
		chrome.DisableFeatures("ChromeWhatsNewUI", deferARC),
		// --disable-sync disables test account info sync, eg. Wi-Fi credentials,
		// so that each test run does not remember info from last test run.
		chrome.ExtraArgs("--disable-sync"),
	}
	// Only enable arc if it's supported.
	if arc.Supported() {
		// We enable ARC initially to fully initialize it.
		options = append(options, chrome.ARCSupported(), disableARCSyncOption)
	}
	if param.browserType == browser.TypeLacros {
		defaultOpts, err := lacrosConfig.Opts()
		if err != nil {
			return chrome.Creds{}, errors.Wrap(err, "failed to get default options")
		}
		options = append(options, defaultOpts...)
	}
	if param.preloadLacros {
		options = append(options, chrome.EnableFeatures("LacrosLaunchAtLoginScreen"))
		options = append(options, chrome.EnableFeatures("LacrosForkZygotesAtLoginScreen"))
		options = append(options, chrome.ExtraArgs("--force-lacros-launch-at-login-screen-for-testing"))
		testing.ContextLog(ctx, "initializeLoginPerfTest: Enabling LacrosLaunchAtLoginScreen feature")
	} else {
		options = append(options, chrome.DisableFeatures("LacrosLaunchAtLoginScreen"))
		testing.ContextLog(ctx, "initializeLoginPerfTest: Disabling LacrosLaunchAtLoginScreen feature")
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

	testing.ContextLog(ctx, "Opting into Play Store")
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return chrome.Creds{}, errors.Wrap(err, "failed to connect to test api")
	}
	if arc.Supported() {
		if err := optin.Perform(ctx, cr, tconn); err != nil {
			return chrome.Creds{}, errors.Wrap(err, "failed to optin to Play Store")
		}
		testing.ContextLog(ctx, "Optin finished")
	} else {
		testing.ContextLog(ctx, "ARC++ is not supported. Running test without ARC")
	}

	var l *lacros.Lacros
	if param.browserType == browser.TypeLacros {
		var err error
		l, err = lacros.Launch(ctx, tconn)
		if err != nil {
			return chrome.Creds{}, errors.Wrap(err, "failed to launch lacros-chrome")
		}
	}

	// Wait for ARC++ aps to download and initialize.
	if arc.Supported() {
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
		testing.ContextLog(ctx, "Initialize: Waiting 1 minute to allow time for session to fully initialize")
		// GoBigSleepLint: Give session time to settle.
		if err := testing.Sleep(ctx, time.Minute); err != nil {
			return chrome.Creds{}, errors.Wrap(err, "failed to run initial wait time")
		}
	}
	defer faillog.DumpUITreeOnError(ctx, sOutDir, func() bool { return retErr != nil }, tconn)
	if err := setAlwaysRestoreSettings(ctx, tconn); err != nil {
		return chrome.Creds{}, errors.Wrap(err, "failed to adjust always restore settings")
	}
	if err := logout(ctx, cr, l); err != nil {
		return creds, errors.Wrap(err, "failed to log out")
	}
	cr = nil

	testing.ContextLog(ctx, "Create new windows: Sign in to create new windows")
	// Log in and log out to create a user pod on the login screen and required number of windows in session.
	err = func() error {
		// We do not need ARC to create Chrome windows.
		testConfig := &loginPerfTestConfig{
			arcMode:              param.arcMode,
			arcOpt:               []chrome.Option{},
			creds:                creds,
			inTabletMode:         false,
			lacrosCfg:            lacrosConfig,
			param:                param,
			windows:              param.windows,
			signinExtManifestKey: signinExtManifestKey,
		}
		cr, err := loginPerfStartToLoginScreen(ctx, testConfig)
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

		l, _, err := loginPerfDoLogin(ctx, cr, creds, param.browserType)
		if err != nil {
			return err
		}
		// Wait for windows to be restored.
		var visible int
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			var err error
			if visible, err = countVisibleWindows(ctx, cr); err != nil {
				return testing.PollBreak(err)
			}
			if visible != 0 && visible != 1 {
				return errors.Errorf("unexpected number of visible windows: expected %d, found %d", 0, visible)
			}
			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 10 * time.Millisecond}); err != nil {
			return errors.Wrap(err, "failed to check number of existing windows before creating new ones")
		}
		testing.ContextLogf(ctx, "Before creating windows: visible=%d", visible)
		if err := loginPerfCreateWindows(ctx, cr, l, url, param.windows); err != nil {
			return err
		}
		testing.ContextLog(ctx, "Sign out: sleep for 20 seconds to let session settle")
		// GoBigSleepLint: Give session time to settle.
		if err := testing.Sleep(ctx, 20*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep for 20 seconds")
		}
		if err := logout(ctx, cr, l); err != nil {
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

// testFunction is the actual test flow that could executed multiple times to get average data or generate tracing.
func testFunction(
	ctx context.Context,
	s *testing.State,
	name string,
	testConfig *loginPerfTestConfig,
	runTracing bool,
) (
	*chrome.Chrome,
	*lacros.Lacros,
	[]*histogram.Histogram,
	map[perf.Metric][]float64,
	error,
) {
	cr, err := loginPerfStartToLoginScreen(ctx, testConfig)
	if err != nil {
		return cr, nil, nil, nil, errors.Wrap(err, "failed to start to login screen")
	}

	var lacrosConnectTime *time.Duration
	var l *lacros.Lacros

	// The actual test function
	testFunc := func(ctx context.Context, stopTracing func(ctx context.Context) error) error {
		// Limit each test run to 2 minutes (so that we could retry early).
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		out, err := exec.Command("ps", "aux").Output()
		testing.ContextLog(ctx, "ps aux result:")
		testing.ContextLog(ctx, string(out))
		if err != nil {
			s.Fatal("ps aux failed with: ", err)
		}
		l, lacrosConnectTime, err = loginPerfDoLogin(ctx, cr, testConfig.creds, testConfig.param.browserType)
		if err != nil {
			return errors.Wrap(err, "failed to log in")
		}
		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to connect to test api")
		}
		if err := ash.ForEachWindow(ctx, tconn, func(w *ash.Window) error {
			return ash.WaitWindowFinishAnimating(ctx, tconn, w.ID)
		}); err != nil {
			return errors.Wrap(err, "failed to wait")
		}
		sleepSeconds := 10 * time.Second
		if stopTracing != nil {
			// Stopping tracing before the full 10 seconds have
			// elapsed reduces the trace file size by approximately
			// 20% (from ~10MB to ~8MB) per file.
			s.Log("Sleep for 5 seconds to wait for last metrics before stopping tracing")
			// GoBigSleepLint: Controls the tracing time.
			if err := testing.Sleep(ctx, 5*time.Second); err != nil {
				return errors.Wrap(err, "failed to sleep for 5 seconds")
			}
			if err := stopTracing(ctx); err != nil {
				return errors.Wrap(err, "failed to stop tracing")
			}
			sleepSeconds = 5 * time.Second
		}
		s.Logf("Sleep for %f seconds to let session settle and save restore data", sleepSeconds.Seconds())
		// GoBigSleepLint: Give session time to settle and save restore data.
		if err := testing.Sleep(ctx, sleepSeconds); err != nil {
			return errors.Wrapf(err, "failed to sleep for %f seconds", sleepSeconds.Seconds())
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
		return cr, l, nil, nil, errors.Wrap(err, "creating login test api connection failed")
	}
	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Initialize CUJ recording.
	cujRecorder, err := cujrecorder.NewRecorderWithTestConn(
		ctx,
		tLoginConn,
		cr,
		tLoginConn,
		nil,
		cujrecorder.RecorderOptions{},
	)
	if err != nil {
		s.Fatal("Failed to create a CUJ recorder: ", err)
	}
	defer cujRecorder.Close(closeCtx)

	// TODO(b/237400719): support lacros
	for _, metricConfig := range [][]cujrecorder.MetricConfig{
		cujrecorder.AshCommonMetricConfigs(),
		cujrecorder.BrowserCommonMetricConfigs(),
		cujrecorder.AnyChromeCommonMetricConfigs(),
	} {
		if err := cujRecorder.AddCollectedMetrics(tLoginConn, browser.TypeAsh, metricConfig...); err != nil {
			s.Fatal("Failed to add recorded metrics: ", err)
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
		if runTracing {
			// See go/trace-in-cuj-tests about rules for tracing.
			if err := cujRecorder.StartTracingWithName(ctx, s.OutDir(), name+"-trace.data", s.DataPath(loginPerfTraceConfigFileName)); err != nil {
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
			testConfig.expectHistograms...,
		)
		if err != nil {
			return err
		}

		visible := 0
		if visible, err = countVisibleWindows(ctx, cr); err != nil {
			return err
		}
		if visible != testConfig.currentWindows && visible != testConfig.currentWindows+1 {
			err = errors.Errorf("unexpected number of visible windows: expected %d, found %d", testConfig.currentWindows, visible)
		}
		return err
	}
	if err := cujRecorder.Run(ctx, cujFunc); err != nil {
		return cr, l, nil, nil, errors.Wrap(err, "failed to run the test scenario")
	}
	tpsValues := perf.NewValues()
	if err := cujRecorder.Record(ctx, tpsValues); err != nil {
		return cr, l, nil, nil, errors.Wrap(err, "failed to collect the data from the recorder")
	}
	if err := cujRecorder.SaveTraceFiles(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to save trace files: ", err)
	}
	if lacrosConnectTime != nil {
		tpsValues.Set(perf.Metric{
			Name:      "Ash.Tast.LacrosConnectTime",
			Unit:      "millisecond",
			Direction: perf.SmallerIsBetter,
		}, float64(lacrosConnectTime.Milliseconds()))
	}
	return cr, l, histograms, tpsValues.GetValues(), err
}

// storeHistograms transforms []*histogram.Histogram test results into perf Values to report.
func storeHistograms(
	ctx context.Context,
	expectHistograms, heuristicsHistograms []string,
	pv *perfutil.Values,
	hists []*histogram.Histogram,
) error {
	heuristicsHistogramsMap := make(map[string]bool, len(expectHistograms))
	for _, v := range heuristicsHistograms {
		heuristicsHistogramsMap[v] = true
	}

	for _, hist := range hists {
		if heuristicsHistogramsMap[hist.Name] {
			perfutil.StoreMetricWithHeuristics(ctx, pv, hist, "")
			continue
		}
		valueName := hist.Name
		switch hist.Name {
		case ensureWorkVisibleHistogram:
			reportMaxHistogramValue(ctx, pv, hist, "microsecond", valueName)
		case allBrowserWindowsCreated,
			allBrowserWindowsPresented,
			allBrowserWindowsShown,
			allShelfIconsLoaded,
			ashTastBootTimeLogin2,
			bootTimeLogin3,
			ensureWorkVisibleLowResHistogram,
			shelfLoginAnimationEnd,
			ashTastArcUIAvailableAfterLoginDuration,
			arcTastUIAvailableTimeDelta,
			uptimeLogoutToUIStopAfterLogout,
			uptimeUIStopToProcessesTerminatedAfterLogout,
			uptimeOtherProcessesTerminatedToChromeExecAfterLogout,
			uptimeChromeExecToLoginPromptVisibleAfterLogout,
			uptimeLogout,
			uptimeLoginPromptSetupTimeAfterLogout,
			uptimeLogoutToLoginPromptVisible:

			reportMaxHistogramValue(ctx, pv, hist, "millisecond", valueName)
		default:
			return errors.Errorf("unknown histogram %q", hist.Name)
		}
	}
	return nil
}

func LoginPerf(ctx context.Context, s *testing.State) {
	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	param := s.Param().(loginPerfTestParam)
	lacrosCfg := lacrosfixt.NewConfig(
		lacrosfixt.Selection(param.lacrosSelection),
		lacrosfixt.KeepAlive(true), // Enable keep alive to emulate production environment.
	)

	// Run an http server to serve the test contents for accessing from the chrome browsers.
	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	url := server.URL + "/animation.html"

	displCount, err := graphics.NumberOfOutputsConnected(ctx)
	if err != nil {
		s.Fatal("Failed to get connected displays count: ", err)
	}

	// Run the login flow for various situations.
	// - change the number of browser windows, 2 or 8
	// - the window system status; clamshell mode or tablet mode.
	windows := param.windows
	arcMode := param.arcMode

	var arcOpt []chrome.Option
	switch arcMode {
	case noarc:
	case arcenabled:
		arcOpt = []chrome.Option{chrome.ARCSupported(),
			chrome.DisableFeatures("ArcExternalStorageAccess"),
			disableARCSyncOption}
	default:
		s.Fatal("Unknown arcMode value=", arcMode)
	}

	// Log in and log out to create a user pod on the login screen.
	creds, err := initializeLoginPerfTest(
		ctx,
		s.OutDir(),
		lacrosCfg,
		s.RequiredVar("ui.gaiaPoolDefault"),
		param,
		s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
		url,
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
	// Use 3 (successful) runs instead of 10, to reduce tests time.
	r.SetRunsNumber(perfutil.RunnerCyclesOptions{MaxRuns: 5, MinSuccessfulRuns: 3})
	s.Logf("Starting test: %s for  %d windows", arcMode, windows)

	inTabletMode := param.tabletMode
	suffix := ".ClamshellMode"
	if inTabletMode {
		suffix = ".TabletMode"
	}

	heuristicsHistograms := []string{
		"Ash.LoginAnimation.Smoothness" + suffix,
		"Ash.LoginAnimation.Jank" + suffix,
		"Ash.LoginAnimation.Duration2" + suffix,
	}

	allHistograms := []string{
		ensureWorkVisibleHistogram,
		ensureWorkVisibleLowResHistogram,
		allBrowserWindowsCreated,
		allBrowserWindowsShown,
		allShelfIconsLoaded,
		ashTastBootTimeLogin2,
		bootTimeLogin3,
		uptimeLogoutToUIStopAfterLogout,
		uptimeUIStopToProcessesTerminatedAfterLogout,
		uptimeOtherProcessesTerminatedToChromeExecAfterLogout,
		uptimeChromeExecToLoginPromptVisibleAfterLogout,
		uptimeLogout,
		uptimeLoginPromptSetupTimeAfterLogout,
		uptimeLogoutToLoginPromptVisible,
	}
	// Histogram is only collected when the DUT is connected to the display.
	if displCount > 0 {
		allHistograms = append(allHistograms, allBrowserWindowsPresented)
		allHistograms = append(allHistograms, shelfLoginAnimationEnd)
		allHistograms = append(allHistograms, heuristicsHistograms...)
	}
	if arcMode != noarc {
		allHistograms = append(allHistograms,
			ashTastArcUIAvailableAfterLoginDuration,
			arcTastUIAvailableTimeDelta,
		)
	}

	testConfig := &loginPerfTestConfig{
		arcMode,
		arcOpt,
		creds,
		param.windows,
		allHistograms,
		inTabletMode,
		lacrosCfg,
		param,
		windows,
		s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
	}

	// |cr| and |l| are shared between multiple
	// runs, because Chrome connection must to be
	// closed only after histograms are stored.
	var cr *chrome.Chrome
	var l *lacros.Lacros

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
				// Fill in external 'cr', 'l'.
				cr, l, histograms, tpsValues, err = testFunction(ctx, s, name, testConfig, false)
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

			if err := storeHistograms(
				ctx,
				testConfig.expectHistograms,
				heuristicsHistograms,
				pv,
				hists,
			); err != nil {
				return errors.Wrap(err, "storeHistograms failed")
			}
			if err := logout(ctx, cr, l); err != nil {
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

	var tracingHistograms []*metrics.Histogram
	var tpsValues map[perf.Metric][]float64

	cr, l, tracingHistograms, tpsValues, err = testFunction(ctx, s, fmt.Sprintf("%s-tracing", testName), testConfig, true)
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
	} else if err := storeHistograms(
		ctx,
		testConfig.expectHistograms,
		heuristicsHistograms,
		tracingValues,
		tracingHistograms,
	); err != nil {
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
	if err := logout(ctx, cr, l); err != nil {
		s.Logf("WARNING: Failed to sign out from the tracing session %s-tracing: %v", testName, err)
	}
	cr = nil

	if err := r.Values().Save(closeCtx, s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}
