// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package networkui

import (
	"context"
	"reflect"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/networkui/proxysettings"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// proxyRetainTestParam is a parameter for ProxyRetain test.
type proxyRetainTestParam struct {
	description string
	test        proxyRetainTest
	loginUsers  []chrome.Option
}

// Define account information for primary user and secondary user.
var (
	primaryUser   chrome.Creds = chrome.Creds{User: "testuser1@gmail.com", Pass: "123456"}
	secondaryUser chrome.Creds = chrome.Creds{User: "testuser2@gmail.com", Pass: "123456"}
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ProxyRetain,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies that the proxy settings will be retained after login or across different users",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent: "b:1318544", // ChromeOS > Software > System Services > Connectivity > General
		Attr:         []string{"group:network", "network_e2e"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"ui.signinProfileTestExtensionManifestKey"},
		Fixture:      "shillReset",
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Name: "after_login",
				Val: &proxyRetainTestParam{
					description: "Proxy information retained after login",
					test:        &retainAfterLoginTest{},
					loginUsers: []chrome.Option{
						chrome.FakeLogin(primaryUser),
					},
				},
			}, {
				Name: "across_users",
				Val: &proxyRetainTestParam{
					description: "Proxy information retained after logging to different types of users",
					test:        &retainAcrossUsersTest{},
					loginUsers: []chrome.Option{
						chrome.FakeLogin(primaryUser),
						chrome.FakeLogin(secondaryUser),
						chrome.GuestLogin(),
					},
				},
			},
		},
	})
}

// proxyRetainResource holds resources for ProxyRetain test.
type proxyRetainResource struct {
	tconn       *chrome.TestConn
	ui          *uiauto.Context
	kb          *input.KeyboardEventWriter
	outDir      string
	manifestKey string
}

// ProxyRetain verifies proxy settings will be retained.
func ProxyRetain(ctx context.Context, s *testing.State) {
	param := s.Param().(*proxyRetainTestParam)

	// proxyValues holds proxy hosts and ports for http, https and socks.
	proxyValues := []*proxysettings.Config{
		{
			Protocol: proxysettings.HTTP,
			Host:     "localhost",
			Port:     "123",
		},
		{
			Protocol: proxysettings.HTTPS,
			Host:     "localhost",
			Port:     "456",
		},
		{
			Protocol: proxysettings.Socks,
			Host:     "socks5://localhost",
			Port:     "8080",
		},
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	resources := &proxyRetainResource{
		kb:     kb,
		outDir: s.OutDir(),
	}

	if _, ok := param.test.(*retainAfterLoginTest); ok {
		// The retainAfterLoginTest test requires the manifest key to do operations without login.
		resources.manifestKey = s.RequiredVar("ui.signinProfileTestExtensionManifestKey")
	}

	cr, err := chrome.New(ctx, chrome.FakeLogin(primaryUser))
	if err != nil {
		s.Fatal("Failed to create primary user: ", err)
	}
	if err := cr.Close(ctx); err != nil {
		s.Log("Failed to close Chrome: ", err)
	}

	if err := param.test.preparationAtLoginScreen(ctx, resources, proxyValues); err != nil {
		s.Fatalf("Failed to set proxy at login screen for test %q: %v", param.description, err)
	}

	if err := param.test.preparationAfterLoggedIn(ctx, resources, proxyValues); err != nil {
		s.Fatalf("Failed to set proxy after logged in for test %q: %v", param.description, err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	s.Logf("Looping proxy verification for test %q", param.description)
	for _, loginOpt := range param.loginUsers {
		func() {
			cr, err := startChrome(ctx, resources, false /* isNoLogin */, loginOpt)
			if err != nil {
				s.Fatal("Failed to sign in: ", err)
			}
			defer cr.Close(cleanupCtx)
			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "verify_proxy_ui_dump")

			proxyManager := proxysettings.NewProxySettingsManager(proxysettings.LoggedIn)
			if err := proxyManager.LaunchAndPrepare(ctx, cr, resources.tconn, proxysettings.Ethernet()); err != nil {
				s.Fatal("Failed to launch proxy settings instance: ", err)
			}
			defer proxyManager.Close(cleanupCtx, cr, resources.tconn)

			// Verify proxy values.
			for _, pv := range proxyValues {
				if resultPv, err := proxyManager.ManualConfigContent(ctx, resources.tconn, pv.Protocol); err != nil {
					s.Fatalf("Failed to get proxy value for %q: %v", pv.Protocol.Name(), err)
				} else if !reflect.DeepEqual(resultPv, pv) {
					s.Fatalf("Failed to verify proxy value for %q: got %q, want %q", pv.Protocol.Name(), resultPv, pv)
				}
			}
		}()
	}
}

// startChrome starts the Chrome with specified configurations and returns the Chrome instance.
// It also reestablish other resources associated with the chrome.Chrome instance.
func startChrome(ctx context.Context, res *proxyRetainResource, isNoLogin bool, loginOpt ...chrome.Option) (cr *chrome.Chrome, retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	opts := append(loginOpt, chrome.KeepState())
	if isNoLogin {
		opts = append(opts,
			chrome.NoLogin(),
			chrome.LoadSigninProfileExtension(res.manifestKey),
		)
	}

	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			if err := cr.Close(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to close Chrome: ", err)
			}
		}
	}(cleanupCtx)

	getTconn := cr.TestAPIConn
	if isNoLogin {
		getTconn = cr.SigninProfileTestAPIConn
	}

	tconn, err := getTconn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get Test API connection")
	}
	res.tconn = tconn
	res.ui = uiauto.New(tconn)

	return cr, nil
}

