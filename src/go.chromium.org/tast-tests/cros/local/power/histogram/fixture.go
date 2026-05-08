// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package histogram contains fixtures related to power histograms.
package histogram

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	chromeQuickMetricsCollectionArg := "--external-metrics-collection-interval=1"

	// Sets version to "unknown" to ensure smart dim uses builtin model.
	chromeSmartDimBuiltinModelArg := "--enable-features=SmartDimExperimentalComponent:smart_dim_experimental_version/unknown"

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeFastHistograms",
		Desc:     "Logged into a user session and enabled quick metrics collection for fast histogram validation",
		Contacts: []string{"chrome-knowledge-eng@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeQuickMetricsCollectionArg),
			}, nil
		}),
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:     "chromeFastHistogramsAndBuiltinSmartDimModel",
		Desc:     "Similar to chromeFastHistograms, plus force chrome to use builtin smart dim models",
		Contacts: []string{"chrome-knowledge-eng@google.com"},
		Impl: chrome.NewLoggedInFixture(func(ctx context.Context, s *testing.FixtState) ([]chrome.Option, error) {
			return []chrome.Option{
				chrome.ExtraArgs(chromeQuickMetricsCollectionArg),
				chrome.ExtraArgs(chromeSmartDimBuiltinModelArg),
			}, nil
		}),
		SetUpTimeout:    chrome.LoginTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}
