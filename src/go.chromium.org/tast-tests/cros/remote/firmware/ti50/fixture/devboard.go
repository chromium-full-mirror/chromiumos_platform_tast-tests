// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture provides ti50 devboard related fixtures.
package fixture

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
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

	setUpTimeout    = 10 * time.Minute
	resetTimeout    = 5 * time.Second
	tearDownTimeout = 5 * time.Second
	preTestTimeout  = 15 * time.Second
	postTestTimeout = rescueTwiceTimeout
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            SystemDevboard,
		Desc:            "Uses devboardsvc to flash a system image",
		Contacts:        []string{"cros-hwsec@google.com", "ecgh@google.com"},
		BugComponent:    "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
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
		Contacts:        []string{"cros-hwsec@google.com", "ecgh@google.com"},
		BugComponent:    "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
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
		Contacts:        []string{"cros-hwsec@google.com", "ecgh@google.com"},
		BugComponent:    "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
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
	grpcConn                *grpc.ClientConn
	devboard                *remoteTi50.DUTControlAndreiboard
	ImagePath               string
	FwConfigJsons           []string
	TestbedProperties       remoteTi50.TestbedProperties
	efiImagePath            string
	debugImagePath          string
	imageMayBeUpdatedByTest bool
}

// DevBoard returns the existing DevBoard connection instance.
func (v *Value) DevBoard() *remoteTi50.DUTControlAndreiboard {
	return v.devboard
}

// ImageMayBeUpdatedByTest must be called by test cases which may update (by means of CCD or
// otherwise) to other firmware images besides the one declared in this fixture.  This method
// would typically be called at the top of such tests, before any actual test logic runs.
func (v *Value) ImageMayBeUpdatedByTest() error {
	if v.ImagePath == "" {
		return errors.New("Supply buildurl")
	}
	v.imageMayBeUpdatedByTest = true
	return nil
}

// EfiImagePath returns the path to an EFI image suitable for the current testbed, if one exists.
func (v *Value) EfiImagePath(ctx context.Context) (string, error) {
	if v.efiImagePath != "" {
		return v.efiImagePath, nil
	}
	efiImage, err := DownloadEfiImage(ctx, v.TestbedProperties)
	if err != nil {
		return "", err
	}
	v.efiImagePath = efiImage
	return v.efiImagePath, nil
}

// DebugImagePath returns the path to an debug image suitable for the current testbed, if one
// exists.  Debug image is required for H1, but not strictly required for DT.
func (v *Value) DebugImagePath(ctx context.Context) (string, error) {
	if v.debugImagePath != "" {
		return v.debugImagePath, nil
	}
	debugImage, err := DownloadDebugImage(ctx, v.TestbedProperties)
	if err != nil {
		return "", err
	}
	v.debugImagePath = debugImage
	return v.debugImagePath, nil
}

type devboardFixture struct {
	image      ImageType
	imageValue *ImageValue
	hostPort   string
	v          *Value
	resultInfo ResultInfoTags
}

