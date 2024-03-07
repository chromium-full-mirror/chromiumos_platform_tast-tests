// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package camera contains camera-related utility functions for local tests.
package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	serviceTimeout = 30 * time.Second
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.CameraServiceReady,
		Desc:            "The cros-camera service is ready",
		Contacts:        []string{"chromeos-camera-eng@google.com", "hidenorik@chromium.org"},
		Impl:            &serviceFixture{request: startService},
		Parent:          fixture.CameraEnumerated,
		SetUpTimeout:    serviceTimeout,
		ResetTimeout:    serviceTimeout,
		TearDownTimeout: serviceTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.CameraConnectorReady,
		Desc:            "The camera connector is ready",
		Contacts:        []string{"chromeos-camera-eng@google.com", "hidenorik@chromium.org"},
		Impl:            &connectorFixture{},
		Parent:          fixture.CameraServiceReady,
		SetUpTimeout:    chrome.LoginTimeout,
		TearDownTimeout: chrome.ResetTimeout,
	})
}

type serviceRequest uint

// We use enum here, because we plan to support stopping service.
const (
	// startService starts cros-camera service in Setup() and Reset()
	startService serviceRequest = iota
)

// The ServiceRequest only affect the operation in Setup() and Reset().
// TearDown() always brings the service to running, which is the default state.
type serviceFixture struct {
	request serviceRequest
}

func (f *serviceFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if err := ensureServiceState(ctx, f.request); err != nil {
		s.Fatal("Failed to setup camera service: ", err)
	}

	return nil
}

func (f *serviceFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
		s.Log("Failed to start camera service: ", err)
	}
}

func (f *serviceFixture) Reset(ctx context.Context) error {
	if err := ensureServiceState(ctx, f.request); err != nil {
		return errors.Wrap(err, "failed to reset camera service")
	}

	return nil
}

func (f *serviceFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *serviceFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

// ensureServiceState makes sure that the cros-camera service is in a state
// that is requested by serviceRequest.
func ensureServiceState(ctx context.Context, request serviceRequest) error {
	if request == startService {
		// WaitForCameraSocket includes a call to EnsureJobRunning.
		if err := testutil.WaitForCameraSocket(ctx); err != nil {
			return err
		}
	}

	return nil
}

type connectorFixture struct {
	cr *chrome.Chrome
}

// SetUp only ensures that we don't have a stale user of the connector.
// The readiness of the connector itself should be ensured in the parent fixture
// by calling testutil.WaitForCameraSocket().
func (f *connectorFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cr, err := chrome.New(ctx, chrome.NoLogin())
	if err != nil {
		s.Fatal("Failed to start chrome: ", err)
	}
	f.cr = cr
	return nil
}
func (f *connectorFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.cr.Close(ctx); err != nil {
		s.Error("Failed to close chrome: ", err)
	}
}
func (f *connectorFixture) Reset(ctx context.Context) error {
	return nil
}
func (f *connectorFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *connectorFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
