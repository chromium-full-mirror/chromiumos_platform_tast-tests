// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture provides ti50 devboard related fixtures.
package fixture

import (
	"context"
	"time"

	"google.golang.org/grpc"

	"chromiumos/tast/common/firmware/ti50"
	remoteTi50 "chromiumos/tast/remote/firmware/ti50"
	"chromiumos/tast/testing"
)

const (
	// DevBoardService arg name for the service's host:port pair and also the name of the fixture.
	DevBoardService = "devboardsvc"

	// Ti50 fixture flashes a ti50 image.
	Ti50 = "ti50"

	// SystemTestAuto fixture flashes a system_test_auto image.
	SystemTestAuto = "systemTestAuto"

	setUpTimeout    = 2 * time.Minute
	resetTimeout    = 5 * time.Second
	tearDownTimeout = 5 * time.Second
	preTestTimeout  = 5 * time.Second
	postTestTimeout = 5 * time.Second
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            DevBoardService,
		Desc:            "A DevBoard connected to a devboardsvc service host",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "jbettis@google.com"},
		Impl:            &impl{},
		Vars:            []string{DevBoardService},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            Ti50,
		Desc:            "Uses devboardsvc to flash a Ti50 image",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "jbettis@google.com"},
		Impl:            &impl{},
		Vars:            []string{DevBoardService},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		Parent:          Ti50Image,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            SystemTestAuto,
		Desc:            "Uses devboardsvc to flash a system_test_auto image",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "jbettis@google.com"},
		Impl:            &impl{},
		Vars:            []string{DevBoardService},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		Parent:          SystemTestAutoImage,
	})
}

// Value allows tests to obtain a ti50 devboard.
type Value struct {
	grpcConn  *grpc.ClientConn
	devboard  *remoteTi50.DUTControlAndreiboard
	ImagePath string
}

// DevBoard returns the existing DevBoard connection instance.
func (v *Value) DevBoard() ti50.DevBoard {
	return v.devboard
}

type impl struct {
	imageValue *ImageValue
	hostPort   string
	v          *Value
}

func (i *impl) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	if s.ParentValue() != nil {
		i.imageValue = s.ParentValue().(*ImageValue)
	}

	i.hostPort = s.RequiredVar(DevBoardService)
	i.v = &Value{}

	if err := i.dialGrpc(ctx); err != nil {
		s.Fatal("dial grpc: ", err)
	}
	board := remoteTi50.NewDUTControlAndreiboard(i.v.grpcConn, 0, 0*time.Second)
	defer board.Close(ctx)

	imagePath := ""
	var fwConfigJsons []string
	if i.imageValue != nil {
		imagePath = i.imageValue.ImagePath()
		fwConfigJsons = i.imageValue.FwConfigPaths()
	}

	testing.ContextLog(ctx, "Setting up image: ", imagePath)
	if err := board.Setup(ctx, imagePath, fwConfigJsons); err != nil {
		s.Fatal("Setup: ", err)
	}

	i.v.ImagePath = i.imageValue.ImagePath()

	return i.v
}

func (i *impl) Reset(ctx context.Context) error {
	if i.v.devboard != nil {
		if err := i.v.devboard.Reset(ctx); err != nil {
			return err
		}
		if err := i.v.devboard.Close(ctx); err != nil {
			return err
		}
		i.v.devboard = nil
	}
	return nil
}

func (i *impl) PreTest(ctx context.Context, s *testing.FixtTestState) {
	testing.ContextLog(ctx, "Starting OTT session")
	i.v.devboard = remoteTi50.NewDUTControlAndreiboard(i.v.grpcConn, 10000, time.Second)
	// At this point, the plan is to start an opentitantool session, which could invove either
	// starting a host emulation instance, or resetting a devboard and its debugger to a known
	// state.
	if err := i.v.devboard.StartSession(ctx); err != nil {
		if err2 := i.v.devboard.Close(ctx); err2 != nil {
			s.Error("Failed to close devboard: ", err2)
		}
		i.v.devboard = nil
		s.Fatal("Failed to start session: ", err)
	}
}

func (i *impl) PostTest(ctx context.Context, s *testing.FixtTestState) {
	testing.ContextLog(ctx, "Ending OTT session")
	// At this point, we should end the opentitantool session, that is, stop host emulator, or
	// disconnect from devboard.
	if err := i.v.devboard.EndSession(ctx); err != nil {
		s.Error("Failed to end session: ", err)
	}
	if err := i.v.devboard.Close(ctx); err != nil {
		s.Fatal("Failed to close devboard: ", err)
	}
	i.v.devboard = nil
}

func (i *impl) TearDown(ctx context.Context, s *testing.FixtState) {
	if i.v.grpcConn != nil {
		if err := i.v.grpcConn.Close(); err != nil {
			s.Error("Failed to close grpc: ", err)
		}
		i.v.grpcConn = nil
	}
}

func (i *impl) String() string {
	if i.imageValue != nil {
		return DevBoardService + "_" + i.imageValue.ImageType()
	}
	return DevBoardService
}

// dialGrpc connects to the devboardsvc host.
func (i *impl) dialGrpc(ctx context.Context) error {
	conn, err := grpc.DialContext(ctx, i.hostPort, grpc.WithInsecure())
	if err != nil {
		return err
	}
	i.v.grpcConn = conn
	return nil
}