// ResultInfoTags stores tags to be reported with the test result.
type ResultInfoTags struct {
	// TagROVersion is the RO version of the running image.
	TagROVersion string `json:"gsc_ro_version"`
	// TagRWVersion is the RW version of the running image.
	TagRWVersion string `json:"gsc_rw_version"`
	// TagRWBranch is the branch name of the running image.
	TagRWBranch string `json:"gsc_rw_branch"`
	// TagRWRev is the revision count of the running image.
	TagRWRev string `json:"gsc_rw_rev"`
	// TagRWSHA is the git commit hash of the running image.
	TagRWSHA string `json:"gsc_rw_sha"`
	// TagBuildURL is the build URL used to download the image file.
	TagBuildURL string `json:"gsc_buildurl"`
	// TagTestbedType is the type of HW being tested (eg gsc_dt_shield).
	TagTestbedType string `json:"gsc_testbed_type"`
	// TagCCDSerial is the serial number of the GSC chip (and CCD USB serial).
	TagCCDSerial string `json:"gsc_ccd_serial"`
	// TagServiceVersion is the container version of the devboard service.
	TagServiceVersion string `json:"gsc_devboardservice_version"`
	// TagOpenTitanToolVersion is the opentitantool version.
	TagOpenTitanToolVersion string `json:"gsc_opentitantool_version"`
	// TagHyperdebugVersion is the Hyperdebug firmware version.
	TagHyperdebugVersion string `json:"gsc_hyperdebug_version"`
	// TagHyperdebugSerial is the Hyperdebug USB serial number.
	TagHyperdebugSerial string `json:"gsc_hyperdebug_serial"`
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

	serviceVersion, err := i.v.devboard.ServiceVersionInfo(ctx)
	if err != nil {
		s.Log("Could not get service version info: ", err)
	} else {
		i.resultInfo.TagServiceVersion = serviceVersion.ContainerVersion
	}

	if p, err := i.v.devboard.Query(ctx); err != nil {
		s.Fatal("querying testbed: ", err)
	} else {
		i.v.TestbedProperties = p
	}

	i.resultInfo.TagTestbedType = string(i.v.TestbedProperties.TestbedType)
	i.resultInfo.TagCCDSerial = i.v.TestbedProperties.UsbSerial
	i.resultInfo.TagOpenTitanToolVersion = i.v.TestbedProperties.OpentitantoolVersion
	i.resultInfo.TagHyperdebugVersion = i.v.TestbedProperties.HyperdebugVersion
	i.resultInfo.TagHyperdebugSerial = i.v.TestbedProperties.HyperdebugSerial
	burl, _ := s.Var(BuildURL)
	i.resultInfo.TagBuildURL = burl

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

	i.v.ImagePath = imagePath
	i.v.FwConfigJsons = fwConfigJsons

	setupImage(ctx, i.v, s)

	ver, err := i.v.devboard.GSCVersionInfo(ctx)
	if err != nil {
		s.Log("Could not get version info: ", err)
	}
	i.resultInfo.TagROVersion = ver.ROVersion
	i.resultInfo.TagRWVersion = ver.RWVersion
	i.resultInfo.TagRWBranch = ver.Branch
	i.resultInfo.TagRWRev = fmt.Sprint(ver.Rev)
	i.resultInfo.TagRWSHA = ver.SHA

	return i.v
}

// setupImage flashes the image under test on the devboard using the image and
// json files provided in the `Value` parameter.
func setupImage(ctx context.Context, v *Value, s TestingState) {
	// Host emulation does not support `Setup()` without specifying image, and it does not
	// make sense to attempt to skip flashing the image.
	if v.TestbedProperties.TestbedType != ti50.GscHostEmulation {
		// Setup the board with the correct jsons. Use an empty string for the image
		// path so setup doesn't try to flash the image.
		if err := v.devboard.Setup(ctx, "", v.FwConfigJsons); err != nil {
			s.Fatal("Setup: ", err)
		}

		if needsUpdate(ctx, s, v.devboard, v.ImagePath, false) {
			testing.ContextLog(ctx, "Image is already running")
			return
		}
	}
	testing.ContextLog(ctx, "Setting up image: ", v.ImagePath)
	if v.TestbedProperties.TestbedType == ti50.GscH1Shield {
		setupCr50Image(ctx, s, v.devboard, v.ImagePath, v.FwConfigJsons, v.TestbedProperties, false)
	} else if err := v.devboard.Setup(ctx, v.ImagePath, v.FwConfigJsons); err != nil {
		s.Fatal("Setup: ", err)
	}
}

