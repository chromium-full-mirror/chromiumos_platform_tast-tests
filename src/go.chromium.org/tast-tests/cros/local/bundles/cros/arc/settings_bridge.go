// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/arc"
	arca11y "go.chromium.org/tast-tests/cros/local/bundles/cros/arc/a11y"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/chromeproxy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SettingsBridge,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that Chrome settings are persisted in ARC",
		Contacts:     []string{"arc-framework+tast@google.com", "yhanada@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Chrome Integration
		BugComponent: "b:537221",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBootedWithoutUIAutomator",
		Timeout:      4 * time.Minute,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name:              "container_r",
			ExtraAttr:         []string{"informational", "group:criticalstaging"},
			ExtraSoftwareDeps: []string{"android_container_r"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

// checkAndroidAccessibility checks that Android accessibility Settings is expectedly enabled/disabled.
func checkAndroidAccessibility(ctx context.Context, a *arc.ARC, enable bool) error {
	if res, err := arca11y.IsEnabledAndroid(ctx, a); err != nil {
		return err
	} else if res != enable {
		return errors.Errorf("accessibility_enabled is %t in Android", res)
	}

	services, err := arca11y.EnabledAndroidAccessibilityServices(ctx, a)
	if err != nil {
		return err
	}
	enabled := len(services) == 1 && services[0] == arca11y.ArcAccessibilityHelperService
	disabled := len(services) == 1 && len(services[0]) == 0
	if (enable && !enabled) || (!enable && !disabled) {
		return errors.Errorf("enabled accessibility services are not expected: %v", services)
	}

	return nil
}

// testAccessibilitySync runs the test to ensure spoken feedback settings
// are synchronized between Chrome and Android.
func testAccessibilitySync(ctx context.Context, s *testing.State, tconn *chrome.TestConn, a *arc.ARC) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	if enabled, err := arca11y.IsEnabledAndroid(ctx, a); err != nil {
		s.Error("Failed to check if android accessibility is enabled: ", err)
		return
	} else if enabled {
		s.Error("accessibility is unexpectedly enabled on boot")
		return
	}

	// Ensure that disable switch access confirmation dialog does not get shown.
	if err := tconn.Eval(ctx, `chrome.autotestPrivate.disableSwitchAccessDialog();`, nil); err != nil {
		s.Error(err, "Failed to disable Switch Access dialog")
		return
	}

	features := []a11y.Feature{
		a11y.SpokenFeedback,
		a11y.SwitchAccess,
		a11y.SelectToSpeak,
		a11y.FocusHighlight,
		a11y.ScreenMagnifier,
		a11y.DockedMagnifier,
	}

	for _, feature := range features {
		s.Run(ctx, string(feature), func(ctx context.Context, s *testing.State) {
			arca11y.AttachSystemFaillog(ctx, s, a, "dumpsys-"+string(feature))

			for _, enable := range []bool{true, false} {
				if err := a11y.SetFeatureEnabled(ctx, tconn, feature, enable); err != nil {
					s.Fatalf("Failed to toggle %s to %t: %v", feature, enable, err)
				}

				if err := testing.Poll(ctx, func(ctx context.Context) error {
					if err := checkAndroidAccessibility(ctx, a, enable); err != nil {
						return err
					}
					return nil
				}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
					s.Fatalf("Failed to synchronize accessibility status of %s to be %t: %v", feature, enable, err)
				}
			}
		})

		if err := a11y.ClearFeature(ctx, tconn, feature); err != nil {
			s.Errorf("Failed clear settings of %s: %v", feature, err)
		}
	}
}

// proxySettingsTestCase contains fields necessary to test proxy settings.
type proxySettingsTestCase struct {
	name       string                  // subtestcase name
	config     chromeproxy.ProxyConfig // config value to be set
	host       string                  // expected host name
	port       string                  // expected port
	bypassList string                  // expected bypassList
	pacURL     string                  // expected proxy auto-config file URL
}

// getAndroidProxy obtains specified proxy value from Android.
// proxy is one of:
// global_http_proxy_host|global_http_proxy_port|global_proxy_pac_url|global_http_proxy_exclusion_list.
func getAndroidProxy(ctx context.Context, a *arc.ARC, proxyString string) (string, error) {
	res, err := a.Command(ctx, "settings", "get", "global", proxyString).Output(testexec.DumpLogOnError)
	if err != nil {
		return "", err
	}
	proxy := strings.TrimSpace(string(res))
	if proxy == "null" {
		return "", nil
	}
	return proxy, nil
}

// testProxySync runs the test to ensure that proxy settings are
// synchronized between Chrome and Android.
func testProxySync(ctx context.Context, s *testing.State, tconn *chrome.TestConn, a *arc.ARC) {
	for _, tc := range []proxySettingsTestCase{{
		name: "Direct",
		config: chromeproxy.ProxyConfig{
			Mode: chromeproxy.ModeDirect,
		},
	}, {
		name: "FixedServers",
		config: chromeproxy.ProxyConfig{
			Mode: chromeproxy.ModeFixedServers,
			Rules: &chromeproxy.ProxyRules{
				SingleProxy: &chromeproxy.ProxyServer{
					Host: "proxy",
					Port: 8080,
				},
				BypassList: []string{"foobar.com", "*.de"},
			},
		},
		host:       "proxy",
		port:       "8080",
		bypassList: "foobar.com,*.de",
	}, {
		name: "AutoDetect",
		config: chromeproxy.ProxyConfig{
			Mode: chromeproxy.ModeAutoDetect,
		},
		host:   "localhost",
		port:   "-1",
		pacURL: "http://wpad/wpad.dat",
	}, {
		name: "PacScript",
		config: chromeproxy.ProxyConfig{
			Mode: chromeproxy.ModePacScript,
			PacScript: &chromeproxy.PacScript{
				URL: "http://example.com",
			},
		},
		host:   "localhost",
		port:   "-1",
		pacURL: "http://example.com",
	}} {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			if err := runProxyTest(ctx, tconn, a, tc); err != nil {
				s.Error("Failed to run proxy sync: ", err)
			}
		})
	}
}

