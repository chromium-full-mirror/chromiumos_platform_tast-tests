// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tabswitchcuj

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/wpr"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "tabSwitchCUJWPR",
		Desc: "Base fixture for TabSwitchCUJ with WPR",
		Contacts: []string{
			"xiyuan@chromium.org",
			"cros-sw-perf@google.com",
		},
		Impl:            wpr.NewFixture(WPRArchiveName, wpr.Replay),
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Data:            []string{WPRArchiveName},
	})

	testing.AddFixture(&testing.Fixture{
		Name: "tabSwitchCUJWPRAsh",
		Desc: "Composed fixture for TabSwitchCUJ with WPR",
		Contacts: []string{
			"xiyuan@chromium.org",
			"cros-sw-perf@google.com",
		},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return s.ParentValue().(wpr.FixtValue).FOpt()(ctx, s)
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Parent:          "tabSwitchCUJWPR",
	})

	testing.AddFixture(&testing.Fixture{
		Name: "tabSwitchCUJWPRLacros",
		Desc: "Composed fixture for TabSwitchCUJ with WPR",
		Contacts: []string{
			"xiyuan@chromium.org",
			"cros-sw-perf@google.com",
		},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			opts, err := s.ParentValue().(wpr.FixtValue).FOpt()(ctx, s)
			if err != nil {
				return nil, err
			}
			if strings.ToLower(cuj.EnableWaylandLoggingVar.Value()) == "true" {
				opts = append(opts, chrome.ExtraArgs("--lacros-chrome-additional-env=WAYLAND_DEBUG=1"))
			}
			return lacrosfixt.NewConfig(lacrosfixt.ChromeOptions(opts...)).Opts()
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Parent:          "tabSwitchCUJWPR",
	})

	testing.AddFixture(&testing.Fixture{
		Name: "tabSwitchCUJWPRAshWithFieldTrials",
		Desc: "Variant of tabSwitchCUJWPRAsh with all field trials enabled",
		Contacts: []string{
			"ramsaroop@google.com",
			"cros-sw-perf@google.com",
		},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			opts, err := s.ParentValue().(wpr.FixtValue).FOpt()(ctx, s)
			if err != nil {
				return nil, err
			}
			opts = append(opts, chrome.ExtraArgs("enable-field-trial-config"))
			return opts, nil
		}),
		SetUpTimeout:    chrome.FixtureSetUpTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Parent:          "tabSwitchCUJWPR",
	})

}
