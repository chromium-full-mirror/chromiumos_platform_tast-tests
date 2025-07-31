// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/arc/arcent"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/retry"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"go.chromium.org/tast/core/timing"
)

const (
	managedEntVpnAccountPoolName = "arc.managedEntVpnAccountPool"
	vpnPackage                   = "com.paloaltonetworks.globalprotect"
	vpnAppName                   = "GlobalProtect"
	vpnServerURL                 = "palo-okta.capse-iss.com"
	vpnConnectedState            = "CONNECTED"

	connectShieldButtonID           = ":id/btnShield"
	vpnAddressTextEntryID           = ":id/etPortal"
	connectButtonID                 = ":id/btnSubmit"
	okButtonID                      = "android:id/button1"
	vpnStateTextID                  = ":id/state"
	errorMessageTextID              = ":id/tvInfo"
	skipEnableNotificationsButtonID = ":id/btnSkip"

	arcCertInstallLogRegex = `ArcCertInstaller::InstallArcCert User_.*`
	skipArcTermsLogRegex   = `Skip ARC Terms of Service negotiation`
)

type managedEntVpnCertTestParam struct {
	performSecondLoginFlag bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     ManagedEnterpriseVpnCert,
		Desc:     "Verify Enterprise VPN works",
		Contacts: []string{"arc-commercial@google.com", "batoon@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      20 * time.Minute,
		SoftwareDeps: []string{"android_vm", "chrome", "no_qemu"},
		VarDeps:      []string{managedEntVpnAccountPoolName},
		HardwareDeps: hwdep.D(hwdep.MinStorage(17)), // UI Automator is flaky on low storage devices.
		Params: []testing.Param{{
			Name: "single_login",
			Val:  managedEntVpnCertTestParam{performSecondLoginFlag: false},
		}, {
			Name: "double_login",
			Val:  managedEntVpnCertTestParam{performSecondLoginFlag: true},
		}},
	})
}

func ManagedEnterpriseVpnCert(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: 3,
		DoRetries:   true,
		Errorf:      s.Errorf,
		Logf:        s.Logf}

	if err := testing.Poll(ctx, func(ctx context.Context) (retErr error) {
		performSecondLoginFlag := s.Param().(managedEntVpnCertTestParam).performSecondLoginFlag
		cr, a, tconn, err := logInAndStartArc(
			ctx, s.RequiredVar(managedEntVpnAccountPoolName), s.OutDir(), s.HasError, performSecondLoginFlag, rl.Attempts)
		if err != nil {
			return rl.Retry("prepare device for testing", err)
		}
		defer cr.Close(cleanupCtx)
		defer a.Close(ctx)
		defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
		defer a.DumpUIHierarchyOnError(cleanupCtx, s.OutDir(), s.HasError)
		// Dump logcat on error since it gets overwritten by subsequent attempts.
		defer dumpLogcatOnError(cleanupCtx, a, s.OutDir(), s.HasError, rl.Attempts)

		// Wait for Chrome logs to show ARC Certs installed.
		s.Log("Waiting for ARC Certs to be installed")
		if err := waitForArcCertsInstallationInChromeLog(ctx, cr); err != nil {
			return rl.Retry("see the ARC Certs installed in Chrome logs", err)
		}
		s.Log("ARC Certs successfully installed")

		d, err := a.NewUIDevice(ctx)
		if err != nil {
			return rl.Retry("initialize UI Automator", err)
		}
		defer d.Close(cleanupCtx)

		kb, err := input.Keyboard(ctx)
		if err != nil {
			return rl.Retry("get keyboard controller", err)
		}
		defer kb.Close(cleanupCtx)

		s.Log("Launching GlobalProtect")
		app, err := apputil.NewApp(ctx, kb, tconn, a, d, vpnAppName, vpnPackage)
		if err != nil {
			return rl.Retry("create the instance of GlobalProtect app", err)
		}
		if _, err := app.Launch(ctx); err != nil {
			return rl.Retry("launch GlobalProtect app", err)
		}

		// Connect with Global VPN Protect.
		if err := connectToVpnWithGlobalProtect(ctx, tconn, cr, a, d); err != nil {
			return rl.Exit("connect to VPN", err)
		}
		s.Log("Global Protect VPN successfully connected")

		// Disconnect at the end of successful test.
		connectButton := d.Object(ui.ID(vpnPackage + connectShieldButtonID))
		if err := connectButton.Click(ctx); err != nil {
			s.Log("Error while trying to disconnect from vpn during cleanup: ", err)
		}
		return nil
	}, nil); err != nil {
		s.Fatal("Enterprise VPN cert test failed: ", err)
	}
}

