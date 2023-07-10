// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/services/cros/platform"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:         fixture.ServicesOnBoot,
		Desc:         "Fixture for platform.ServicesOnBoot.* tests; DO NOT USE for other tests",
		Contacts:     []string{"aaronyu@google.com", "chromeos-audio-sw@google.com"},
		Impl:         ServicesOnBootFixt{},
		SetUpTimeout: servicesOnBootSetUpTimeout,
		ServiceDeps:  []string{"tast.cros.platform.UpstartService"},
	})
}

const (
	servicesOnBootRebootTimeout      time.Duration = 5 * time.Minute
	servicesOnBootWaitJobTimeout                   = 3 * time.Minute
	servicesOnBootCollectionDuration               = 30 * time.Second
	servicesOnBootCleanupTimeout                   = 1 * time.Minute
	servicesOnBootSetUpTimeout                     = servicesOnBootRebootTimeout + servicesOnBootWaitJobTimeout + servicesOnBootCollectionDuration + servicesOnBootCleanupTimeout
)

// ServicesOnBootFixt is a fixture for the platform.ServiceOnBoot.* tests.
// DO NOT USE it on other tests.
type ServicesOnBootFixt struct{}

var _ testing.FixtureImpl = ServicesOnBootFixt{}

func (ServicesOnBootFixt) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Perform a reboot so that events happen before the test starts do not
	// have an impact on the test result. For example other tests failing a
	// service should not fail the platform.ServiceOnBoot.* tests.
	s.Log("Rebooting DUT")
	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	// Leave time for clean up.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, servicesOnBootCleanupTimeout)
	defer cancel()

	if err := s.DUT().Connect(ctx); err != nil {
		s.Fatal("Failed to connect to DUT: ", err)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	defer cl.Close(cleanupCtx)

	s.Log("Waiting for system-services")
	upstartService := platform.NewUpstartServiceClient(cl.Conn)
	if _, err := upstartService.WaitForJobStatus(ctx, &platform.WaitForJobStatusRequest{
		JobName: "system-services",
		Goal:    string(upstart.StartGoal),
		State:   string(upstart.RunningState),
		Timeout: durationpb.New(servicesOnBootWaitJobTimeout),
	}); err != nil {
		s.Fatal("Failed to wait for system-services: ", err)
	}

	s.Logf("Sleeping for %s to collect service activity", servicesOnBootCollectionDuration)
	// GoBigSleepLint: Sleep to observe service failures.
	// Generally, we recommend polling with a timeout instead of sleeping,
	// due to sleeping makes the test either slow or flaky.
	// However here, we're waiting for a service to fail. We don't have a
	// good way to tell if services have finished their initialization
	// sequences and can no longer fail spontaneously.
	// For our case, in the happy path we should reach the timeout.
	// Polling with a timeout instead of sleeping makes only the sad path faster if we
	// detect failures early, but optimizing the sad path is not useful.
	testing.Sleep(ctx, servicesOnBootCollectionDuration)

	return nil
}

func (ServicesOnBootFixt) Reset(ctx context.Context) error {
	return nil
}

func (ServicesOnBootFixt) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (ServicesOnBootFixt) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func (ServicesOnBootFixt) TearDown(ctx context.Context, s *testing.FixtState) {}
