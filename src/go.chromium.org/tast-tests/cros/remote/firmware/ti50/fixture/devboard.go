// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture provides ti50 devboard related fixtures.
package fixture

import (
	"context"
	"os"
	"strconv"
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
	grpcConn      *grpc.ClientConn
	devboard      *remoteTi50.DUTControlAndreiboard
	ImagePath     string
	FwConfigJsons []string
	TestbedType   ti50.TestbedType
}

// DevBoard returns the existing DevBoard connection instance.
func (v *Value) DevBoard() *remoteTi50.DUTControlAndreiboard {
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
	if hostPort, ok := s.Var(DevBoardService); ok {
		i.hostPort = hostPort
	} else {
		testing.ContextLogf(ctx, "-var=%s= not provided, using default: localhost:39999", DevBoardService)
		i.hostPort = "localhost:39999"
	}
	i.v = &Value{}

	if err := i.dialGrpc(ctx); err != nil {
		s.Fatal("dial grpc: ", err)
	}
	// Create devboard controller used for remainder of tests
	i.v.devboard = remoteTi50.NewDUTControlAndreiboard(i.v.grpcConn)

	testbedProperties, err := i.v.devboard.Query(ctx)
	if err != nil {
		s.Fatal("querying testbed: ", err)
	}
	i.v.TestbedType = testbedProperties.TestbedType

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
	if err := i.v.devboard.Setup(ctx, imagePath, fwConfigJsons); err != nil {
		s.Fatal("Setup: ", err)
	}

	i.v.ImagePath = imagePath
	i.v.FwConfigJsons = fwConfigJsons

	return i.v
}

func (i *devboardFixture) Reset(ctx context.Context) error {
	return nil
}

func (i *devboardFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	testing.ContextLog(ctx, "Starting OTT session")
	// At this point, start an opentitantool session, which could involve either
	// starting a host emulation instance, or resetting a devboard and its debugger to a known
	// state.
	mustSucceed(s, i.v.devboard.StartSession(ctx, ti50.StrapReset), "Start testing session")
}

func (i *devboardFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	b := i.v.devboard
	testing.ContextLog(ctx, "Ending OTT session")
	if err := b.EndSession(ctx); err != nil {
		s.Error("Failed to end session: ", err)
	}
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
	conn, err := grpc.DialContext(ctx, i.hostPort, grpc.WithInsecure(), grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(128*1024*1024), grpc.MaxCallRecvMsgSize(128*1024*1024)))
	if err != nil {
		return err
	}
	i.v.grpcConn = conn
	return nil
}

// Convenience functions copied from tpm_helper (since the helper isn't accessible at this layer)

type errorThrower interface {
	Fatalf(format string, args ...interface{})
}

func gpioSet(ctx context.Context, s errorThrower, b ti50.DevBoard, g ti50.GpioName, val bool) {
	if _, err := b.PlainCommand(ctx, "gpio", "write", string(g), strconv.FormatBool(val)); err != nil {
		s.Fatalf("Failed to set gpio %s: %s", g, err)
	}
}

func gpioApplyStrap(ctx context.Context, s errorThrower, b ti50.DevBoard, straps ...ti50.GpioStrap) {
	for _, strap := range straps {
		if _, err := b.PlainCommand(ctx, "gpio", "apply", string(strap)); err != nil {
			s.Fatalf("Failed to apply gpio strap %s: %s", strap, err)
		}
	}
}

func mustSucceed(s errorThrower, err error, format string, args ...interface{}) {
	if err != nil {
		s.Fatalf(format+": %s", append(args, err))
	}
}

func runCommand(ctx context.Context, s errorThrower, image *ti50.CrOSImage, command string) string {
	out, err := image.Command(ctx, command)
	if err != nil {
		s.Fatalf("Running Command `%s` failed: %s", command, err)
	}
	return out
}
