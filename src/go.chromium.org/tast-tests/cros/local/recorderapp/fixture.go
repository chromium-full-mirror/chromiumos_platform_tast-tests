// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package recorderapp

import (
	"context"
	"fmt"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "recorderAppPrepared",
		Desc:            "Set necessary settings for launching the Recorder App",
		Contacts:        []string{"chromeos-recorder-app@google.com", "kamchonlathorn@chromium.org"},
		BugComponent:    "b:1522466", // ChromeOS > Platform > Technologies > Audio > Recorder App
		Impl:            &fixture{},
		SetUpTimeout:    chrome.FixtureSetUpTimeout,
		ResetTimeout:    chrome.ResetTimeout,
		TearDownTimeout: chrome.ResetTimeout,
		Vars:            []string{"recorderapp.conchKey"},
	})
}

type fixture struct {
	cr *chrome.Chrome
}

// FixtureData is the struct exposed to tests.
type FixtureData struct {
	Chrome *chrome.Chrome
}

func (f *fixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	chromeOpts := []chrome.Option{
		chrome.EnableFeatures("Conch"),
		chrome.ExtraArgs(fmt.Sprintf("--conch-key=%s", s.RequiredVar("recorderapp.conchKey"))),
	}
	cr, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	f.cr = cr
	return FixtureData{
		Chrome: cr,
	}
}

func (f *fixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cr.Close(ctx); err != nil {
		s.Error("Failed to tear down Chrome: ", err)
	}
	f.cr = nil
}

func (f *fixture) Reset(ctx context.Context) error {
	return nil
}

func (f *fixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *fixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