type proxyRetainTest interface {
	// preparationAtLoginScreen prepares the test environment at the login screen.
	preparationAtLoginScreen(context.Context, *proxyRetainResource, []*proxysettings.Config) error

	// preparationAfterLoggedIn prepares the test environment after logged in.
	preparationAfterLoggedIn(context.Context, *proxyRetainResource, []*proxysettings.Config) error
}

// retainAfterLoginTest is a test case structure for the proxy retain after login.
type retainAfterLoginTest struct{}

func (t *retainAfterLoginTest) preparationAtLoginScreen(ctx context.Context, res *proxyRetainResource, pvs []*proxysettings.Config) (retErr error) {
	testing.ContextLog(ctx, "Setting up proxy at login screen")

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := startChrome(ctx, res, true /* isNoLogin */)
	if err != nil {
		return errors.Wrap(err, "failed to sign in")
	}
	defer cr.Close(cleanupCtx)
	defer faillog.DumpUITreeOnErrorToFile(cleanupCtx, res.outDir, func() bool { return retErr != nil }, res.tconn, "before_login_ui_dump")

	proxyManager := proxysettings.NewProxySettingsManager(proxysettings.SignInScreen)
	if err := proxyManager.LaunchAndPrepare(ctx, cr, res.tconn, proxysettings.Ethernet()); err != nil {
		return errors.Wrap(err, "failed to launch proxy settings instance")
	}
	defer proxyManager.Close(cleanupCtx, cr, res.tconn)

	if err := proxyManager.SetManualConfig(ctx, res.tconn, res.kb, pvs); err != nil {
		return errors.Wrap(err, "failed to set proxy fields")
	}

	return nil
}

func (t *retainAfterLoginTest) preparationAfterLoggedIn(ctx context.Context, res *proxyRetainResource, pvs []*proxysettings.Config) error {
	return nil
}

// retainAcrossUsersTest is a test case structure for the proxy retain across users.
type retainAcrossUsersTest struct{}

func (t *retainAcrossUsersTest) preparationAtLoginScreen(ctx context.Context, res *proxyRetainResource, pvs []*proxysettings.Config) error {
	return nil
}

func (t *retainAcrossUsersTest) preparationAfterLoggedIn(ctx context.Context, res *proxyRetainResource, pvs []*proxysettings.Config) (retErr error) {
	testing.ContextLog(ctx, "Setting up proxy after logged in")

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := startChrome(ctx, res, false /* isNoLogin */, chrome.FakeLogin(primaryUser))
	if err != nil {
		return errors.Wrap(err, "failed to sign in")
	}
	defer cr.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, res.outDir, func() bool { return retErr != nil }, cr, "after_login_ui_dump")

	proxyManager := proxysettings.NewProxySettingsManager(proxysettings.LoggedIn)
	if err := proxyManager.LaunchAndPrepare(ctx, cr, res.tconn, proxysettings.Ethernet()); err != nil {
		return errors.Wrap(err, "failed to launch proxy settings instance")
	}
	defer proxyManager.Close(cleanupCtx, cr, res.tconn)

	if err := proxyManager.SetManualConfig(ctx, res.tconn, res.kb, pvs); err != nil {
		return errors.Wrap(err, "failed to set proxy fields")
	}

	return nil
}
