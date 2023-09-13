// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/helpapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeOSPageInfo,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Check the ChromeOS page shows enough information to user",
		Contacts: []string{
			"cros-settings@google.com",
			"chromeos-sw-engprod@google.com",
		},
		// OS > Systems > Settings
		BugComponent: "b:1246072",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      3 * time.Minute,
		Fixture:      fixture.ChromeLoggedIn,
	})
}

type chromeOSPageInfo struct {
	cr       *chrome.Chrome
	tconn    *chrome.TestConn
	ui       *uiauto.Context
	settings *ossettings.OSSettings
	chrome   apps.App
}

// ChromeOSPageInfo checks chromeOS version info and help app available to user.
func ChromeOSPageInfo(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	ui := uiauto.New(tconn)

	s.Log("Open setting page and starting test")
	settings, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.AboutChromeOS)
	if err != nil {
		s.Fatal("Failed to open setting page: ", err)
	}
	defer settings.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	chrome, err := apps.ChromeOrChromium(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to find the chrome app: ", err)
	}

	resource := &chromeOSPageInfo{cr, tconn, ui, settings, chrome}

	s.Log("Check ChromeOS version")
	if err := checkVersion(settings)(ctx); err != nil {
		s.Fatal("Failed to check ChromeOS version: ", err)
	}

	s.Log("Check update")
	if err := checkUpdate(settings)(ctx); err != nil {
		s.Fatal("Failed to check update to ChromeOS: ", err)
	}

	s.Log("Check help app")
	if err := checkHelpApp(resource)(ctx); err != nil {
		s.Fatal("Failed to check help app: ", err)
	}

	s.Log("Check feedback app")
	if err := checkSendFeedback(resource)(ctx); err != nil {
		s.Fatal("Failed to check feedback app: ", err)
	}

	s.Log("Check detailed build informations")
	if err := checkDetail(resource)(ctx); err != nil {
		s.Fatal("Failed to check detailed build informations: ", err)
	}

	s.Log("Check open source links")
	if err := checkOpenSources(resource)(ctx); err != nil {
		s.Fatal("Failed to check open source links: ", err)
	}

	s.Log("Check term of services")
	if err := checkTermsOfServiceLinks(resource)(ctx); err != nil {
		s.Fatal("Failed to check term of services: ", err)
	}
}

func checkVersion(settings *ossettings.OSSettings) uiauto.Action {
	return settings.WaitUntilExists(ossettings.VersionInfo)
}

func checkUpdate(settings *ossettings.OSSettings) uiauto.Action {
	return settings.WaitUntilExists(ossettings.CheckUpdateBtn)
}

func checkHelpApp(resource *chromeOSPageInfo) uiauto.Action {
	helpAppCtx := helpapp.NewContext(resource.cr, resource.tconn)

	return uiauto.Combine("Open and close help app",
		resource.settings.LaunchHelpApp(),
		helpAppCtx.WaitForApp(),
		helpAppCtx.HasURL(helpapp.BaseURL),
		helpAppCtx.Close(),
		resource.ui.WaitUntilGone(helpapp.RootFinder),
	)
}

func checkSendFeedback(resource *chromeOSPageInfo) uiauto.Action {
	return uiauto.Combine("Open and close feedback app",
		resource.settings.LeftClick(ossettings.ReportIssue),
		resource.ui.WaitUntilExists(ossettings.FeedbackWindowFinder),
		resource.ui.LeftClick(nodewith.Name("Close").Ancestor(ossettings.FeedbackWindowFinder)),
		resource.ui.WaitUntilGone(ossettings.FeedbackWindowFinder),
	)
}

func checkDetail(resource *chromeOSPageInfo) uiauto.Action {
	detailRoot := nodewith.Name("Chrome - About Version").HasClass("BrowserFrame").Role(role.Window)

	// The "Additional Details" can be off-screen when the screen size is small.
	// Focus before clicking to ensure it is on-screen.
	return uiauto.Combine("click details",
		resource.settings.FocusAndWait(ossettings.AdditionalDetails),
		resource.settings.LeftClick(ossettings.AdditionalDetails),
		func(ctx context.Context) error {
			arr, err := resource.ui.Info(ctx, ossettings.ChangeChannelBtn)
			if err != nil {
				return err
			}
			if arr.HTMLAttributes["aria-disabled"] == "true" {
				return errors.New("change channel button diabled")
			}
			return nil
		},
		resource.settings.LeftClick(ossettings.BuildDetailsBtn),
		resource.ui.WaitUntilExists(nodewith.Name("Platform").Role(role.StaticText).Ancestor(detailRoot)),
		func(ctx context.Context) error { return apps.Close(ctx, resource.tconn, resource.chrome.ID) },
		resource.ui.WaitUntilGone(detailRoot),
		resource.settings.LeftClick(ossettings.BackArrowBtn),
		resource.settings.WaitUntilGone(ossettings.BackArrowBtn),
	)
}

func checkOpenSources(resource *chromeOSPageInfo) uiauto.Action {
	return func(ctx context.Context) error {
		// Focus on the second link to ensure both links are on-screen.
		if err := resource.settings.FocusAndWait(ossettings.OSOpenSourceSoftware)(ctx); err != nil {
			return errors.Wrap(err, "failed to focus on node")
		}

		matchTargetCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		for _, opensource := range []struct {
			node *nodewith.Finder
			url  string
		}{
			{node: ossettings.BrowserOpenSourceSoftware, url: "chrome://credits/"},
			{node: ossettings.OSOpenSourceSoftware, url: "chrome://os-credits/"},
		} {
			testing.ContextLogf(ctx, "Current open source link: %q", opensource.url)
			if err := resource.ui.DoDefault(opensource.node)(ctx); err != nil {
				return errors.Wrap(err, "failed to click on open source link")
			}

			conn, err := resource.cr.NewConnForTarget(matchTargetCtx, chrome.MatchTargetURL(opensource.url))
			if err != nil {
				return errors.Wrap(err, "failed to find expected page")
			}

			if err := conn.CloseTarget(ctx); err != nil {
				return errors.Wrap(err, "failed to close target")
			}
			if err := conn.Close(); err != nil {
				return errors.Wrap(err, "failed to close connection")
			}
		}
		return nil
	}
}

func checkTermsOfServiceLinks(resource *chromeOSPageInfo) uiauto.Action {
	title := "Google Chrome and ChromeOS Additional Terms of Service"
	termsRoot := nodewith.Name("Chrome - " + title).HasClass("BrowserFrame").Role(role.Window)
	termsOfServiceTitle := nodewith.Name(title).Role(role.Heading).Ancestor(termsRoot)

	return uiauto.Combine("click term of service",
		resource.settings.FocusAndWait(ossettings.TermsOfService),
		resource.settings.LeftClick(ossettings.TermsOfService),
		resource.ui.WaitUntilExists(termsOfServiceTitle),
		func(ctx context.Context) error { return apps.Close(ctx, resource.tconn, resource.chrome.ID) },
	)
}
