// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	androidui "go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
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
	perAppLangTestAppID         = "ljodfcljdagbflnefogibkoibobehcah"
	perAppLangTestAppName       = "ARC Locale Changer Test"
	perAppLangTestApkFileName   = "ArcLocaleChangerTest.apk"
	perAppLangTestPackageName   = "org.chromium.arc.testapp.localechanger"
	perAppLangTestActvitityName = ".MainActivity"
	englishUSLocale             = "en_US"
	englishUSLanguage           = "English (United States)"
	frenchFRLocale              = "fr_FR"
	frenchFRLanguage            = "French (France)"
	spanishUSLocale             = "es_US"
	spanishUSLanguage           = "Spanish (United States)"
	deviceLanguageLabel         = "Device language"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChangeAppLanguage,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies integration of ARC apps with ChromeOS app language settings",
		Contacts:     []string{"arc-framework+tast@google.com", "nergi@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Chrome Integration
		BugComponent: "b:537221",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		// Per-App Language is currently only enabled on ARC-T.
		SoftwareDeps: []string{"chrome", "android_vm", "no_android_vm_r"},
		Timeout:      10 * time.Minute,
	})
}

// ChangeAppLanguage tests integration of ARC apps with ChromeOS app language settings.
func ChangeAppLanguage(ctx context.Context, s *testing.State) {
	// Give 10 seconds to clean up and dump out UI tree.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx,
		chrome.Region("en"),
		chrome.ARCEnabled(),
		chrome.EnableFeatures("PerAppLanguage"),
		chrome.UnRestrictARCCPU())
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	if err := ensureDeviceLanguageIsSetToEnglish(ctx, tconn); err != nil {
		s.Fatal("Device language failed to be set: ", err)
	}

	// Ensure device in clamshell mode to keep both ChromeOS Settings app and
	// Android test app opened.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure the device is in clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)
	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)
	d, err := a.NewUIDevice(ctx)
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	// Install and start activity.
	s.Log("Installing app")
	if err := a.Install(ctx, arc.APKPath(perAppLangTestApkFileName)); err != nil {
		s.Fatal("Failed installing app: ", err)
	}
	act, err := arc.NewActivity(a, perAppLangTestPackageName, perAppLangTestActvitityName)
	if err != nil {
		s.Fatal("Failed to create an activity: ", err)
	}
	defer act.Close(cleanupCtx)
	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatal("Failed to start an activity: ", err)
	}
	defer act.Stop(cleanupCtx, tconn)

	s.Run(ctx, "Verify changing app language from Android", func(ctx context.Context, s *testing.State) {
		// Setup ChromeOS page
		osSettings, err := openAppDetailPage(ctx, tconn, cr, ui)
		if err != nil {
			s.Fatal("Failed to open AppDetail Page: ", err)
		}
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx,
			s.OutDir(),
			s.HasError,
			cr,
			"changing_app_language_from_android_test_ui_tree")
		defer func() {
			if err := osSettings.Close(cleanupCtx); err != nil {
				s.Error("Failed to close AppDetail page: ", err)
			}
		}()
		// Ensure system default
		verifyAction := appDetailPageVerifyAction(osSettings, deviceLanguageLabel)
		if err := ensureAppLanguageSetAsSystemDefault(ctx, d, verifyAction); err != nil {
			s.Fatal("Failed to verify default state: ", err)
		}
		// Change and assert (French)
		frenchLanguageButtonID := perAppLangTestPackageName + ":id/french_button_text"
		verifyAction = appDetailPageVerifyAction(osSettings, frenchFRLanguage)
		if err := changeLanguageFromAppAndVerify(ctx, d, frenchLanguageButtonID, frenchFRLocale, verifyAction); err != nil {
			s.Fatal("Failed to change app language from Android: ", err)
		}
	})

	s.Run(ctx, "Verify changing app language from ChromeOS AppDetail page", func(ctx context.Context, s *testing.State) {
		// Setup ChromeOS page
		osSettings, err := openAppDetailPage(ctx, tconn, cr, ui)
		if err != nil {
			s.Fatal("Failed to open AppDetail page: ", err)
		}
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx,
			s.OutDir(),
			s.HasError,
			cr,
			"changing_app_language_from_cros_app_detail_test_ui_tree")
		defer func() {
			if err := osSettings.Close(cleanupCtx); err != nil {
				s.Error("Failed to close AppDetail page: ", err)
			}
		}()
		// Ensure system default
		verifyAction := appDetailPageVerifyAction(osSettings, deviceLanguageLabel)
		if err := ensureAppLanguageSetAsSystemDefault(ctx, d, verifyAction); err != nil {
			s.Fatal("Failed to verify default state: ", err)
		}
		// Change (Spanish)
		openDialogAction := osSettings.LeftClick(nodewith.NameContaining("App language"))
		if err := changeLanguageFromAppLanguageDialog(ctx, osSettings, spanishUSLanguage, openDialogAction); err != nil {
			s.Fatal("Failed to change app language from AppDetail page: ", err)
		}
		// Assert
		if err := verifyAppLanguageInARC(ctx, d, spanishUSLocale); err != nil {
			s.Fatal("Failed to verify app language in ARC: ", err)
		}
		if err := appDetailPageVerifyAction(osSettings, spanishUSLanguage)(ctx); err != nil {
			s.Fatal("Failed to verify app language in AppDetail page: ", err)
		}
	})

	s.Run(ctx, "Verify changing app language from ChromeOS AppLanguages page", func(ctx context.Context, s *testing.State) {
		// Setup ChromeOS page
		// Open Languages > AppLanguages page
		osSettings, err := ossettings.LaunchAtLanguageSettingsPage(ctx, tconn, cr)
		if err != nil {
			s.Fatal("Failed to open Languages Page: ", err)
		}
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx,
			s.OutDir(),
			s.HasError,
			cr,
			"changing_app_language_from_cros_app_languages_test_ui_tree")
		defer func() {
			if err := osSettings.Close(cleanupCtx); err != nil {
				s.Error("Failed to close Languages page: ", err)
			}
		}()
		appLanguagesButton := nodewith.NameContaining("App languages")
		if err := uiauto.Combine("open AppLanguages page",
			osSettings.WaitUntilExists(appLanguagesButton),
			osSettings.LeftClick(appLanguagesButton),
		)(ctx); err != nil {
			s.Fatal("Failed to open AppLanguages page: ", err)
		}
		// Ensure system default
		verifyAction := appLanguagesPageVerifyAction(osSettings, deviceLanguageLabel)
		if err := ensureAppLanguageSetAsSystemDefault(ctx, d, verifyAction); err != nil {
			s.Fatal("Failed to verify default state: ", err)
		}
		// Change (French)
		threeDotsButton := nodewith.NameContaining(perAppLangTestAppName).HasClass("icon-more-vert").Role(role.Button)
		ediLanguageSelection := nodewith.NameContaining("Edit language selection").First()
		openDialogAction := uiauto.Combine("open app dialog",
			osSettings.LeftClick(threeDotsButton),
			osSettings.LeftClick(ediLanguageSelection))
		if err := changeLanguageFromAppLanguageDialog(ctx, osSettings, frenchFRLanguage, openDialogAction); err != nil {
			s.Fatal("Failed to change app language from AppLanguages page: ", err)
		}
		// Assert
		if err := verifyAppLanguageInARC(ctx, d, frenchFRLocale); err != nil {
			s.Fatal("Failed to verify app language in ARC: ", err)
		}
		if err := appLanguagesPageVerifyAction(osSettings, regexp.QuoteMeta(frenchFRLanguage))(ctx); err != nil {
			s.Fatal("Failed to verify app language in AppLanguages page: ", err)
		}
	})
}