// checkProxySettings checks that current Android proxy settings match with expected values.
func checkProxySettings(ctx context.Context, a *arc.ARC, p proxySettingsTestCase) error {
	currHost, err := getAndroidProxy(ctx, a, "global_http_proxy_host")
	if err != nil {
		return err
	}
	if currHost != p.host {
		return errors.Errorf("host does not match, got %q, want %q", currHost, p.host)
	}

	currPort, err := getAndroidProxy(ctx, a, "global_http_proxy_port")
	if err != nil {
		return err
	}
	if currPort != p.port {
		return errors.Errorf("port does not match, got %q, want %q", currPort, p.port)
	}

	currBypassList, err := getAndroidProxy(ctx, a, "global_http_proxy_exclusion_list")
	if err != nil {
		return err
	}
	if currBypassList != p.bypassList {
		return errors.Errorf("bypassList does not match, got %q, want %q", currBypassList, p.bypassList)
	}

	currPacURL, err := getAndroidProxy(ctx, a, "global_proxy_pac_url")
	if err != nil {
		return err
	}
	if currPacURL != p.pacURL {
		return errors.Errorf("pacURL does not match, got %q, want %q", currPacURL, p.pacURL)
	}

	return nil
}

// runProxyTest performs necessary tasks to ensure that proxy settings are
// synchronized between Chrome and Android.
// Proxy settings in Chrome are set, then the proxy settings in Android are checked to see if they match.
func runProxyTest(ctx context.Context, tconn *chrome.TestConn, a *arc.ARC, p proxySettingsTestCase) error {
	if err := chromeproxy.SetSettings(ctx, tconn, chromeproxy.ProxySettings{Value: p.config}); err != nil {
		return errors.Wrap(err, "setting chrome proxy failed")
	}

	return testing.Poll(ctx, func(ctx context.Context) error {
		return checkProxySettings(ctx, a, p)
	}, &testing.PollOptions{Timeout: 30 * time.Second})
}

func SettingsBridge(ctx context.Context, s *testing.State) {
	d := s.FixtValue().(*arc.PreData)
	a := d.ARC
	cr := d.Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	if err := a.WaitIntentHelper(ctx); err != nil {
		s.Fatal("Failed to wait for ArcIntentHelper: ", err)
	}

	testAccessibilitySync(ctx, s, tconn, a)

	testProxySync(ctx, s, tconn, a)
}
