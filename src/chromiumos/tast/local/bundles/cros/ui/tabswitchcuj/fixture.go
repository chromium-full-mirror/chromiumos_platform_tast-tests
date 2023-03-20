// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tabswitchcuj

import (
	"context"
	"strings"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/local/wpr"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "tabSwitchCUJWPR",
		Desc: "Base fixture for TabSwitchCUJ with WPR",
		Contacts: []string{
			"xiyuan@chromium.org",
			"chromeos-perfmetrics-eng@google.com",
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
			"chromeos-perfmetrics-eng@google.com",
		},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return s.ParentValue().(wpr.FixtValue).FOpt()(ctx, s)
		}),
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Parent:          "tabSwitchCUJWPR",
	})

	testing.AddFixture(&testing.Fixture{
		Name: "tabSwitchCUJWPRLacros",
		Desc: "Composed fixture for TabSwitchCUJ with WPR",
		Contacts: []string{
			"xiyuan@chromium.org",
			"chromeos-perfmetrics-eng@google.com",
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
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Parent:          "tabSwitchCUJWPR",
	})

	testing.AddFixture(&testing.Fixture{
		Name: "tabSwitchCUJWPRAshWithBackupRefPtr",
		Desc: "Variant of tabSwitchCUJWPRAsh with BackupRefPtr enabled",
		Contacts: []string{
			"ramsaroop@chromium.org",
			"chromeos-perfmetrics-eng@google.com",
		},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			opts, err := s.ParentValue().(wpr.FixtValue).FOpt()(ctx, s)
			if err != nil {
				return nil, err
			}
			opts = append(opts, chrome.EnableFeatures("PartitionAllocBackupRefPtr:enabled-processes/browser-only"))
			return opts, nil
		}),
		SetUpTimeout:    chrome.LoginTimeout + 7*time.Minute,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Parent:          "tabSwitchCUJWPR",
	})

}