// ====================
// ARC-specific methods
// ====================

// ensureAppLanguageSetAsSystemDefault in both ARC and corresponding
// ChromeOS settings page.
func ensureAppLanguageSetAsSystemDefault(ctx context.Context,
	d *androidui.Device,
	osSettingsPageVerifyAction action.Action) error {
	systemDefaultButtonID := perAppLangTestPackageName + ":id/default_button_text"
	// System default should be set as "en_US" as region has been set as "us".
	return changeLanguageFromAppAndVerify(ctx, d, systemDefaultButtonID, englishUSLocale, osSettingsPageVerifyAction)
}

// changeLanguageFromAppAndVerify in both ARC and corresponding
// ChromeOS settings page.
func changeLanguageFromAppAndVerify(ctx context.Context,
	d *androidui.Device,
	appChangeLanguageButtonID,
	appLanguageLocale string,
	osSettingsPageVerifyAction action.Action) error {
	button := d.Object(androidui.ID(appChangeLanguageButtonID))
	if err := button.WaitForExists(ctx, 5*time.Second); err != nil {
		return errors.Wrap(err, "failed to find button")
	}
	if err := button.Click(ctx); err != nil {
		return errors.Wrap(err, "failed to click system default language button")
	}
	if err := verifyAppLanguageInARC(ctx, d, appLanguageLocale); err != nil {
		return errors.Wrap(err, "failed to verify app language in ARC")
	}
	return osSettingsPageVerifyAction(ctx)
}

