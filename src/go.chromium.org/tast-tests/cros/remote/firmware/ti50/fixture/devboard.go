// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture provides ti50 devboard related fixtures.
package fixture

import (
	"context"
	"os"
	"time"

	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	remoteTi50 "go.chromium.org/tast-tests/cros/remote/firmware/ti50"
	"go.chromium.org/tast/core/testing"
)

const (
	// DevBoardService arg name for the service's host:port pair and also the name of the fixture.
	DevBoardService = "devboardsvc"

	// Ti50Devboard fixture flashes a ti50 image and sets up a devboard connection
	Ti50Devboard = "ti50Devboard"

	// SystemTestAutoDevboard fixture flashes a system_test_auto image and sets up a devboard
	// connection
	SystemTestAutoDevboard = "systemTestAutoDevboard"

	// SystemTestAuto2Devboard fixture flashes a system_test_auto_2 image and sets up a devboard
	// connection
	SystemTestAuto2Devboard = "systemTestAuto2Devboard"

	setUpTimeout    = 2 * time.Minute
	resetTimeout    = 5 * time.Second
	tearDownTimeout = 5 * time.Second
	preTestTimeout  = 15 * time.Second
	postTestTimeout = 5 * time.Second
)

type extraPreTestMethod func(ctx context.Context, board ti50.DevBoard) error

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            Ti50Devboard,
		Desc:            "Uses devboardsvc to flash a Ti50 image",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "ecgh@google.com"},
		Impl:            &devboardFixture{image: Ti50Image},
		Vars:            []string{DevBoardService, BuildURL, FwConfigJSON, Chip, Variant, Slot},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            SystemTestAutoDevboard,
		Desc:            "Uses devboardsvc to flash a system_test_auto image",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "ecgh@google.com"},
		Impl:            &devboardFixture{image: SystemTestAutoImage},
		Vars:            []string{DevBoardService, BuildURL, FwConfigJSON, Chip, Variant, Slot},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})
	testing.AddFixture(&testing.Fixture{
		Name:            SystemTestAuto2Devboard,
		Desc:            "Uses devboardsvc to flash a system_test_auto_2 image",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "ecgh@google.com"},
		Impl:            &devboardFixture{image: SystemTestAuto2Image},
		Vars:            []string{DevBoardService, BuildURL, FwConfigJSON, Chip, Variant, Slot},
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
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

type devboardFixture struct {
	image      ImageType
	imageValue *ImageValue
	hostPort   string
	v          *Value
	preTest    extraPreTestMethod
}

func (i *devboardFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	i.hostPort = s.RequiredVar(DevBoardService)
	i.v = &Value{}

	if err := i.dialGrpc(ctx); err != nil {
		s.Fatal("dial grpc: ", err)
	}
	board := remoteTi50.NewDUTControlAndreiboard(i.v.grpcConn, 0, 0*time.Second)
	defer board.Close(ctx)

	testbedProperties, err := board.Query(ctx)
	if err != nil {
		s.Fatal("querying testbed: ", err)
	}

	iv, err := downloadImage(ctx, testbedProperties, i.image, s)
	if err != nil {
		s.Fatal("download image: ", err)
	}
	i.imageValue = iv

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

func (i *devboardFixture) Reset(ctx context.Context) error {
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

// resetTpmOpenCcd assumes that testlab has already been enabled for this device previously.
// If not, then this code will silently fail and not do anything
func resetTpmOpenCcd(ctx context.Context, board ti50.DevBoard) error {
	testing.ContextLog(ctx, "Erasing TPM data and opening CCD")
	image := ti50.NewCrOSImage(board)
	// If we don't close the UART connections here, then tests don't get uart data correctly
	defer board.Close(ctx)
	image.WaitUntilBooted(ctx)
	image.Command(ctx, "ccd testlab open")
	image.Command(ctx, "ccd reset factory")
	image.Command(ctx, "ccd set OpenNoTPMWipe ifopened")
	image.Command(ctx, "ccd lock")
	image.Command(ctx, "ccd open")
	image.Command(ctx, "ccd reset factory")
	return nil
}

func (i *devboardFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
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

func (i *devboardFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	// Ensure any pending console output is written to log file.
	i.v.devboard.ClearInput(ctx)
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

func (i *devboardFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if i.imageValue.downloaded && i.imageValue.imagePath != "" {
		if err := os.Remove(i.imageValue.imagePath); err != nil {
			s.Errorf("Failed to remove downloaded image %q: %v", i.imageValue.imagePath, err)
		}
		for _, confPath := range i.imageValue.configPaths {
			if err := os.Remove(confPath); err != nil {
				s.Errorf("Failed to remove downloaded conf %q: %v", confPath, err)
			}
		}
	}
	if i.v.grpcConn != nil {
		if err := i.v.grpcConn.Close(); err != nil {
			s.Error("Failed to close grpc: ", err)
		}
		i.v.grpcConn = nil
	}
}

func (i *devboardFixture) String() string {
	return DevBoardService + "_" + string(i.image)
}

// dialGrpc connects to the devboardsvc host.
func (i *devboardFixture) dialGrpc(ctx context.Context) error {
	conn, err := grpc.DialContext(ctx, i.hostPort, grpc.WithInsecure())
	if err != nil {
		return err
	}
	i.v.grpcConn = conn
	return nil
}
