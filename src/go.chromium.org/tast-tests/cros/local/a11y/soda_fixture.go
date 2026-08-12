// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package a11y

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// SodaDLCInstalled is the fixture name for SODA DLC cleanup.
	SodaDLCInstalled = "sodaDLCInstalled"
	// PowerAshWithSoda is the fixture name for PowerAsh with SODA DLC cleanup.
	PowerAshWithSoda = "sodaDLCInstalled.power_ash"
	// LoggedInARCWithInternalCameraAndEffectsDisabledWithSoda is the fixture name for LoggedInARCWithInternalCameraAndEffectsDisabled with SODA DLC cleanup.
	LoggedInARCWithInternalCameraAndEffectsDisabledWithSoda = "sodaDLCInstalled.arc"
	// LoggedInWithFakeHALAndEffectsDisabledWithSoda is the fixture name for LoggedInWithFakeHALAndEffectsDisabled with SODA DLC cleanup.
	LoggedInWithFakeHALAndEffectsDisabledWithSoda = "sodaDLCInstalled.fake_hal"
	// PowerAshCaptionsOnBrailleWithSoda is the fixture name for PowerAshCaptionsOnBraille with SODA DLC cleanup.
	PowerAshCaptionsOnBrailleWithSoda = "sodaDLCInstalled.braille"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: SodaDLCInstalled,
		Desc: "Purges libsoda and libsoda-model-en-us DLCs after test",
		Contacts: []string{
			"ml-service-team@google.com",
			"amoylan@chromium.org",
		},
		BugComponent:    "b:1116342", // Software > Machine Intelligence > libsoda & ChromeOS Live Caption
		Impl:            &sodaDLCFixture{},
		SetUpTimeout:    5 * time.Minute,
		TearDownTimeout: 5 * time.Minute,
		Params: []testing.FixtureParam{
			{}, // Default, no parent
			{
				Name:   "power_ash",
				Parent: "powerAsh",
			},
			{
				Name:   "arc",
				Parent: "loggedInARCWithInternalCameraAndEffectsDisabled",
			},
			{
				Name:   "fake_hal",
				Parent: "loggedInWithFakeHALAndEffectsDisabled",
			},
			{
				Name:   "braille",
				Parent: "powerAshCaptionsOnBraille",
			},
		},
	})
}

type sodaDLCFixture struct {
}

func (f *sodaDLCFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// We don't download SODA DLCs here because libsoda-model-en-us has a variable suffix.
	// The test itself is expected to trigger implicit download.
	// We purge them here to ensure a clean state before the test starts.
	if err := purgeSodaDLCs(ctx); err != nil {
		s.Fatal("Failed to purge SODA DLCs in SetUp: ", err)
	}
	return s.ParentValue()
}

func purgeSodaDLCs(ctx context.Context) error {
	dlcMap, err := dlc.List(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list DLCs")
	}
	for id := range dlcMap {
		if id == "libsoda" || strings.HasPrefix(id, "libsoda-model-en-us") {
			if err := dlc.Purge(ctx, id); err != nil {
				return errors.Wrapf(err, "failed to purge DLC %s", id)
			}
		}
	}
	return nil
}

func (f *sodaDLCFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := purgeSodaDLCs(ctx); err != nil {
		s.Error("Failed to purge SODA DLCs: ", err)
	}
}

func (f *sodaDLCFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *sodaDLCFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *sodaDLCFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
