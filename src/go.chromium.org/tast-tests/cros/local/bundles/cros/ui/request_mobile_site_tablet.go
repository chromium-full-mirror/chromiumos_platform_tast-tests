// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/family"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RequestMobileSiteTablet,
		Desc: "Test request mobile site function on websites under different types of login account",
		Contacts: []string{
			"cj.tsai@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent:   "b:1238037", // ChromeOS > Software > Window Management
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Attr:           []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps:   []string{"chrome", "chrome_internal", "gaia"},
		Timeout:        12 * time.Minute,
		VarDeps: []string{
			family.ParentAccountVarName,
			family.UnicornAccountVarName,
		},
	})
}

type userType string

const (
	guest  userType = "guest_user"
	normal userType = "normal_user"
	child  userType = "child_user"
)

type mobileTestResources struct {
	user                 userType
	ui                   *uiauto.Context
	threeDotMenuBtn      *nodewith.Finder
	requestMobileSiteBtn *nodewith.Finder
	// cr is for faillog.
	cr *chrome.Chrome
	// outDir is for faillog.
	outDir string
}

// RequestMobileSiteTablet tests request mobile site function on websites under different types of login account.
func RequestMobileSiteTablet(ctx context.Context, s *testing.State) {

	pUser, pPass, err := dma.UserPassFromPool(family.ParentAccountVarName)
	if err != nil {
		s.Fatal("Failed to get parent user and pass: ", err)
	}

	uUser, uPass, err := dma.UserPassFromPool(family.UnicornAccountVarName)
	if err != nil {
		s.Fatal("Failed to get parent user and pass: ", err)
	}

	parentCred := chrome.Creds{
		User: pUser,
		Pass: pPass,
	}
	childCred := chrome.Creds{
		User:       uUser,
		Pass:       uPass,
		ParentUser: parentCred.User,
		ParentPass: parentCred.Pass,
	}

	// Dev tools are necessary for the test instrumentation to work, but by default
	// disabled for supervised users. Always force enable them in supervised users tests.
	const extraArg = "--force-devtools-available"

	optsForUser := map[userType][]chrome.Option{
		normal: {chrome.GAIALogin(parentCred)},
		child:  {chrome.GAIALogin(childCred), chrome.ExtraArgs(extraArg)},
		guest:  {chrome.GuestLogin()},
	}

	websites := map[string]string{
		"X":       "https://x.com",
		"YouTube": "https://www.youtube.com",
		"Google":  "http://maps.google.com",
	}

	res := &mobileTestResources{
		requestMobileSiteBtn: nodewith.Name("Request mobile site").Role(role.MenuItem).Ancestor(nodewith.HasClass("SubmenuView")),
		outDir:               s.OutDir(),
	}

	for user, opts := range optsForUser {
		s.Run(ctx, fmt.Sprintf("request mobile site for %s", user), func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
			defer cancel()

			s.Log("Logging in as ", user)
			cr, err := chrome.New(ctx, opts...)
			if err != nil {
				s.Fatal("Failed to sign in: ", err)
			}
			defer cr.Close(cleanupCtx)

			tconn, err := cr.TestAPIConn(ctx)
			if err != nil {
				s.Fatal("Failed to get Test API connection: ", err)
			}

			res.user = user
			res.ui = uiauto.New(tconn)
			res.cr = cr

			cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, true)
			if err != nil {
				s.Fatal("Failed to enable the tablet mode: ", err)
			}
			defer cleanup(cleanupCtx)

			// Guest has no left off setting.
			if user != guest {
				if err := ensureLeftOffSettingEnabled(ctx, cr, res); err != nil {
					s.Fatal("Failed to turn on left off setting: ", err)
				}
			}

			for websiteName, url := range websites {
				browserRoot := nodewith.Role(role.Window).HasClass("BrowserFrame").NameContaining(websiteName)
				res.threeDotMenuBtn = nodewith.HasClass("BrowserAppMenuButton").Role(role.PopUpButton).Ancestor(nodewith.HasClass("ToolbarView").Role(role.Toolbar).Ancestor(browserRoot))
				if err := mobileSiteTest(ctx, cr, res, websiteName, url); err != nil {
					s.Fatalf("Failed to run mobileSiteTest on website %q: %v", websiteName, err)
				}
			}
		})
	}
}

