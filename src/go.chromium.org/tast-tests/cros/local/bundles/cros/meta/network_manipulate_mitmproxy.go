// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/proxy/mitmproxy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	httpRedirect       = "mitmproxy_redirect_requests.py"
	httpErrorInject    = "mitmproxy_inject_500_requests.py"
	allowedEndpoints   = "allowed_endpoints.py"
	extraConfig        = "allowed_endpoints_yaml.py"
	endpoints          = "endpoints.yml"
	discoveryEndpoints = "discovery_traffic.py"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NetworkManipulateMitmproxy,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that manipulating network traffic with mitmproxy",
		Contacts: []string{
			"cros-ufo-testing@google.com",
			"yanghenry@google.com",
		},
		BugComponent: "b:1359643", // ChromeOS > EngProd > Software > Trust & Safety
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Fixture:      fixture.ChromeLoggedIn,
		Params: []testing.Param{{
			Name:      "redirect",
			Val:       "redirect",
			ExtraData: []string{httpRedirect},
		}, {
			Name: "dump",
			Val:  "dump",
		}, {
			Name:      "error",
			Val:       "error",
			ExtraData: []string{httpErrorInject},
		}, {
			Name:      "discovery",
			Val:       "discovery",
			ExtraData: []string{discoveryEndpoints},
		}, {
			Name:      "diff",
			Val:       "diff",
			ExtraData: []string{allowedEndpoints, extraConfig, endpoints},
		}},
	})
}

func NetworkManipulateMitmproxy(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	proxy, err := newMitmproxy(s)
	if err := cr.LaunchAndApplyProxy(ctx, proxy); err != nil {
		s.Fatal("Failed to launch and apply proxy: ", err)
	}
	defer cr.CleanupProxy(cleanupCtx)

	conn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browser.TypeAsh, "https://www.example.com")
	if err != nil {
		s.Fatal("Failed to open test page: ", err)
	}
	br.ReloadActiveTab(ctx)

	if err := verify(ctx, s, conn); err != nil {
		s.Fatal("Failed to verify page: ", err)
	}

	defer closeBrowser(cleanupCtx)
	defer conn.Close()
}

func verify(ctx context.Context, s *testing.State, conn *chrome.Conn) error {
	switch s.Param().(string) {
	case "redirect":
		return verifyPageContent(ctx, conn, "google")
	case "error":
		return verifyPageContent(ctx, conn, "error injected by proxy")
	default:
		return nil
	}
}

func verifyPageContent(ctx context.Context, conn *chrome.Conn, expected string) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		content, err := conn.PageContent(ctx)
		if err != nil {
			return errors.Wrap(err, "fail to get page content")
		}
		if !strings.Contains(content, expected) {
			return errors.Wrapf(err, "page doesn't contain expected content: %s, page is %s", expected, content)
		}

		return nil
	}, &testing.PollOptions{Timeout: 3 * time.Second, Interval: 1 * time.Second}); err != nil {
		return err
	}

	return nil
}

func newMitmproxy(s *testing.State) (*mitmproxy.MitmProxy, error) {
	testCase := s.Param().(string)
	var opts []mitmproxy.Option
	switch testCase {
	case "redirect":
		opts = append(opts,
			mitmproxy.ScriptPath(s.DataPath(httpRedirect)),
			mitmproxy.OutDir(s.OutDir()),
		)
	case "error":
		opts = append(opts,
			mitmproxy.ScriptPath(s.DataPath(httpErrorInject)),
			mitmproxy.OutDir(s.OutDir()),
		)
	case "diff":
		opts = append(opts,
			mitmproxy.ScriptPath(s.DataPath(allowedEndpoints), s.DataPath(extraConfig)),
			mitmproxy.CustomOptions(fmt.Sprintf("allowed_endpoints_yaml=%s", s.DataPath(endpoints))),
			mitmproxy.OutDir(s.OutDir()),
		)
	case "discovery":
		opts = append(opts,
			mitmproxy.ScriptPath(s.DataPath(discoveryEndpoints)),
			mitmproxy.CustomOptions(fmt.Sprintf("endpoint_info_folder=%s", s.OutDir()), fmt.Sprintf("patterns_to_record=%s", "example.com")),
			mitmproxy.OutDir(s.OutDir()),
		)
	}
	return mitmproxy.New(opts...)
}
