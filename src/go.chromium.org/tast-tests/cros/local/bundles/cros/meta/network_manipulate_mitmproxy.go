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
	httpRedirect     = "mitmproxy_redirect_requests.py"
	httpErrorInject  = "mitmproxy_inject_500_requests.py"
	allowedEndpoints = "allowed_endpoints.py"
	extraConfig      = "allowed_endpoints_yaml.py"
	endpoints        = "endpoints.yml"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NetworkManipulateMitmproxy,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that manipulating network traffic with mitmproxy",
		Contacts: []string{
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

	redirectCase := strings.HasSuffix(s.TestName(), "redirect")
	errorInjectCase := strings.HasSuffix(s.TestName(), "error")
	diffCase := strings.HasSuffix(s.TestName(), "diff")

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	proxy := mitmproxy.New()

	proxy.SetOutDir(s.OutDir())

	if redirectCase {
		proxy.AddScriptPath(s.DataPath(httpRedirect))
	} else if errorInjectCase {
		proxy.AddScriptPath(s.DataPath(httpErrorInject))
	} else if diffCase {
		proxy.AddScriptPath(s.DataPath(allowedEndpoints))
		proxy.AddScriptPath(s.DataPath(extraConfig))
		proxy.AddOtherOptions(fmt.Sprintf("allowed_endpoints_yaml=%s", s.DataPath(endpoints)))
	}

	if err := cr.LaunchAndApplyProxy(ctx, proxy); err != nil {
		s.Fatal("Failed to launch and apply proxy: ", err)
	}
	defer cr.CleanupProxy(cleanupCtx)

	conn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browser.TypeAsh, "https://www.example.com")
	if err != nil {
		s.Fatal("Failed to open test page: ", err)
	}
	br.ReloadActiveTab(ctx)

	if redirectCase {
		if err := verifyPageContent(ctx, conn, "google"); err != nil {
			s.Fatal("Failed to redirect page to google: ", err)
		}
	} else if errorInjectCase {
		if err := verifyPageContent(ctx, conn, "error injected by proxy"); err != nil {
			s.Fatal("Failed to inject error: ", err)
		}
	}

	defer closeBrowser(cleanupCtx)
	defer conn.Close()
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