// verifyAppLanguageInARC to match given locale.
func verifyAppLanguageInARC(ctx context.Context, d *androidui.Device, appLanguageLocale string) error {
	appLanguageTextID := perAppLangTestPackageName + ":id/locale_text"
	applanguageText := d.Object(androidui.ID(appLanguageTextID), androidui.TextContains(appLanguageLocale))
	return applanguageText.WaitForExists(ctx, 5*time.Second)
}

// ==============================
// AppDetailPage-specific methods
// ==============================

// openAppDetailPage to open AppManagement > AppDetail page
func openAppDetailPage(ctx context.Context,
	tconn *chrome.TestConn,
	cr *chrome.Chrome,
	ui *uiauto.Context) (*ossettings.OSSettings, error) {
	appHeader := nodewith.Name(perAppLangTestAppName).Role(role.Heading).Ancestor(ossettings.WindowFinder)
	return ossettings.LaunchAtAppMgmtPage(ctx, tconn, cr, perAppLangTestAppID, ui.Exists(appHeader))
}

// appDetailPageVerifyAction to verify app language name in AppDetail page.
func appDetailPageVerifyAction(appDetailPage *ossettings.OSSettings, appLanguageName string) action.Action {
	return appDetailPage.WaitUntilExists(nodewith.NameContaining(appLanguageName))
}

// =================================
// AppLanguagesPage-specific methods
// =================================

// appLanguagesPageVerifyAction to verify app language name in
// AppLanguages page.
func appLanguagesPageVerifyAction(appLanguagesPage *ossettings.OSSettings, appLanguageName string) action.Action {
	// Verify through `icon-more-vert` button because app name and
	// app language are set as `aria-hidden` and instead attached to
	// `icon-more-vert` a11y button label.
	appLanguageLabel := nodewith.NameRegex(regexp.MustCompile(perAppLangTestAppName + ".*" + appLanguageName)).HasClass("icon-more-vert")
	return appLanguagesPage.WaitUntilExists(appLanguageLabel)
}

// =====================
// Common helper methods
// =====================

// changeLanguageFromAppLanguageDialog in corresponding ChromeOS settings page.
func changeLanguageFromAppLanguageDialog(ctx context.Context,
	osSettings *ossettings.OSSettings,
	appLanguageName string,
	openDialogAction action.Action) error {
	selectLanguage := nodewith.NameContaining(appLanguageName).First()
	confirmButton := nodewith.Name("Update").Role(role.Button)
	return uiauto.Combine("change app language",
		openDialogAction,
		osSettings.LeftClick(selectLanguage),
		osSettings.LeftClick(confirmButton),
	)(ctx)
}

// ensureDeviceLanguageIsSetToEnglish verify UI Language is set to en-US.
func ensureDeviceLanguageIsSetToEnglish(ctx context.Context, tconn *chrome.TestConn) error {
	var lang string
	if err := tconn.Eval(ctx, "chrome.i18n.getUILanguage()", &lang); err != nil {
		return errors.Wrap(err, "failed to call chrome.i18n.getUILanguage")
	}
	if lang != "en-US" {
		return errors.Errorf("language is still set to %s", lang)
	}
	return nil
}
