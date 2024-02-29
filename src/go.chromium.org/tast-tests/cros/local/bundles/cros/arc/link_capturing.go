// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	arcui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	testPackageName = "org.chromium.arc.testapp.linkcapturing"
	testIntentText  = testPackageName + ":id/intent_text"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LinkCapturing,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies link capturing integration between ARC and the browser",
		Contacts: []string{
			"chromeos-apps-foundation-team@google.com",
			"tsergeant@chromium.org",
		},
		BugComponent: "b:1203766",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
			Fixture:           "arcBooted",
			Val:               browser.TypeAsh,
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			Fixture:           "arcBooted",
			Val:               browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"android_container", "lacros"},
			Fixture:           "lacrosWithArcBooted",
			Val:               browser.TypeLacros,
		}, {
			Name:              "lacros_vm",
			ExtraSoftwareDeps: []string{"android_vm", "lacros"},
			Fixture:           "lacrosWithArcBooted",
			Val:               browser.TypeLacros,
		}},
		Timeout: chrome.LoginTimeout + arc.BootTimeout + 1*time.Minute,
		Data: []string{
			"link_capturing/link_capturing_index.html",
			"link_capturing/app/app_index.html",
		},
	})

}

// Specifies the value of the "Opening supported links" setting.
type openInSetting int

const (
	openInDefault openInSetting = iota
	openInApp
	openInBrowser
)

// Specifies where a supported link should be clicked.
type clickLocation int

const (
	clickInBrowser clickLocation = iota
	clickInAndroid
)

// Specifies where the link click should be handled.
type linkOpenLocation int

const (
	linkOpensInBrowser linkOpenLocation = iota
	linkOpensInAndroid
	linkOpensInAndroidPicker
)

func LinkCapturing(ctx context.Context, s *testing.State) {
	const (
		// ArcLinkCapturingTest accepts links on port 8000.
		serverPort = 8000
		testApk    = "ArcLinkCapturingTest.apk"
	)

	cr := s.FixtValue().(*arc.PreData).Chrome
	arcDevice := s.FixtValue().(*arc.PreData).ARC
	uiAutomator := s.FixtValue().(*arc.PreData).UIDevice

	// Give 5 seconds to clean up and dump out UI tree.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Setup Test API Connection.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Setup ARC and install APK.
	if err := arcDevice.WaitIntentHelper(ctx); err != nil {
		s.Fatal("Failed waiting for intent helper: ", err)
	}

	if err := arcDevice.Install(ctx, arc.APKPath(testApk)); err != nil {
		s.Fatal("Failed installing the APK: ", err)
	}

	recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn)
	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "screen_recording.webm"), s.HasError)

	// Open chrome://app-service-internals to and download the installed apps.
	// This is called for both succeeded cases and failed cases to compare.
	// The whole process should not fail the test case.
	// TODO (crbug/1416707): to remove this part once the bug crbug/1416707 is resolved.
	defer func() {
		if err := apps.DownloadAppServiceInternals(ctx, cr, s.OutDir()); err != nil {
			s.Log("Failed to download app service internal: ", err)
		}
	}()

	// Enable link capturing on the ARC side. Automatically verifying the link
	// (as per https://developer.android.com/training/app-links/verify-site-associations)
	// is difficult in a test environment, so this is a shortcut which has the
	// same visible impact.
	// PackageManager commands for preferring links change across Android versions.
	version, err := arc.SDKVersion()
	if err != nil {
		s.Fatal("Failed to get ARC version: ", err)
	}

	openInBrowserFromAndroidExpectation := linkOpensInBrowser
	if version >= arc.SDKT {
		if err := arcDevice.Command(ctx, "pm", "set-app-links", "--package", testPackageName, "2", "all").Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to set Android link capturing setting: ", err)
		}
	} else {
		if err := arcDevice.Command(ctx, "pm", "set-app-link", testPackageName, "always").Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to set Android link capturing setting: ", err)
		}

		// On Android R or before, disambiguation dialog was the defined behavior
		// as in https://developer.android.com/training/app-links#web-links,
		// which was different from the latest behavior
		openInBrowserFromAndroidExpectation = linkOpensInAndroidPicker
	}

	// Start local server.
	server := &http.Server{Addr: fmt.Sprintf(":%d", serverPort), Handler: http.FileServer(s.DataFileSystem())}
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			s.Fatal("Failed to create local server: ", err)
		}
	}()
	defer server.Shutdown(ctx)

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to open browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	for _, tc := range []struct {
		name    string
		setting openInSetting
		link    clickLocation
		opensIn linkOpenLocation
	}{
		{
			name:    "default_link_in_browser_stay_in_browser",
			setting: openInDefault,
			link:    clickInBrowser,
			opensIn: linkOpensInBrowser,
		},
		{
			name:    "default_link_in_android_stay_in_android",
			setting: openInDefault,
			link:    clickInAndroid,
			opensIn: linkOpensInAndroid,
		},
		{
			name:    "app_setting_link_in_browser_opens_app",
			setting: openInApp,
			link:    clickInBrowser,
			opensIn: linkOpensInAndroid,
		},
		{
			name:    "browser_setting_link_in_android_opens_picker",
			setting: openInBrowser,
			link:    clickInAndroid,
			opensIn: openInBrowserFromAndroidExpectation,
		},
	} {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			if tc.setting != openInDefault {
				if err := changeLinkCapturingSetting(ctx, tconn, cr, tc.setting == openInApp, s.OutDir()); err != nil {
					s.Fatal("Failed to change link capturing setting: ", err)
				}
			}

			var verifier uiauto.Action

			if tc.opensIn == linkOpensInBrowser {
				heading := nodewith.Name("In-scope page").Role(role.Heading)
				ui := uiauto.New(tconn)
				verifier = ui.WaitUntilExists(heading)
			} else if tc.opensIn == linkOpensInAndroid {
				verifier = func(ctx context.Context) error {
					return uiAutomator.Object(arcui.ID(testIntentText)).WaitForExists(ctx, 10*time.Second)
				}
			} else if tc.opensIn == linkOpensInAndroidPicker {
				verifier = func(ctx context.Context) error {
					return uiAutomator.Object(arcui.Text("Open with")).WaitForExists(ctx, 10*time.Second)
				}
			}

			if tc.link == clickInBrowser {
				if err := clickBrowserLinkAndVerify(ctx, tconn, br, verifier); err != nil {
					s.Fatal("Failed to verify link click: ", err)
				}
			} else {
				if err := clickAndroidLinkAndVerify(ctx, tconn, arcDevice, uiAutomator, verifier); err != nil {
					s.Fatal("Failed to verify link click: ", err)
				}
			}
		})
	}
}