// logInAndStartArc logs into the device with credentials from the specified pool and starts ARC.
func logInAndStartArc(ctx context.Context, poolName, outDir string, hasError func() bool, performSecondLogin bool, attemptNum int) (*chrome.Chrome, *arc.ARC, *chrome.TestConn, error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	creds, err := credconfig.PickRandomCreds(poolName)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to get credentials from cred pool")
	}

	cr, err := chrome.New(
		ctx,
		chrome.GAIALogin(creds),
		chrome.ARCSupported(),
		chrome.UnRestrictARCCPU(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...),
		chrome.ProdPolicy(),
	)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to start Chrome")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to create test API connection")
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, outDir, hasError, tconn)

	testing.ContextLog(ctx, "Enabling the Play Store")
	if err := optin.SetPlayStoreEnabled(ctx, tconn, true); err != nil {
		return nil, nil, nil, errors.Wrap(err, "unable to set the Play Store to enable")
	}

	testing.ContextLog(ctx, "Checking for Optin in Chrome logs")
	if err := maybePerformOptin(ctx, tconn, cr); err != nil {
		return nil, nil, nil, err
	}

	testing.ContextLog(ctx, "Starting ARC")
	a, err := arc.New(ctx, outDir, cr.NormalizedUser())
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to start ARC")
	}

	// Dump logcat on error since it gets overwritten by subsequent attempts.
	defer dumpLogcatOnError(cleanupCtx, a, outDir, hasError, attemptNum)

	// Wait for Play Store to be ready.
	testing.ContextLog(ctx, "Waiting for Play Store Ready")
	if err := optin.WaitForPlayStoreReady(ctx, tconn); err != nil {
		return nil, nil, nil, errors.Wrap(err, "could not wait for Play Store to be ready")
	}

	// Wait for the  Global Protect package to be installed.
	packages := []string{vpnPackage}
	installCtx, cancel := context.WithTimeout(ctx, arcent.InstallTimeout)
	defer cancel()
	if err := a.WaitForPackagesWithTimeout(installCtx, packages, 5*time.Minute); err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to install GlobalProtect")
	}

	// TODO(b/409336666):  Remove this once certificates work after first login.
	if !performSecondLogin {
		return cr, a, tconn, nil
	}

	// Logout the user.
	testing.ContextLog(ctx, "Logging out the user")
	if err := quicksettings.SignOut(ctx, tconn); err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to logout")
	}
	a.Close(ctx)
	cr.Close(ctx)

	// Re-login with the same user credentials.
	testing.ContextLog(ctx, "Logging back again with the same user")
	cr, err = chrome.New(
		ctx,
		chrome.KeepState(),
		chrome.ARCSupported(),
		chrome.FakeLogin(creds),
	)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to login with the same user")
	}

	tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to create test API connection")
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, outDir, hasError, tconn)

	// Re-create the ARC instance.
	testing.ContextLog(ctx, "Starting ARC again")
	a, err = arc.New(ctx, outDir, cr.NormalizedUser())
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to start ARC")
	}
	return cr, a, tconn, nil
}

// maybePerformOptin checks Chrome logs to see if the ARC Terms of Service were skipped.
// If they were not skipped, perform optin.
func maybePerformOptin(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) error {
	logContent, err := os.ReadFile(syslog.ChromeLogFile)
	if err != nil {
		return errors.Wrap(err, "could not read the Chrome log file")
	}

	r := regexp.MustCompile(fmt.Sprintf(skipArcTermsLogRegex))
	matches := r.FindAllStringSubmatch(string(logContent), -1)

	if matches != nil {
		testing.ContextLog(ctx, "Skipped the ARC Terms of Service Page. No need to optin")
		return nil
	}

	testing.ContextLog(ctx, "Opt into Play Store")
	if err := optin.Perform(ctx, cr, tconn); err != nil {
		return errors.Wrap(err, "failed to optin to Play Store")
	}
	return nil
}

// waitForArcCertsInstallationInChromeLog waits for expected InstallArcCert log.
func waitForArcCertsInstallationInChromeLog(ctx context.Context, cr *chrome.Chrome) error {
	ctx, st := timing.Start(ctx, "wait_logged_events")
	defer st.End()

	return testing.Poll(ctx, func(ctx context.Context) error {
		logContent, err := os.ReadFile(syslog.ChromeLogFile)
		if err != nil {
			return testing.PollBreak(err)
		}

		r := regexp.MustCompile(fmt.Sprintf(arcCertInstallLogRegex))
		matches := r.FindAllStringSubmatch(string(logContent), -1)
		if matches == nil {
			return errors.New("no event logged yet")
		}

		return nil
	}, &testing.PollOptions{Timeout: 60 * time.Second})
}