func needsUpdate(ctx context.Context, s TestingState, board *remoteTi50.DUTControlAndreiboard, imagePath string, checkInfoSpace bool) bool {
	if imagePath == "" {
		testing.ContextLog(ctx, "No image given. Nothing to do")
		return true
	}
	if err := board.StartSession(ctx, ti50.StrapReset); err != nil {
		s.Fatal("StartSession: ", err)
	}
	defer func() {
		if err := board.EndSession(ctx); err != nil {
			s.Fatal("EndSession: ", err)
		}
	}()

	_, imageVer, _, _, err := board.GSCToolBinVersion(ctx, imagePath)
	mustSucceed(s, err, "parse bin version")

	// Sometimes cr50 does not show up on the USB bus until it is reset.
	gpioSet(ctx, s, board, ti50.GpioTi50ResetL, false)
	gpioSet(ctx, s, board, ti50.GpioTi50ResetL, true)

	gscConsole := board.PhysicalUart(ti50.UartConsole)

	i, err := ti50.OpenCrOSImage(ctx, gscConsole)
	if err != nil {
		s.Fatal("Unable to open gsc console: ", err)
	}
	defer i.Close(ctx)

	i.WaitUntilBooted(ctx)

	if checkInfoSpace {
		// Check if any fields in the info pages are set. If they are,
		// the test has to run the update.
		isErased, err := i.WriteOnceInfoPagesAreErased(ctx)
		if err != nil {
			testing.ContextLog(ctx, "Unable to check the info space: ", err)
			return false
		}
		if !isErased {
			return false
		}
	}
	// Check the running version string is found in the image file. It's possible multiple
	// images will be built with the same minor version. The version string contains the
	// git sha which should be unique per build.
	versionInfo, err := i.VersionInfo(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Unable to get the gsc version")
		return false
	}
	runningVersion := versionInfo.RwA.Version
	if versionInfo.RwB.Active {
		runningVersion = versionInfo.RwB.Version
	}
	if runningVersion != imageVer.String() {
		testing.ContextLogf(ctx, "GSC is running %s not %s", runningVersion, imageVer.String())
		return false
	}
	testing.ContextLogf(ctx, "GSC is running %s, same as image under test", runningVersion)
	if versionInfo.Build.VersionStr == "" {
		testing.ContextLog(ctx, "Did not find a valid version string. Running update")
		return false
	}
	if strings.Contains(versionInfo.Build.VersionStr, "+") {
		testing.ContextLogf(ctx, "%s is dirty", versionInfo.Build.VersionStr)
		testing.ContextLog(ctx, "Can't skip update")
		return false
	}
	runningCorrectVersionStr := false
	// Check if the image under test contains the running version string
	imageContents, err := ioutil.ReadFile(imagePath)
	if err != nil {
		testing.ContextLog(ctx, "Unable to read file", imagePath)
	} else {
		imageContentStr := string(imageContents)
		runningCorrectVersionStr = strings.Contains(imageContentStr, versionInfo.Build.VersionStr)
	}
	if runningCorrectVersionStr {
		testing.ContextLogf(ctx, "%s found in %s", versionInfo.Build.VersionStr, imagePath)
		testing.ContextLog(ctx, "GSC is running the image under test")
	} else {
		testing.ContextLogf(ctx, "%s not found in %s", versionInfo.Build.VersionStr, imagePath)
	}
	return runningCorrectVersionStr
}

