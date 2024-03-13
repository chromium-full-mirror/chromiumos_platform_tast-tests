// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture provides ti50 devboard related fixtures.
package fixture

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	remoteTi50 "go.chromium.org/tast-tests/cros/remote/firmware/ti50"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// DevBoardService arg name for the service's host:port pair and also the name of the fixture.
	DevBoardService = "devboardsvc"

	// SystemDevboard fixture flashes a system (ti50, cr50, etc) image and sets up a devboard connection
	SystemDevboard = "systemDevboard"

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

	cr50DebugImageTemplate = "gs://chromeos-localmirror-private/distfiles/chromeos-cr50-debug-0.0.11/h1_shield/cr50.dbg.0x%s_0x%s.bin.*"
)

type extraPreTestMethod func(ctx context.Context, board ti50.DevBoard) error

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            SystemDevboard,
		Desc:            "Uses devboardsvc to flash a system image",
		Contacts:        []string{"tast-fw-library-reviewers@google.com", "ecgh@google.com"},
		Impl:            &devboardFixture{image: SystemImage},
		Vars:            []string{DevBoardService, BuildURL, FwConfigJSON, Chip, Variant, Slot},
		Data:            defaultFwConfigs,
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
		Data:            defaultFwConfigs,
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
		Data:            defaultFwConfigs,
		SetUpTimeout:    setUpTimeout,
		ResetTimeout:    resetTimeout,
		TearDownTimeout: tearDownTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
	})
}

// Value allows tests to obtain a ti50 devboard.
type Value struct {
	grpcConn          *grpc.ClientConn
	devboard          *remoteTi50.DUTControlAndreiboard
	ImagePath         string
	FwConfigJsons     []string
	TestbedProperties remoteTi50.TestbedProperties
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

	if p, err := i.v.devboard.Query(ctx); err != nil {
		s.Fatal("querying testbed: ", err)
	} else {
		i.v.TestbedProperties = p
	}

	iv, err := downloadImage(ctx, i.v.TestbedProperties, i.image, s)
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
	if i.v.TestbedProperties.TestbedType == "gsc_h1_shield" {
		setupCr50Image(ctx, s, i.v.devboard, imagePath, fwConfigJsons, i.v.TestbedProperties)
	} else if err := i.v.devboard.Setup(ctx, imagePath, fwConfigJsons); err != nil {
		s.Fatal("Setup: ", err)
	}

	i.v.ImagePath = imagePath
	i.v.FwConfigJsons = fwConfigJsons

	return i.v
}

// setupCr50Image uses gsctool to flash the cr50 image.
// The GSC UART must be closed before calling this method since it opens it to issue commands to the board.
// TODO(b/140534392): Support changing the board id.
func setupCr50Image(ctx context.Context, s TestingState, board *remoteTi50.DUTControlAndreiboard, imagePath string, fwConfigJsons []string,
	testbedProperties remoteTi50.TestbedProperties) {
	if err := board.Setup(ctx, "", fwConfigJsons); err != nil {
		s.Fatal("Setup: ", err)
	}

	if err := board.StartSession(ctx, ti50.StrapReset); err != nil {
		s.Fatal("StartSession: ", err)
	}
	defer func() {
		if err := board.EndSession(ctx); err != nil {
			s.Fatal("EndSession: ", err)
		}
	}()

	gpioApplyStrap(ctx, s, board, ti50.CcdSuzyQ)
	if imagePath == "" {
		return
	}

	// Sometimes cr50 does not show up on the USB bus until it is reset.
	gpioSet(ctx, s, board, ti50.GpioTi50ResetL, false)
	gpioSet(ctx, s, board, ti50.GpioTi50ResetL, true)

	mustSucceed(s, board.GSCToolWaitUntilReady(ctx), "wait until gsc ready")

	_, rw, err := board.GSCToolCurrentFwVersion(ctx)
	mustSucceed(s, err, "get current fwver")

	_, imageVer, _, _, err := board.GSCToolBinVersion(ctx, imagePath)
	mustSucceed(s, err, "parse bin version")

	gscConsole := board.PhysicalUart(ti50.UartConsole)

	i, err := ti50.OpenCrOSImage(ctx, gscConsole)
	if err != nil {
		s.Fatal("Unable to open gsc console: ", err)
	}
	defer i.Close(ctx)

	if imageVer.Less(rw) {
		testing.ContextLogf(ctx, "Rollback required for flashing %s to %s", rw, imageVer)

		debugImageURL, err := findCr50DebugImage(ctx, testbedProperties)
		if err != nil {
			s.Fatal("find cr50 debug image failed: ", err)
		}

		debugImage, err := downloadToTempFile(ctx, "debug image", debugImageURL)
		mustSucceed(s, err, "download debug image")

		mustSucceed(s, board.RollbackUpdate(ctx, i, imagePath, debugImage), "rollback update to image")
	} else {
		testing.ContextLogf(ctx, "Direct gsctool update for %s to %s", rw, imageVer)
		mustSucceed(s, board.DirectUpdate(ctx, i, imagePath), "direct updateto image")
	}
}

// findCr50DebugImage finds the debug image for cr50 board.
func findCr50DebugImage(ctx context.Context, testbedProperties remoteTi50.TestbedProperties) (string, error) {
	devIds := strings.Split(testbedProperties.UsbSerial, "-")
	if len(devIds) != 2 {
		return "", errors.New("usb_serial parse error " + testbedProperties.UsbSerial)
	}

	debugImageGlob := fmt.Sprintf(cr50DebugImageTemplate, strings.ToLower(devIds[0]), strings.ToLower(devIds[1]))
	debugImageURL, err := gsLs(ctx, "list debug image", debugImageGlob)
	if err != nil || len(debugImageURL) != 1 {
		return "", errors.New("find debug image")
	}

	return debugImageURL[0], nil
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

// TestingState provides methods that can log context messages or fail test execution.
type TestingState interface {
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Log(args ...interface{})
	Logf(format string, args ...interface{})
}

func gpioSet(ctx context.Context, s TestingState, b ti50.DevBoard, g ti50.GpioName, val bool) {
	if _, err := b.PlainCommand(ctx, "gpio", "write", string(g), strconv.FormatBool(val)); err != nil {
		s.Fatalf("Failed to set gpio %s: %s", g, err)
	}
}

func gpioApplyStrap(ctx context.Context, s TestingState, b ti50.DevBoard, straps ...ti50.GpioStrap) {
	for _, strap := range straps {
		if _, err := b.PlainCommand(ctx, "gpio", "apply", string(strap)); err != nil {
			s.Fatalf("Failed to apply gpio strap %s: %s", strap, err)
		}
	}
}

func mustSucceed(s TestingState, err error, format string, args ...interface{}) {
	if err != nil {
		s.Fatalf(format+": %s", append(args, err)...)
	}
}

func runCommand(ctx context.Context, s TestingState, image *ti50.CrOSImage, command string) string {
	out, err := image.Command(ctx, command)
	if err != nil {
		s.Fatalf("Running Command `%s` failed: %s", command, err)
	}
	return out
}