// connectToVpnWithGlobalProtect launches the GlobalProtect app and completes the UI flow to connect to VPN.
func connectToVpnWithGlobalProtect(ctx context.Context, tconn *chrome.TestConn,
	cr *chrome.Chrome, a *arc.ARC, d *ui.Device) error {
	testing.ContextLog(ctx, "Using GlobalProtect app to connect to VPN")
	if err := apputil.DismissMobilePrompt(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to dismiss 'designed for mobile' prompt")
	}

	// Enter the VPN server URL in the app.
	addressTextEntry := d.Object(ui.ID(vpnPackage + vpnAddressTextEntryID))
	if err := addressTextEntry.WaitForExists(ctx, 10*time.Second); err != nil {
		// If addressTextEntry does not exist, there may be a screen showing about enabling notifications.
		if err := skipEnableNotifications(ctx, d); err != nil {
			return err
		}
		if err := addressTextEntry.WaitForExists(ctx, 10*time.Second); err != nil {
			return err
		}
	}
	if err := addressTextEntry.SetText(ctx, vpnServerURL); err != nil {
		return err
	}

	// Wait for Connect button to be enabled and then click it.
	connectButton := d.Object(ui.ID(vpnPackage + connectButtonID))
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		enabled, err := connectButton.IsEnabled(ctx)
		if err != nil {
			return testing.PollBreak(err)
		}
		if !enabled {
			return errors.New("connect button is not enabled")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		return err
	}
	if err := connectButton.Click(ctx); err != nil {
		return err
	}

	// Click "OK" to window pop-up about confirming the VPN connection.
	okButton := d.Object(ui.ID(okButtonID))
	if err := okButton.WaitForExists(ctx, 15*time.Second); err != nil {
		// Click the connect button if there is no "OK" pop-up.
		if err := clickConnectButtonAgain(ctx, d); err != nil {
			return err
		}
	}
	if err := okButton.Click(ctx); err != nil {
		return err
	}

	// Wait for the state text to change to "CONNECTED".
	stateText := d.Object(ui.ID(vpnPackage + vpnStateTextID))
	if err := stateText.WaitForExists(ctx, 10*time.Second); err != nil {
		return err
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		state, err := stateText.GetText(ctx)
		if err != nil {
			return testing.PollBreak(err)
		}
		if state != vpnConnectedState {
			return errors.Errorf("connection state is %q, not 'CONNECTED'", state)
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		// Log the error message if there is one.
		if err := checkForErrorMessage(ctx, d); err != nil {
			testing.ContextLogf(ctx, "Error message: %q", err)
		}
		return err
	}
	return nil
}

// skipEnableNotifications clicks the "skip" button on the screen for enabling
// notifications.
func skipEnableNotifications(ctx context.Context, d *ui.Device) error {
	testing.ContextLog(ctx, "Skipping enable notifications")
	skipButton := d.Object(ui.ID(vpnPackage + skipEnableNotificationsButtonID))
	if err := skipButton.WaitForExists(ctx, 10*time.Second); err != nil {
		return err
	}
	if err := skipButton.Click(ctx); err != nil {
		return err
	}
	return nil
}

// clickConnectButtonAgain waits for the shield connect button to exist and clicks it.
func clickConnectButtonAgain(ctx context.Context, d *ui.Device) error {
	testing.ContextLog(ctx, "Clicking on Connect button again")
	// Click on the Connect button.
	connectButton := d.Object(ui.ID(vpnPackage + connectShieldButtonID))
	if err := connectButton.WaitForExists(ctx, 10*time.Second); err != nil {
		// If the confirmation didn't pop up, there may be an error message on the screen.
		if err := checkForErrorMessage(ctx, d); err != nil {
			return err
		}
		// Otherwise, return the original error from okButton.WaitForExists().
		return err
	}
	if err := connectButton.Click(ctx); err != nil {
		return err
	}

	return nil
}

// checkForErrorMessage extracts and returns the error message from the UI, if there is one.
func checkForErrorMessage(ctx context.Context, d *ui.Device) error {
	errorMessage := d.Object(ui.ID(vpnPackage + errorMessageTextID))
	// nil error means the error message does exist.
	if err := errorMessage.Exists(ctx); err == nil {
		errorText, err := errorMessage.GetText(ctx)
		if err != nil {
			return errors.Wrap(err, "connection failed and unable get error text")
		}
		return errors.Errorf("Connection failed with error: %q", errorText)
	}

	// There was no error message.
	return nil
}

func dumpLogcatOnError(ctx context.Context, a *arc.ARC, outDir string, hasError func() bool, attemptNum int) {
	if hasError() {
		logcatPath := filepath.Join(outDir, fmt.Sprintf("logcat_attempt_%d.txt", attemptNum))
		a.DumpLogcat(ctx, logcatPath)
	}
}
