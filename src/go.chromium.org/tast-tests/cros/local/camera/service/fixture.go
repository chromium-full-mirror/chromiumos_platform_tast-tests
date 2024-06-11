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
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	serviceTimeout = 30 * time.Second
)

func init() {
	// Fixtures that ensure all built-in cameras are enumerated; intended for user space testing.
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.CameraServiceReady,
		Desc:            "The cros-camera service is ready, with all built-in cameras enumerated",
		Contacts:        []string{"chromeos-camera-eng@google.com", "hidenorik@chromium.org"},
		BugComponent:    "b:167281", // ChromeOS > Platform > Technologies > Camera
		Impl:            &serviceFixture{request: startService},
		Parent:          fixture.CameraEnumerated,
		SetUpTimeout:    serviceTimeout,
		ResetTimeout:    serviceTimeout,
		TearDownTimeout: serviceTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.CameraServiceRestarted,
		Desc:            "The cros-camera service is restarted, with all built-in cameras enumerated",
		Contacts:        []string{"chromeos-camera-eng@google.com", "hidenorik@chromium.org"},
		BugComponent:    "b:167281", // ChromeOS > Platform > Technologies > Camera
		Impl:            &serviceFixture{request: restartService},
		Parent:          fixture.CameraEnumerated,
		SetUpTimeout:    serviceTimeout,
		ResetTimeout:    serviceTimeout,
		TearDownTimeout: serviceTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.CameraConnectorReady,
		Desc:            "The camera connector is ready, with all built-in cameras enumerated",
		Contacts:        []string{"chromeos-camera-eng@google.com", "hidenorik@chromium.org"},
		BugComponent:    "b:167281", // ChromeOS > Platform > Technologies > Camera
		Impl:            &serviceFixture{request: startServiceWithConnector},
		Parent:          fixture.CameraEnumerated,
		SetUpTimeout:    serviceTimeout + chrome.LoginTimeout,
		ResetTimeout:    serviceTimeout,
		TearDownTimeout: serviceTimeout + chrome.ResetTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.CameraConnectorRestarted,
		Desc:            "The camera connector is ready, with all built-in cameras enumerated & service restarted",
		Contacts:        []string{"chromeos-camera-eng@google.com", "hidenorik@chromium.org"},
		BugComponent:    "b:167281", // ChromeOS > Platform > Technologies > Camera
		Impl:            &serviceFixture{request: restartServiceWithConnector},
		Parent:          fixture.CameraEnumerated,
		SetUpTimeout:    serviceTimeout + chrome.LoginTimeout,
		ResetTimeout:    serviceTimeout,
		TearDownTimeout: serviceTimeout + chrome.ResetTimeout,
	})
	// Fixture that does not ensure all built-in cameras are enumerated; intended for kernel testing.
	testing.AddFixture(&testing.Fixture{
		Name:            fixture.CameraServiceStopped,
		Desc:            "The cros-camera service is stopped",
		Contacts:        []string{"chromeos-camera-eng@google.com", "hidenorik@chromium.org"},
		BugComponent:    "b:167281", // ChromeOS > Platform > Technologies > Camera
		Impl:            &serviceFixture{request: stopService},
		SetUpTimeout:    serviceTimeout,
		ResetTimeout:    serviceTimeout,
		TearDownTimeout: serviceTimeout,
	})
}

type serviceRequest uint

const (
	// startService ensures cros-camera service is started.
	startService serviceRequest = iota
	// stopService ensures cros-camera service is stopped.
	stopService
	// restartService ensures cros-camera service is restarted.
	// This should be used when clean-ups cannot be run reliably, e.g. in suspend test.
	restartService
	// startServiceWithConnector ensures cros-camera service is started, with new Ash.
	startServiceWithConnector
	// restartServiceWithConnector ensures cros-camera service is restarted, with new Ash.
	restartServiceWithConnector
)

// The ServiceRequest only affects the operation in SetUp() and Reset().
// TearDown() always brings the service to running, which is the default state.
type serviceFixture struct {
	request serviceRequest
	cr      *chrome.Chrome
}

func (f *serviceFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// First, reset any modifications (e.g. bridge to FakeHAL) made in preceding tests.
	if err := testutil.RemoveTestBridgeConfigs(ctx); err != nil {
		s.Fatal("Failed to clean-up camera TestBridge: ", err)
	}

	if err := restartCameraService(ctx); err != nil {
		s.Fatal("Failed to restart camera service: ", err)
	}

	// Then, put the stack in the requested state.
	if isConnectorRequested(f.request) {
		cr, err := chrome.New(ctx, chrome.NoLogin())
		if err != nil {
			s.Fatal("Failed to start chrome: ", err)
		}
		f.cr = cr
	}
	defer func() {
		if s.HasError() {
			f.cr.Close(cleanupCtx)
		}
	}()

	if err := ensureServiceState(ctx, f.request); err != nil {
		s.Fatal("Failed to setup camera service: ", err)
	}

	return nil
}

func (f *serviceFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.request == restartService {
		if err := restartCameraService(ctx); err != nil {
			s.Error("Failed to restart camera service: ", err)
		}
	}
	if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
		s.Error("Failed to start camera service: ", err)
	}
	if isConnectorRequested(f.request) {
		if err := f.cr.Close(ctx); err != nil {
			s.Error("Failed to close chrome: ", err)
		}
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
	switch request {
	case startService, startServiceWithConnector:
		// WaitForCameraServiceBinding includes a call to EnsureJobRunning.
		return testutil.WaitForCameraServiceBinding(ctx)
	case stopService:
		return upstart.StopJob(ctx, "cros-camera")
	case restartService, restartServiceWithConnector:
		return restartCameraService(ctx)
	}

	return errors.New("invalid request")
}

func isConnectorRequested(request serviceRequest) bool {
	return request == startServiceWithConnector || request == restartServiceWithConnector
}

func restartCameraService(ctx context.Context) error {
	if err := upstart.RestartJob(ctx, "cros-camera"); err != nil {
		return err
	}
	// WaitForCameraServiceBinding includes a call to EnsureJobRunning.
	return testutil.WaitForCameraServiceBinding(ctx)

}