// mobileSiteTest verifies the expected mobile site status when "request mobile site" is on and off,
// then revisits the website to verify that "request mobile site" is still on.
func mobileSiteTest(ctx context.Context, cr *chrome.Chrome, res *mobileTestResources, websiteName, url string) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	conn, err := cr.NewConn(ctx, url)
	if err != nil {
		return errors.Wrapf(err, "failed to open page %q", url)
	}
	defer func(ctx context.Context) {
		faillog.DumpUITreeWithScreenshotOnError(ctx, res.outDir, func() bool { return retErr != nil }, res.cr, fmt.Sprintf("ui_dump_%s", res.user))
		if err := conn.CloseTarget(ctx); err != nil {
			testing.ContextLogf(ctx, "Failed to close website %q: %v", url, err)
		}
		if err := conn.Close(); err != nil {
			testing.ContextLogf(ctx, "Failed to close test connection to website %q: %v", url, err)
		}
	}(cleanupCtx)

	testing.ContextLog(ctx, "Waiting for website to achieve quiescence")
	if err := webutil.WaitForQuiescence(ctx, conn, time.Minute); err != nil {
		return errors.Wrap(err, "failed to wait for website to achieve quiescence")
	}

	testing.ContextLog(ctx, `Turning on "request mobile site"`)
	if err := requestMobileSiteAndVerify(ctx, conn, websiteName, res, true); err != nil {
		return errors.Wrap(err, `failed to turn on "request mobile site"`)
	}

	testing.ContextLog(ctx, `Turning off "request mobile site"`)
	if err := requestMobileSiteAndVerify(ctx, conn, websiteName, res, false); err != nil {
		return errors.Wrap(err, `failed to turn off "request mobile site"`)
	}

	testing.ContextLog(ctx, `Turning on "request mobile site"`)
	if err := requestMobileSiteAndVerify(ctx, conn, websiteName, res, true); err != nil {
		return errors.Wrap(err, `failed to turn on "request mobile site"`)
	}

	testing.ContextLogf(ctx, "Revisiting website %q", url)
	if err := conn.Navigate(ctx, url); err != nil {
		return errors.Wrapf(err, "failed to navigate to %q", url)
	}

	if err := verifyMobileSite(ctx, conn, res, true); err != nil {
		return errors.Wrap(err, `failed to verify "request mobile site"`)
	}
	return nil
}

// requestMobileSiteAndVerify opens the three dot menu, clicks the "request mobile site"
// button, waits for the website to be stable, and verifies the expected mobile site status.
func requestMobileSiteAndVerify(ctx context.Context, conn *chrome.Conn, websiteName string, res *mobileTestResources, expected bool) error {
	if err := uiauto.Combine(`select "request mobile site"`,
		res.ui.LeftClick(res.threeDotMenuBtn),
		res.ui.LeftClick(res.requestMobileSiteBtn),
		webutil.WaitForQuiescenceAction(conn, time.Minute),
	)(ctx); err != nil {
		return err
	}

	if err := verifyMobileSite(ctx, conn, res, expected); err != nil {
		return errors.Wrap(err, "failed to verify mobile site status")
	}
	return nil
}

// verifyMobileSite verifies the mobile site settings by fetching the user agent data from the browser.
// Note: The button is unavailable in the a11y tree. Using navigator API as a workaround.
func verifyMobileSite(ctx context.Context, conn *chrome.Conn, res *mobileTestResources, expected bool) error {
	// The user agent might not be loaded immediately after the settings is changed.
	return testing.Poll(ctx, func(ctx context.Context) error {
		var enabled bool
		if err := conn.Eval(ctx, "navigator.userAgentData.mobile", &enabled); err != nil {
			return errors.Wrap(err, "failed to get mobile site status")
		}
		if enabled != expected {
			return errors.Errorf("failed to verify mobile site status; got: %v, want: %v", enabled, expected)
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Minute, Interval: time.Second})
}

// ensureLeftOffSettingEnabled opens chrome://settings/onStartup, ensures that "continue
// where you left off" is turned on, and then closes chrome://settings/onStartup.
func ensureLeftOffSettingEnabled(ctx context.Context, cr *chrome.Chrome, res *mobileTestResources) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	const startupPage = "chrome://settings/onStartup"
	conn, err := cr.NewConn(ctx, startupPage)
	if err != nil {
		return errors.Wrapf(err, "failed to open %q", startupPage)
	}
	defer func(ctx context.Context) {
		faillog.DumpUITreeWithScreenshotOnError(ctx, res.outDir, func() bool { return retErr != nil }, res.cr, fmt.Sprintf("ui_dump_left_off_%s", res.user))
		if err := conn.CloseTarget(ctx); err != nil {
			testing.ContextLogf(ctx, "Failed to close website %q: %v", startupPage, err)
		}
		if err := conn.Close(); err != nil {
			testing.ContextLogf(ctx, "Failed to close test connection to website %q: %v", startupPage, err)
		}
	}(cleanupCtx)

	continueLeftOff := nodewith.Name("Continue where you left off").Role(role.RadioButton)
	verify := res.ui.WaitUntilExists(continueLeftOff.Attribute("checked", "true"))
	return uiauto.IfFailThen(
		verify,
		uiauto.Combine(`turn on "continue where you left off"`,
			res.ui.LeftClick(continueLeftOff),
			verify,
		),
	)(ctx)
}
