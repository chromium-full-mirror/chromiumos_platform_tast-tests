// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dns

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "dnsProxyDisabled",
		Desc: "A fixture that ensures DNS proxy is disabled and will re-enable DNS proxy when the fixture is completed",
		Contacts: []string{
			"cros-networking@google.com",
			"jasongustaman@google.com",
		},
		SetUpTimeout:    chrome.LoginTimeout + 5*time.Second,
		PostTestTimeout: 5 * time.Second,
		TearDownTimeout: chrome.LoginTimeout + 5*time.Second,
		Impl:            &dnsProxyFixture{},
	})
}

// isDisabled checks if DNS proxy is disabled from the number of dnsproxyd processes running.
func isDisabled(ctx context.Context) (bool, error) {
	out, err := testexec.CommandContext(ctx, "ps", "-u", "dns-proxy", "--no-headers").Output()
	if err != nil {
		return false, errors.Wrap(err, "failed to get processes")
	}
	// If DNS proxy is disabled, only 1 process (controller) is running.
	return strings.Count(string(out), "\n") == 1, nil
}

// dnsProxyFixture implements testing.FixtureImpl.
type dnsProxyFixture struct{}

// FixtureData is the data returned by SetUp and passed to tests.
type FixtureData struct {
	Chrome *chrome.Chrome
}

func (f *dnsProxyFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cr, err := chrome.New(ctx, chrome.EnableFeatures("DisableDnsProxy"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	if err := RestartDNSProxy(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to restart DNS proxy: ", err)
	}
	if d, err := isDisabled(ctx); err != nil {
		s.Error("Failed to check if DNS proxy is disabled: ", err)
	} else if !d {
		s.Fatal("Failed to disable DNS proxy")
	}

	return FixtureData{Chrome: cr}
}

func (f *dnsProxyFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *dnsProxyFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *dnsProxyFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if d, err := isDisabled(ctx); err != nil {
		s.Error("Failed to check if DNS proxy is disabled: ", err)
	} else if !d {
		s.Fatal("DNS proxy is not disabled, is Chrome and DNS proxy restarted mid test?")
	}
}

func (f *dnsProxyFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if _, err := chrome.New(ctx, chrome.DisableFeatures("DisableDnsProxy"), chrome.ARCDisabled()); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	if err := RestartDNSProxy(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to restart DNS proxy: ", err)
	}
	if d, err := isDisabled(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to check if DNS proxy is disabled: ", err)
	} else if d {
		testing.ContextLog(ctx, "Failed to re-enable DNS proxy: ", err)
	}
}