// clickBrowserLinkAndVerify clicks a link in a browser tab, then calls
// verifier to verify the device state. verifier is passed as an Action to
// allow cleanup after verification is completed.
func clickBrowserLinkAndVerify(ctx context.Context, tconn *chrome.TestConn, br *browser.Browser, verifier uiauto.Action) error {
	const testPageURL = "http://127.0.0.1:8000/link_capturing/link_capturing_index.html"

	conn, err := br.NewConn(ctx, testPageURL)
	if err != nil {
		return errors.Wrap(err, "failed to open test page in browser")
	}
	defer conn.Close()

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)
	link := nodewith.Name("In-scope link").Role(role.Link)

	return ui.LeftClickUntil(link, verifier)(ctx)
}

// clickAndroidLinkAndVerify clicks a link in Android, then calls verifier to
// verify the device state. verifier is passed as an Action to allow cleanup
// after verification is completed.
func clickAndroidLinkAndVerify(ctx context.Context, tconn *chrome.TestConn, arcDevice *arc.ARC, uiAutomator *arcui.Device, verifier uiauto.Action) error {
	const (
		testActivity     = testPackageName + ".MainActivity"
		testIntentButton = testPackageName + ":id/link_action"
	)

	activity, err := arc.NewActivity(arcDevice, testPackageName, testActivity)
	if err != nil {
		return errors.Wrap(err, "failed to create a new activity")
	}
	defer activity.Close(ctx)
	if err := activity.Start(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to start the test activity")
	}
	defer activity.Stop(ctx, tconn)

	intentButton := uiAutomator.Object(arcui.ID(testIntentButton))
	if err := intentButton.WaitForExists(ctx, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to for intent button to appear")
	}

	return uiauto.Combine("click link and verify result",
		intentButton.Click,
		verifier)(ctx)
}

// changeLinkCapturingSetting changes the "Open supported links" setting in app
// management for the test app. If openInApp is true, the setting will be
// enabled and links will open in the app. Otherwise, the setting will be
// disabled and links will open in the browser.
func changeLinkCapturingSetting(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, openInApp bool, outDir string) error {
	const (
		testAppName                = "Link Capturing Test App"
		testAppID                  = "cacnggingocklkpmmmniidnncakhjgob"
		linkCapturingOpenInApp     = "Open in " + testAppName + " app"
		linkCapturingOpenInBrowser = "Open in Chrome browser"
	)

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)
	appHeader := nodewith.Name(testAppName).Role(role.Heading).Ancestor(ossettings.WindowFinder)
	osSettings, err := ossettings.LaunchAtAppMgmtPage(ctx, tconn, cr, testAppID, ui.Exists(appHeader))
	if err != nil {
		return errors.Wrap(err, "failed to open OS Settings")
	}
	defer osSettings.Close(ctx)

	settingRadioButton := nodewith.Name(linkCapturingOpenInBrowser).Role(role.RadioButton)
	if openInApp {
		settingRadioButton = nodewith.Name(linkCapturingOpenInApp).Role(role.RadioButton)
	}

	if err := ui.LeftClickUntil(settingRadioButton, ui.WaitUntilCheckedState(settingRadioButton, true))(ctx); err != nil {
		// Dump UI tree before OS Settings closes to help diagnose timeouts.
		faillog.DumpUITreeToFile(ctx, outDir, tconn, "app_management_ui_tree.txt")
		return err
	}
	return nil
}