// setupCr50Image uses gsctool to flash the cr50 image.
// The GSC UART must be closed before calling this method since it opens it to issue commands to the board.
// TODO(b/140534392): Support changing the board id.
func setupCr50Image(ctx context.Context, s TestingState, board *remoteTi50.DUTControlAndreiboard, imagePath string, fwConfigJsons []string,
	testbedProperties remoteTi50.TestbedProperties, runEraseFlashInfo bool) {
	if err := board.StartSession(ctx, ti50.StrapReset); err != nil {
		s.Fatal("StartSession: ", err)
	}
	defer func() {
		if err := board.EndSession(ctx); err != nil {
			s.Fatal("EndSession: ", err)
		}
	}()

	if imagePath == "" {
		return
	}

	testing.ContextLog(ctx, "Updating GSC")

	// Sometimes cr50 does not show up on the USB bus until it is reset.
	gpioSet(ctx, s, board, ti50.GpioTi50ResetL, false)
	gpioSet(ctx, s, board, ti50.GpioTi50ResetL, true)

	_, imageVer, _, _, err := board.GSCToolBinVersion(ctx, imagePath)
	mustSucceed(s, err, "parse bin version")
	gpioApplyStrap(ctx, s, board, ti50.CCDModeOn)
	mustSucceed(s, board.GSCToolWaitUntilReady(ctx), "wait until gsc ready")

	_, rw, err := board.GSCToolCurrentFwVersion(ctx)
	mustSucceed(s, err, "get current fwver")

	gscConsole := board.PhysicalUart(ti50.UartConsole)
	i, err := ti50.OpenCrOSImage(ctx, gscConsole)
	if err != nil {
		s.Fatal("Unable to open gsc console: ", err)
	}
	defer i.Close(ctx)

	i.WaitUntilBooted(ctx)

	if imageVer.Less(rw) || runEraseFlashInfo {
		testing.ContextLogf(ctx, "Rollback required for flashing %s to %s", rw, imageVer)
		debugImage, efiImage, err := DownloadGSCTestImages(ctx, testbedProperties)
		mustSucceed(s, err, "failed to download debug and efi image")

		if debugImage == "" || efiImage == "" {
			s.Fatal("Supply EFI and debug image to rollback with ccd")
		}
		err = board.RollbackAndRunEraseFlashInfoUpdate(ctx, i, imagePath, efiImage, debugImage)
		mustSucceed(s, err, "failed efi rollback update to image")
	} else {
		testing.ContextLogf(ctx, "Direct gsctool update for %s to %s", rw, imageVer)
		mustSucceed(s, board.DirectUpdate(ctx, i, imagePath), "direct updateto image")
	}
}

func (i *devboardFixture) Reset(ctx context.Context) error {
	return nil
}

func (i *devboardFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	i.logResultInfo(ctx, s)
	testing.ContextLog(ctx, "Starting OTT session")
	// At this point, start an opentitantool session, which could involve either
	// starting a host emulation instance, or resetting a devboard and its debugger to a known
	// state.
	mustSucceed(s, i.v.devboard.StartSession(ctx, ti50.StrapReset), "Start testing session")

	// Reset state variable, allowing this next test to flag if it may update the storage, for
	// `PostTest()` to inspect.
	i.v.imageMayBeUpdatedByTest = false
}

func (i *devboardFixture) logResultInfo(ctx context.Context, s *testing.FixtTestState) {
	jsonData, err := json.Marshal(i.resultInfo)
	mustSucceed(s, err, "Failed to marshal gsc_info.json")
	err = os.WriteFile(filepath.Join(s.OutDir(), "gsc_info.json"), jsonData, 0666)
	mustSucceed(s, err, "Failed to write gsc_info.json")
}

func (i *devboardFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	b := i.v.devboard
	testing.ContextLog(ctx, "Ending OTT session")
	if err := b.EndSession(ctx); err != nil {
		s.Error("Failed to end session: ", err)
	}
	if i.v.imageMayBeUpdatedByTest {
		// Let's make sure that the image from this fixture is set up correctly after
		// finishing the test since it's possible the test has updated to something else.
		setupImage(ctx, i.v, s)
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

func runCommandExpectOutput(ctx context.Context, s TestingState, image *ti50.CrOSImage, command string, expect *regexp.Regexp) []string {
	out, err := image.Command(ctx, command)
	if err != nil {
		s.Fatalf("Running Command `%s` failed: %s", command, err)
	}
	match := expect.FindStringSubmatch(out)
	if match == nil {
		s.Fatalf("Running Command `%s` unexpected output: %s", command, out)
	}
	return match
}
