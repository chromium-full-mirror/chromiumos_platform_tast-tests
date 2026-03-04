// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:         fixture.TuwunelServerSetup,
		Desc:         "Set up a local Tuwunel server for testing",
		BugComponent: "b:1361410", // ChromeOS > Platform > System > Core Power
		Contacts: []string{
			"chromeos-power-team@google.com",
			"cienet-development@googlegroups.com",
			"jason.hsiao@cienet.com",
		},
		Impl:            &tuwunelServerSetupImpl{},
		SetUpTimeout:    time.Minute,
		TearDownTimeout: time.Minute,
		ResetTimeout:    time.Minute,
	})
}

type tuwunelServerSetupImpl struct {
	server    *power.TuwunelServer
	serverCtx context.Context
	forwarder *ssh.Forwarder
}

func (f *tuwunelServerSetupImpl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	server := power.NewTuwunelServer()

	if err := server.Initiate(ctx); err != nil {
		s.Fatal("Failed to initiate Tuwunel server: ", err)
	}
	f.server = server
	defer func() {
		if s.HasError() {
			f.server.CleanUp()
		}
	}()

	// Use s.FixtContext() so the server can live until the fixture is torn down.
	f.serverCtx = s.FixtContext()
	if err := f.server.Start(f.serverCtx); err != nil {
		s.Fatal("Failed to start Tuwunel server: ", err)
	}
	defer func() {
		if s.HasError() {
			f.server.Stop()
		}
	}()

	DUTAddr := fmt.Sprintf("127.0.0.1:%d", power.TuwunelServerDefaultPort)
	hostAddr := fmt.Sprintf("127.0.0.1:%d", f.server.Port())
	forwarder, err := s.DUT().Conn().ForwardRemoteToLocal("tcp", DUTAddr, hostAddr, nil)
	if err != nil {
		s.Fatal("Failed to forward port from DUT to host: ", err)
	}
	f.forwarder = forwarder
	return nil
}

func (f *tuwunelServerSetupImpl) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.forwarder.Close(); err != nil {
		s.Log("Failed to close the forwarder: ", err)
	}
	if err := f.server.Stop(); err != nil {
		s.Log("Failed to stop Tuwunel server: ", err)
	}
	if err := f.server.CleanUp(); err != nil {
		s.Log("Failed to clean up Tuwunel server: ", err)
	}
}

func (f *tuwunelServerSetupImpl) Reset(ctx context.Context) error {
	if err := f.server.Reset(f.serverCtx); err != nil {
		return errors.Wrap(err, "failed to reset Tuwunel server")
	}
	return nil
}

func (f *tuwunelServerSetupImpl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// No-op.
}

func (f *tuwunelServerSetupImpl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// No-op.
}
