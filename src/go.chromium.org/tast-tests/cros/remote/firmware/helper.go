// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	gossh "golang.org/x/crypto/ssh"

	"go.chromium.org/chromiumos/config/go/api"
	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/common/firmware/usb"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast-tests/cros/remote/firmware/rpm"
	fwpb "go.chromium.org/tast-tests/cros/services/cros/firmware"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/framework/protocol"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

// Helper tracks several firmware-related objects. The recommended way to initialize the helper is to use firmware.fixture:
//
// import (
//
//	...
//	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
//
// )
//
//	func init() {
//		testing.AddTest(&testing.Test{
//			...
//	             Fixture: fixture.NormalMode,
//		})
//	}
//
//	func MyTest(ctx context.Context, s *testing.State) {
//		h := s.FixtValue().(*fixture.Value).Helper
//
//		if err := h.RequireServo(ctx); err != nil {
//			s.Fatal("Failed to init servo: ", err)
//		}
//
// ...
// }
type Helper struct {
	// BiosServiceClient provides bios related services such as GBBFlags manipulation.
	BiosServiceClient fwpb.BiosServiceClient

	// KernelServiceClient provides kernel related services such as reading CGPT table.
	KernelServiceClient fwpb.KernelServiceClient

	// TPMServiceClient provides TPM related services.
	TPMServiceClient fwpb.TPMServiceClient

	// Board contains the DUT's board, as reported by the Platform RPC.
	// Currently, this is based on /etc/lsb-release's CHROMEOS_RELEASE_BOARD.
	Board string

	// Config contains a variety of platform-specific attributes.
	Config *Config

	// cfgFilepath is the full path to the data directory containing fw-testing-configs JSON files.
	// Any tests requiring a Config should set cfgFilepath to s.DataPath(firmware.ConfigFile) during NewHelper.
	cfgFilepath string

	// These vars track whether the DUT's on-board image, and the USB images are known to have up-to-date Tast host files.
	dutInternalStorageHasTastFiles bool
	dutUsbHasTastFiles             bool

	// DUT is used for communicating with the device under test.
	DUT *dut.DUT

	// hostFilesTmpDir is a temporary directory on the test server holding a copy of Tast host files.
	hostFilesTmpDir string

	// Model contains the DUT's model, as reported by the Platform RPC.
	// Currently, this is based on cros_config / name.
	Model string

	// Reporter reports various info from the DUT.
	Reporter *reporters.Reporter

	// RPCClient is a direct client connection to the Tast gRPC server hosted on the DUT.
	RPCClient *rpc.Client

	// disallowServices prevents RequireRPCClient from working if set.
	disallowServices bool

	// rpcHint is needed in order to create an RPC client connection.
	rpcHint *testing.RPCHint

	// RPCUtils allows the Helper to call the firmware utils RPC service.
	RPCUtils fwpb.UtilsServiceClient

	// Servo allows us to send commands to a servo device.
	Servo *servo.Servo

	// servoHostPort is the address and port of the machine acting as the servo host, normally provided via the "servo" command-line variable.
	servoHostPort string
	keyFile       string
	keyDir        string

	// ServoProxy wraps the Servo object, and communicates with the servod instance.
	ServoProxy *servo.Proxy

	// RPM is a remote power management client. Only valid in the test lab.
	RPM *rpm.RPM

	// dutHostname is the real name of the dut, even if tast is connected to a forwarded port.
	dutHostname string

	// powerunitHostname, powerunitOutlet, hydraHostname identify the managed power outlet for the DUT.
	powerunitHostname, powerunitOutlet, hydraHostname string

	// eccrashFileCache holds a cache of the current ec crash files in /var/spool/crash/ (without file ext).
	ecCrashFileCache map[string]bool

	// CheckECCrash holds a value that a test can set to false to skip checking for ec crashes.
	CheckECCrash bool

	// HasAPFwState indicates that the DUT firmware version supports reading firmware screen IDs from the EC console.
	HasAPFwState bool
}

// WaitConnectOption includes situations to wait to connect from.
type WaitConnectOption int

const (
	// FromHibernation alerts WaitConnect to skip
	// on setting servo control while DUT is still
	// in the process of waking up from hibernation.
	FromHibernation WaitConnectOption = iota

	// ResetEthernetDongle resets the ethernet dongle
	// for speed-up in ssh connection.
	ResetEthernetDongle

	// SkipPDRoleSnk skips setting pd role as snk if setting
	// servo.DFP fails during WaitConnect.
	SkipPDRoleSnk
)

// SetupUSBOption includes options for setting up a USB device.
type SetupUSBOption int

const (
	// DontFlashIfSameMilestone indicates that flashing the usb
	// is not required if it contains the same ChromeOS milestone
	// as that running currently on the DUT.
	DontFlashIfSameMilestone SetupUSBOption = iota
)

// ECCrashBaseDir is the path to a dir on DUT where ec crash files are stored.
const ECCrashBaseDir = "/var/spool/crash/"

// NewHelper creates a new Helper object with info from testing.State.
// For tests that do not use a certain Helper aspect (e.g. RPC or Servo), it is OK to pass null-values (nil or "").
func NewHelper(d *dut.DUT, rpcHint *testing.RPCHint, cfgFilepath, servoHostPort, dutHostname, powerunitHostname, powerunitOutlet, hydraHostname string) *Helper {
	var reporter *reporters.Reporter
	if d != nil {
		reporter = reporters.New(d)
	}
	return &Helper{
		cfgFilepath:       cfgFilepath,
		DUT:               d,
		keyFile:           d.KeyFile(),
		keyDir:            d.KeyDir(),
		Reporter:          reporter,
		rpcHint:           rpcHint,
		servoHostPort:     servoHostPort,
		dutHostname:       dutHostname,
		powerunitHostname: powerunitHostname,
		powerunitOutlet:   powerunitOutlet,
		hydraHostname:     hydraHostname,
	}
}

// NewHelperWithoutDUT creates a new Helper object with info from testing.State. The resulting Helper will be unable to ssh to the DUT.
func NewHelperWithoutDUT(cfgFilepath, servoHostPort, keyFile, keyDir string) *Helper {
	return &Helper{
		cfgFilepath:   cfgFilepath,
		keyFile:       keyFile,
		keyDir:        keyDir,
		servoHostPort: servoHostPort,
	}
}

// Close shuts down any firmware objects associated with the Helper.
// Generally, tests should defer Close() immediately after initializing a Helper.
func (h *Helper) Close(ctx context.Context) error {
	var allErrors []error
	if h.hostFilesTmpDir != "" {
		if err := os.RemoveAll(h.hostFilesTmpDir); err != nil {
			allErrors = append(allErrors, errors.Wrap(err, "removing server's copy of Tast host files"))
		}
		h.hostFilesTmpDir = ""
	}
	if err := h.CloseRPCConnection(ctx); err != nil {
		allErrors = append(allErrors, errors.Wrap(err, "closing rpc connection"))
	}
	if err := h.CloseServo(ctx); err != nil {
		allErrors = append(allErrors, errors.Wrap(err, "closing servo"))
	}
	if len(allErrors) > 0 {
		for err := range allErrors[1:] {
			testing.ContextLog(ctx, "Suppressed error: ", err)
		}
		return allErrors[0]
	}
	return nil
}

// EnsureDUTBooted checks the power state, and attempts to boot the DUT if it is off.
func (h *Helper) EnsureDUTBooted(ctx context.Context) error {
	if h.DUT != nil {
		testing.ContextLog(ctx, "Connecting to DUT")
		if err := h.DUT.Connect(ctx); err == nil {
			h.CloseRPCConnection(ctx)
			return nil
		}
	}
	if err := h.RequireServo(ctx); err != nil {
		return errors.Wrap(err, "could not connect to servo")
	}
	if hasEC, err := h.Servo.HasControl(ctx, string(servo.ECSystemPowerState)); err != nil {
		testing.ContextLog(ctx, "Error checking for chrome ec: ", err)
	} else if hasEC {
		state, err := h.Servo.GetECSystemPowerState(ctx)
		if err != nil {
			testing.ContextLog(ctx, "Error getting power state: ", err)
		}
		if state == "S0" {
			testing.ContextLog(ctx, "Waiting for DUT to finish booting")
			// The machine is up, just wait for it to finish booting.
			h.CloseRPCConnection(ctx)
			waitBootCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if err = h.WaitConnect(waitBootCtx); err == nil {
				return nil
			}
			// If WaitConnect didn't work, let it reset.
		}
	}
	testing.ContextLog(ctx, "Connecting power")
	if err := h.SetDUTPower(ctx, true); err != nil {
		testing.ContextLog(ctx, "Failed to connect charger: ", err)
	}

	// Cr50 goes to sleep during hibernation and battery cutoff, and when DUT wakes, CCD state might be locked.
	if val, err := h.Servo.GetString(ctx, servo.GSCCCDLevel); err != nil {
		testing.ContextLog(ctx, "Failed to get gsc_ccd_level: ", err)
	} else if val != servo.Open {
		testing.ContextLogf(ctx, "CCD is not open, got %q. Attempting to unlock", val)
		if err := h.Servo.SetString(ctx, servo.GSCTestlab, servo.Open); err != nil {
			testing.ContextLog(ctx, "Failed to unlock CCD: ", err)
		}
	}

	testing.ContextLog(ctx, "Resetting DUT")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		testing.ContextLog(ctx, "Failed to reset DUT: ", err)
	}
	if err := h.DisconnectDUT(ctx); err != nil {
		testing.ContextLog(ctx, "Error closing connections to DUT: ", err)
	}
	h.CloseRPCConnection(ctx)
	return h.WaitConnect(ctx)
}

// DisallowServices prevents RequireRPCClient from being used for the lifetime of this Helper.
func (h *Helper) DisallowServices() {
	h.disallowServices = true
}

// AllowServices allows use of RequireRPCClient for the lifetime of this Helper.
func (h *Helper) AllowServices() {
	if h.DUT != nil {
		h.disallowServices = false
	}
}

// RequireRPCClient creates a client connection to the DUT's gRPC server, unless a connection already exists.
func (h *Helper) RequireRPCClient(ctx context.Context) error {
	if h.disallowServices {
		return errors.New("RPC services disabled by fixture")
	}
	if h.RPCClient != nil {
		return nil
	}
	// rpcHint comes from testing.State, so it needs to be manually set in advance.
	if h.rpcHint == nil {
		return errors.New("cannot create RPC client connection without rpcHint")
	}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 1*time.Minute)
	defer cancelWaitConnect()

	if err := h.WaitConnect(waitConnectCtx); err != nil {
		return errors.Wrap(err, "dut not connected")
	}
	testing.ContextLog(ctx, "Opening RPCClient connection")
	cl, err := rpc.Dial(ctx, h.DUT, h.rpcHint)
	if err != nil {
		return errors.Wrap(err, "dialing RPC connection")
	}
	h.RPCClient = cl
	return nil
}

// RequireRPCUtils creates a firmware.UtilsServiceClient, unless one already exists.
func (h *Helper) RequireRPCUtils(ctx context.Context) error {
	if h.RPCUtils != nil {
		return nil
	}
	if err := h.RequireRPCClient(ctx); err != nil {
		return errors.Wrap(err, "requiring RPC client")
	}
	h.RPCUtils = fwpb.NewUtilsServiceClient(h.RPCClient.Conn)
	return nil
}

// RequireKernelServiceClient creates a firmware.KernelServiceClient, unless one already exists.
func (h *Helper) RequireKernelServiceClient(ctx context.Context) error {
	if h.KernelServiceClient != nil {
		return nil
	}
	if err := h.RequireRPCClient(ctx); err != nil {
		return errors.Wrap(err, "requiring RPC client")
	}
	h.KernelServiceClient = fwpb.NewKernelServiceClient(h.RPCClient.Conn)
	return nil
}

// RequireBiosServiceClient creates a firmware.BiosServiceClient, unless one already exists.
// You must add `SoftwareDeps: []string{"flashrom"},` to your `testing.Test` to use this.
func (h *Helper) RequireBiosServiceClient(ctx context.Context) error {
	if h.BiosServiceClient != nil {
		return nil
	}
	if err := h.RequireRPCClient(ctx); err != nil {
		return errors.Wrap(err, "requiring RPC client")
	}
	h.BiosServiceClient = fwpb.NewBiosServiceClient(h.RPCClient.Conn)
	return nil
}

// RequireTPMServiceClient creates a firmware.TPMServiceClient, unless one already exists.
func (h *Helper) RequireTPMServiceClient(ctx context.Context) error {
	if h.TPMServiceClient != nil {
		return nil
	}
	if err := h.RequireRPCClient(ctx); err != nil {
		return errors.Wrap(err, "requiring RPC client")
	}
	h.TPMServiceClient = fwpb.NewTPMServiceClient(h.RPCClient.Conn)
	return nil
}

// CloseRPCConnection shuts down the RPC client (if present), and removes any RPC clients that the Helper was tracking.
func (h *Helper) CloseRPCConnection(ctx context.Context) error {
	defer func() {
		h.RPCClient, h.RPCUtils, h.BiosServiceClient, h.KernelServiceClient, h.TPMServiceClient = nil, nil, nil, nil, nil
	}()
	if h.RPCClient != nil {
		testing.ContextLog(ctx, "Closing RPCClient connection")
		if err := h.RPCClient.Close(ctx); err != nil {
			return errors.Wrap(err, "closing rpc client")
		}
	}
	return nil
}

// DisconnectDUT shuts down all connections to the DUT. Call this after you have powered down the DUT.
func (h *Helper) DisconnectDUT(ctx context.Context) error {
	rpcerr := h.CloseRPCConnection(ctx)
	// Disconnect the dut even if the rpc connection failed to close.
	if h.DUT != nil {
		duterr := h.DUT.Disconnect(ctx)
		if duterr != nil {
			return duterr
		}
	}
	return rpcerr
}

// RequirePlatform fetches the DUT's board and model and caches them, unless they have already been cached.
func (h *Helper) RequirePlatform(ctx context.Context) error {
	if h.Board == "" {
		board, err := h.Reporter.Board(ctx)
		if err != nil {
			return errors.Wrap(err, "getting DUT board")
		}
		h.Board = strings.ToLower(board)
	}
	if h.Model == "" {
		model, err := h.Reporter.Model(ctx)
		// Ignore error, as not all boards have a model.
		if err == nil {
			h.Model = strings.ToLower(model)
		} else {
			testing.ContextLogf(ctx, "Failed to get DUT model for board %s: %+v", h.Board, err)
		}
	}
	return nil
}

// OverridePlatform sets board and model if the passed in params are not blank.
func (h *Helper) OverridePlatform(ctx context.Context, board, model string) {
	if board != "" {
		h.Board = strings.ToLower(board)
	}
	if model != "" {
		h.Model = strings.ToLower(model)
	}
}

// RequireConfig creates a firmware.Config, unless one already exists.
// This requires your test to have `Data: []string{firmware.ConfigFile},` in its `testing.Test` block.
func (h *Helper) RequireConfig(ctx context.Context) error {
	if h.Config != nil {
		return nil
	}
	if err := h.RequirePlatform(ctx); err != nil {
		return errors.Wrap(err, "requiring DUT platform")
	}
	// cfgFilepath comes from testing.State, so it needs to be passed during NewHelper.
	if h.cfgFilepath == "" {
		return errors.New("cannot create firmware Config with a null Helper.cfgFilepath")
	}
	cfg, err := NewConfig(h.cfgFilepath, h.Board, h.Model)
	if err != nil {
		return errors.Wrapf(err, "during NewConfig with board=%s, model=%s", h.Board, h.Model)
	}
	h.Config = cfg
	return nil
}

// RequireServo creates a servo.Servo, unless one already exists.
func (h *Helper) RequireServo(ctx context.Context) error {
	if h.Servo != nil {
		return nil
	}
	pxy, err := servo.NewProxy(ctx, h.servoHostPort, h.keyFile, h.keyDir)
	if err != nil {
		return errors.Wrap(err, "connecting to servo")
	}
	h.ServoProxy = pxy
	h.Servo = pxy.Servo()
	return nil
}

// CloseServo closes the connection to the servo, use RequireServo to open it again.
func (h *Helper) CloseServo(ctx context.Context) error {
	defer func() {
		h.ServoProxy = nil
		h.Servo = nil
		h.RPM = nil
	}()
	var err error
	if h.RPM != nil {
		if err = h.RPM.Close(ctx); err != nil {
			err = errors.Wrap(err, "failed to close rpm client")
		}
	}
	if h.ServoProxy != nil {
		h.ServoProxy.Close(ctx)
	}
	return err
}

const (
	// dutLocalRunner, dutLocalBundleDir, and dutLocalDataDir are paths on the DUT containing Tast host files.
	dutLocalRunner    = "/usr/local/bin/local_test_runner"
	dutLocalBundleDir = "/usr/local/libexec/tast/"
	dutLocalDataDir   = "/usr/local/share/tast/"

	// tmpLocalRunner, tmpLocalBundleDir, and tmpLocalDataDir are relative paths, within a tempdir on the server, to copies of Tast host files.
	tmpLocalRunner    = "local-runner"
	tmpLocalBundleDir = "local-bundle-dir/"
	tmpLocalDataDir   = "local-data-dir/"
)

// DoesServerHaveTastHostFiles determines whether the test server has a copy of Tast host files.
func (h *Helper) DoesServerHaveTastHostFiles() bool {
	return h.hostFilesTmpDir != ""
}

// DUTHasNoTastFilesInternalDisk clears the flag indicating that the DUT has tast files on the internal disk.
func (h *Helper) DUTHasNoTastFilesInternalDisk() {
	h.dutInternalStorageHasTastFiles = false
}

// CopyTastFilesFromDUT retrieves Tast host files from the DUT and stores them locally for later use.
// This allows the test server to re-push Tast files to the DUT if a different OS image is booted mid-test.
func (h *Helper) CopyTastFilesFromDUT(ctx context.Context) error {
	if h.DoesServerHaveTastHostFiles() {
		return errors.New("cannot copy Tast files from DUT twice")
	}

	// Create temp dir to hold copied Tast files.
	tmpDir, err := os.MkdirTemp("", "tast-host-files-copy")
	if err != nil {
		return err
	}
	h.hostFilesTmpDir = tmpDir

	// Copy files from DUT onto test server.
	testing.ContextLogf(ctx, "Copying Tast host files to test server at %s", tmpDir)
	for dutSrc, serverDst := range map[string]string{
		dutLocalRunner:    filepath.Join(tmpDir, tmpLocalRunner),
		dutLocalBundleDir: filepath.Join(tmpDir, tmpLocalBundleDir),
		dutLocalDataDir:   filepath.Join(tmpDir, tmpLocalDataDir),
	} {
		// Only copy the file if it exists.
		if err = h.DUT.Conn().CommandContext(ctx, "test", "-x", dutSrc).Run(); err == nil {
			if err = linuxssh.GetFile(ctx, h.DUT.Conn(), dutSrc, serverDst, linuxssh.PreserveSymlinks); err != nil {
				return errors.Wrapf(err, "copying local Tast file %s from DUT", dutSrc)
			}
		} else if _, ok := err.(*gossh.ExitError); !ok {
			return errors.Wrapf(err, "checking for existence of %s", dutSrc)
		}
	}
	return nil
}

// SyncTastFilesToDUT copies the test server's copy of Tast host files back onto the DUT. This is only necessary if you want to use gRPC services.
// TODO(jbettis): When Autotest SSP tarballs contain local Tast test bundles, refactor this code
// so that it pushes Tast files to the DUT via the same means as the upstream Tast framework.
// As of the time of this writing, that is not possible; see http://g/tast-owners/sBhC1w-ET8g.
func (h *Helper) SyncTastFilesToDUT(ctx context.Context) error {
	if !h.DoesServerHaveTastHostFiles() {
		return errors.New("must copy Tast files from DUT before syncing back onto DUT")
	}
	fileMap := map[string]string{
		filepath.Join(h.hostFilesTmpDir, tmpLocalRunner):    dutLocalRunner,
		filepath.Join(h.hostFilesTmpDir, tmpLocalBundleDir): dutLocalBundleDir,
		filepath.Join(h.hostFilesTmpDir, tmpLocalDataDir):   dutLocalDataDir,
	}
	for key := range fileMap {
		if _, err := os.Stat(key); os.IsNotExist(err) {
			delete(fileMap, key)
		}
	}

	testing.ContextLog(ctx, "Syncing Tast files from test server onto DUT: ", fileMap)
	if _, err := linuxssh.PutFiles(ctx, h.DUT.Conn(), fileMap, linuxssh.DereferenceSymlinks); err != nil {
		return errors.Wrap(err, "failed syncing Tast files from test server onto DUT")
	}
	return nil
}

func (h *Helper) validateUSBImage(ctx context.Context, usbdev string, cloudStorage *testing.CloudStorage, opts ...SetupUSBOption) (bool, error) {
	mountPath := fmt.Sprintf("/media/servo_usb/%d", h.ServoProxy.GetPort())
	usbRelease, usbMilestone, err := usb.ValidateUSBImage(ctx, usbdev, mountPath, h.ServoProxy)
	if err != nil {
		return false, err
	}
	dutBuilderPath, err := h.Reporter.BuilderPath(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get DUT builder path")
	}
	if strings.Contains(dutBuilderPath, "-postsubmit") {
		testing.ContextLogf(ctx, "Current build on DUT (%s) is not a release image, using %s from USB stick", dutBuilderPath, usbRelease)
		if usbRelease == "" {
			return false, errors.New("refusing to use -postsubmit version")
		}
		return true, nil
	}

	for _, opt := range opts {
		if opt == DontFlashIfSameMilestone {
			dutMilestone, err := h.Reporter.Milestone(ctx)
			if err != nil {
				return false, errors.Wrap(err, "failed to get DUT milestone")
			}
			if usbMilestone == dutMilestone {
				testing.ContextLog(ctx, "USB image contains the same milestone as the one running on the DUT")
				return true, nil
			}
		}
	}

	if usbRelease == dutBuilderPath {
		testing.ContextLogf(ctx, "Current build on USB (%s) matches DUT (%s), no need to download", usbRelease, dutBuilderPath)
		return true, nil
	}

	if cloudStorage == nil {
		if usbRelease == "" {
			return false, errors.New("bad USB image, and requested no USB image download")
		}
		testing.ContextLogf(ctx, "User requested no USB image download, using %s even though it differs from DUT %s", usbRelease, dutBuilderPath)
		return true, nil
	}
	testing.ContextLogf(ctx, "Current build on USB (%s) differs from DUT (%s), proceed with download", usbRelease, dutBuilderPath)
	return false, nil
}

// SetupUSBKey prepares the USB disk for a test. (Borrowed from Tauto's firmware_test.py)
// It checks the setup of USB disk and a valid ChromeOS test image inside.
// Downloads the test image if the image isn't the right version.
// Will break the DUT if it is currently booted off the USB drive in recovery mode.
//
// CAUTION: You must set ephemeraldevserver='false' in your control file in order to flash usb drives.
func (h *Helper) SetupUSBKey(ctx context.Context, cloudStorage *testing.CloudStorage, opts ...SetupUSBOption) (retErr error) {
	usbdev := ""
	// Do a simple ImageUSBKeyDev if the user said not to flash, don't check the device very carefully.
	if cloudStorage == nil {
		var err error
		usbdev, err = h.Servo.GetStringTimeout(ctx, servo.ImageUSBKeyDev, time.Second*90)
		if err != nil {
			return errors.Wrap(err, "servo call image_usbkey_dev failed")
		}
		if usbdev == "" {
			return errors.New("no USB key detected")
		}
	} else {
		var err error
		usbdev, err = h.CheckUSBOnServoHost(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to check the usb device on servo host")
		}
	}
	if h.DUT.Connected(ctx) {
		if valid, err := h.validateUSBImage(ctx, usbdev, cloudStorage, opts...); valid && err == nil {
			return nil
		} else if err != nil {
			return errors.Wrap(err, "failed to validate USB image")
		}
	}

	// Sometimes servod loses the CCD connection while we are flashing the USB drive.
	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		return errors.Wrap(err, "failed to remove ccd watchdog")
	}

	// If it did have tast files, it won't shortly.
	h.dutUsbHasTastFiles = false

	// Find a devserver that works from servo host, and flash image from there.
	for _, devserver := range cloudStorage.Devservers() {
		testing.ContextLogf(ctx, "Trying devserver at %q", devserver)
		if err := h.ServoProxy.RunCommand(ctx, false, "curl", "-f", "-s", "-S", "--connect-timeout", "3", fmt.Sprintf("%s/check_health", devserver)); err != nil {
			testing.ContextLog(ctx, "Devserver not healthy: ", err)
			continue
		}
		artifactsURL := strings.TrimSuffix(cloudStorage.BuildArtifactsURL(), "/")
		stagingURL := fmt.Sprintf("%s/stage?archive_url=%s&files=chromiumos_test_image.tar.xz", devserver, artifactsURL)
		testing.ContextLogf(ctx, "Staging image %q", stagingURL)
		if err := h.ServoProxy.RunCommand(ctx, false, "curl", "-f", "-s", "-S", stagingURL); err != nil {
			testing.ContextLogf(ctx, "Failed to stage file at %q: %v", stagingURL, err)
			continue
		}
		testImageURL := fmt.Sprintf("%s/extract/%s/chromiumos_test_image.tar.xz?file=chromiumos_test_image.bin", devserver, strings.TrimPrefix(artifactsURL, "gs://"))

		testing.ContextLogf(ctx, "Flashing test OS image to USB from %q", testImageURL)
		if err := h.Servo.SetStringTimeout(ctx, servo.DownloadImageToUSBDev, testImageURL, 2*time.Hour); err != nil {
			if strings.Contains(string(err.Error()), "Read-only file system") {
				modelName, serialNumber, err := h.getUSBModelAndSerial(ctx, usbdev)
				if err != nil {
					testing.ContextLog(ctx, "Failed to get info about usb: ", err)
				}
				return errors.Errorf("failed to flash os image and found usb device as read-only file system, got usb model: %s, serial number: %s", modelName, serialNumber)
			}
			return errors.Wrapf(err, "failed to flash os image %q to USB %q", testImageURL, usbdev)
		}
		if h.DUT.Connected(ctx) {
			// ensure that image was successfully flashed by reading back OS version
			if valid, err := h.validateUSBImage(ctx, usbdev, cloudStorage, opts...); valid && err != nil {
				return errors.Wrap(err, "failed to validate USB image after flashing")
			}
		}
		testing.ContextLogf(ctx, "Successfully flashed %q from %q", usbdev, testImageURL)
		return nil
	}
	return errors.New("no devservers able to stage ChromeOS test image")
}

// CorruptUSBKey makes a minimal change to the USB key to prevent it from booting. Use RestoreUSBKey to repair it afterwards.
func (h *Helper) CorruptUSBKey(ctx context.Context, usbdev string) (retErr error) {
	defer func() {
		if retErr != nil {
			testing.ContextLog(ctx, "Checking for USB model name and serial numbers")
			modelName, serialNumber, err := h.getUSBModelAndSerial(ctx, usbdev)
			if err != nil {
				retErr = errors.Wrapf(err, "got %v, but failed to get usb info", retErr)
			} else {
				retErr = errors.Errorf("got %v, and found usb model %s, serial number %s", retErr, modelName, serialNumber)
			}
		}
	}()
	testing.ContextLog(ctx, "Corrupting ChromeOS image name on usbkey")
	// ChromeOS kernel is at /dev/sdx2.
	kernelPart := usbdev + "2"
	stdin := strings.NewReader("CORRUPTD")

	if err := h.ServoProxy.InputCommand(ctx, true, stdin, "dd", fmt.Sprintf("of=%s", kernelPart), "oflag=sync", "conv=notrunc,nocreat"); err != nil {
		return errors.Wrap(err, "failed to corrupt kernel magic")
	}
	return nil
}

// RestoreUSBKey corrects the minimal change to the usb key made by CorruptUSBKey.
func (h *Helper) RestoreUSBKey(ctx context.Context) (retErr error) {
	// This call is super slow.
	var err error
	usbdev, err := h.Servo.GetStringTimeout(ctx, servo.ImageUSBKeyDev, time.Second*90)
	if err != nil {
		return errors.Wrap(err, "servo call image_usbkey_dev failed")
	}
	if usbdev == "" {
		return errors.New("no USB key detected")
	}
	testing.ContextLogf(ctx, "Sleeping %s to let USB become visible to servo host", UsbVisibleTime)
	// GoBigSleepLint: It takes some time for usb mux state to take effect.
	if err := testing.Sleep(ctx, UsbVisibleTime); err != nil {
		return err
	}
	testing.ContextLog(ctx, "Uncorrupting ChromeOS image name on usbkey")
	// ChromeOS kernel is at /dev/sdx2.
	kernelPart := usbdev + "2"
	stdin := strings.NewReader("CHROMEOS")

	if err := h.ServoProxy.InputCommand(ctx, true, stdin, "dd", fmt.Sprintf("of=%s", kernelPart), "oflag=sync", "conv=notrunc,nocreat"); err != nil {
		return errors.Wrap(err, "failed to restore kernel magic")
	}
	return nil
}

func checkUSBStorage(ctx context.Context, usbInfo string, minimalSize float64) error {
	regexpUSBInfo := regexp.MustCompile(`\w+\D+(\w+\D\w+) GiB`)
	match := regexpUSBInfo.FindStringSubmatch(string(usbInfo))
	if match[0] == "" {
		return errors.New("no regexp match found")
	}
	usbSizeInString := strings.TrimSpace(match[1])
	usbSizeInFloat, err := strconv.ParseFloat(usbSizeInString, 64)
	if err != nil {
		return errors.Wrap(err, "cannot convert the format of usb size into a floating-point number")
	}

	if usbSizeInFloat < minimalSize {
		return errors.Errorf("USB storage is too small: current usb is %v GiB, but should be %v GiB or bigger", usbSizeInFloat, minimalSize)
	}
	return nil
}

// WaitForPowerStates polls for DUT to get to a specific powerstate
func (h *Helper) WaitForPowerStates(ctx context.Context, interval, timeout time.Duration, powerStates ...string) error {
	// Try reading the power state from the EC.
	err := testing.Poll(ctx, func(c context.Context) error {
		currPowerState, err := h.Servo.GetECSystemPowerState(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to check powerstate")
		}
		pattern := `^AP_POWER_STATE_[A-Z][0-9]+[A-Z]*$`
		matched, _ := regexp.MatchString(pattern, currPowerState)
		if matched {
			new := `[A-Z][0-9]+[A-Z]*$`
			re := regexp.MustCompile(new)
			match := re.FindStringSubmatch(currPowerState)

			state := match[0]
			testing.ContextLog(ctx, "state: ", state)
			if !comparePowerStates(state, powerStates...) {
				return errors.Errorf("failed to get expected power state: want %v; got %s", powerStates, state)
			}
		} else {
			if !comparePowerStates(currPowerState, powerStates...) {
				return errors.Errorf("failed to get expected power state: want %v; got %s", powerStates, currPowerState)
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout, Interval: interval})
	if err != nil {
		return errors.Errorf("failed to get one of %v power state: %s", powerStates, err)
	}
	return nil
}

func comparePowerStates(currState string, expectedStates ...string) bool {
	for _, state := range expectedStates {
		if strings.EqualFold(state, currState) {
			return true
		}
	}
	return false
}

func (h *Helper) waitDutS0(ctx context.Context) error {
	const reconnectRetryDelay = time.Second
	testing.ContextLog(ctx, "Waiting DUT to reach S0")
	if err := h.WaitForPowerStates(ctx, reconnectRetryDelay, 1*time.Minute, "S0"); err != nil {
		return errors.Wrap(err, "wait for S0")
	}
	testing.ContextLog(ctx, "Sleeping 20s for boot to finish")
	// GoBigSleepLint: Sleep time determined empirically through trial and error.
	if err := testing.Sleep(ctx, 20*time.Second); err != nil {
		return errors.Wrap(err, "sleep 20s")
	}
	return nil
}

// wcOptsContain determines whether a slice of WaitConnectOption contains a specific Option.
func wcOptsContain(opts []WaitConnectOption, contained WaitConnectOption) bool {
	for _, v := range opts {
		if v == contained {
			return true
		}
	}
	return false
}

// WaitConnect is similar to DUT.WaitConnect, except that it works with RO EC firmware.
// Pass a context with a deadline if you don't want to wait forever.
// If --var noSSH=true is set, this degrades to waiting for S0 + a sleep.
func (h *Helper) WaitConnect(ctx context.Context, opts ...WaitConnectOption) error {
	const reconnectRetryDelay = time.Second

	deadline, ok := ctx.Deadline()
	if ok {
		totalWaitTime := time.Until(deadline)
		if totalWaitTime < 2*time.Second {
			// There isn't enough time to connect
			return errors.Errorf("context timeout too short, need at least %s, got %s", 2*time.Second, totalWaitTime)
		}
	}
	if err := h.RequireServo(ctx); err != nil {
		return errors.Wrap(err, "failed to connect to servo")
	}
	if h.DUT == nil {
		return h.waitDutS0(ctx)
	}
	// Resetting ethernet dongle might speed up connection to the dut.
	// At the moment, it seems that only servo v4.1 supports this feature.
	if wcOptsContain(opts, ResetEthernetDongle) {
		if err := h.ResetServoEthernetDongle(ctx); err != nil {
			return errors.Wrap(err, "failed to reset ethernet")
		}
	}
	hasDUTPDDataRole, err := h.Servo.HasControl(ctx, string(servo.DUTPDDataRole))
	if err != nil {
		return errors.Wrap(err, "failed to check for control")
	}
	testing.ContextLogf(ctx, "Waiting for %s to connect", h.DUT.HostName())
	dfpFailures := 0
	for {
		deadline, ok := ctx.Deadline()
		if ok {
			remainingTime := time.Until(deadline)
			if remainingTime < 2*time.Second {
				// There isn't enough time to connect
				return context.DeadlineExceeded
			}
		}
		// SetDUTPDDataRole would fail when DUT is still in the process
		// of waking up from hibernation.
		if !wcOptsContain(opts, FromHibernation) && hasDUTPDDataRole {
			if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
				dfpFailures++
				if dfpFailures == 3 && !wcOptsContain(opts, SkipPDRoleSnk) {
					ok, err := h.Servo.HasControl(ctx, string(servo.PDRole))
					if err != nil {
						testing.ContextLogf(ctx, "Failed to check for %q control: %s", servo.PDRole, err)
					} else if ok {
						err = h.Servo.SetPDRole(ctx, servo.PDRoleSnk)
						if err != nil {
							testing.ContextLogf(ctx, "Failed to set pd role to %q: %s", servo.PDRoleSnk, err)
						}
					}
				}
			}
		}
		err := func(ctx context.Context) error {
			waitCtx, cancelConnect := context.WithTimeout(ctx, 10*time.Second)
			defer cancelConnect()
			return h.DUT.Connect(waitCtx)
		}(ctx)
		if err == nil {
			return nil
		}

		select {
		case <-time.After(reconnectRetryDelay):
		case <-ctx.Done():
			if err.Error() == ctx.Err().Error() {
				return err
			}
			return errors.Wrapf(err, "context error = %v", ctx.Err())
		}
	}
}

// RequireRPM creates the RPM client in h.RPM.
func (h *Helper) RequireRPM(ctx context.Context) error {
	if h.RPM != nil {
		return nil
	}
	if err := h.RequireServo(ctx); err != nil {
		return err
	}
	var err error
	if h.ServoProxy.Proxied() {
		h.RPM, err = rpm.NewLabRPM(ctx, h.ServoProxy, h.dutHostname, h.powerunitHostname, h.powerunitOutlet, h.hydraHostname)
	} else {
		h.RPM, err = rpm.NewLabRPM(ctx, nil, h.dutHostname, h.powerunitHostname, h.powerunitOutlet, h.hydraHostname)
	}
	if err != nil {
		return errors.Wrap(err, "new rpm client")
	}
	return nil
}

// SetDUTPower turns the DUT's power on or off. Uses servo v4 pd role if possible, and falls back to RPM.
// To use RPM the command line vars `powerunitHostname` and `powerunitOutlet` must be set.
// `dutHostname` can be used to override the DUT's hostname, if ssh and rpm have different names.
// For plugs attached to hyrda, also set var `hydraHostname`.
func (h *Helper) SetDUTPower(ctx context.Context, powerOn bool) error {
	// Try servo SetPDRole (servo v4 type C).
	isServoTypeC, err := h.Servo.IsServoTypeC(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to check the connection type")
	}
	if isServoTypeC {
		role := servo.PDRoleSnk
		if powerOn {
			role = servo.PDRoleSrc
			if err := h.Servo.SetOnOff(ctx, servo.DTSMode, servo.On); err != nil {
				return errors.Wrap(err, "set dts on")
			}
			testing.ContextLogf(ctx, "Set %s:%s", servo.DTSMode, servo.On)
		}

		// servo will fail to turn CC off if DUT is already unpowered (battery cutoff).
		// Since any real errors with servo will show up again in `SetPDRole`, keep this as warning.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if err := h.Servo.ServoCcOff(ctx); err != nil {
				return err
			}
			return nil
		}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 1 * time.Second}); err != nil {
			testing.ContextLogf(ctx, "Cannot reset CC on Servo: %s", err)
		}

		if err := h.Servo.SetPDRole(ctx, role); err != nil {
			return errors.Wrap(err, "set pd role")
		}
		testing.ContextLogf(ctx, "SetPDRole: %q", role)
		return nil
	}
	// Try rpm client
	if h.powerunitHostname != "" {
		testing.ContextLogf(ctx, "Creating rpm client %s", h.powerunitHostname)
		if err := h.RequireRPM(ctx); err != nil {
			return err
		}
		powerState := rpm.Off
		if powerOn {
			powerState = rpm.On
		}
		testing.ContextLog(ctx, "Setting power via rpm")
		if ok, err := h.RPM.SetPower(ctx, powerState); err != nil {
			return errors.Wrap(err, "set power via rpm")
		} else if !ok {
			return errors.Errorf("rpm client did not set power state to %s", powerState)
		}
		return nil
	}
	return testing.PollBreak(errors.New("servo does not support pd role and no rpm vars provided"))
}

// OpenCCD verifies if CCD is open, and if not, tries to use
// ccd testlab open. If that fails, it will attempt regular ap
// open via usb, or run the host command 'gsctool -a -o'.
// When testlab is disabled, OpenCCDNoTestlab would be called,
// which might take up to 8 minutes.
// Args:
//
//	ensureTestlab: If true, this will ensure testlab enabled after CCD is open.
//	resetCCD: If true, reset ccd to factory mode after open.
//	ccdLevel: Should contain the current ccd level as returned by GetCCDLevel().
func (h *Helper) OpenCCD(ctx context.Context, ensureTestlab, resetCCD bool) error {
	// Get CCD current status.
	ccdLevel, err := h.GetCCDLevel(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get CCD level")
	}

	// Check testlab state.
	testlab, err := h.GetTestlabState(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get testlab state")
	}

	switch ccdLevel {
	case servo.Open:
		testing.ContextLog(ctx, "CCD is ", ccdLevel)
	case servo.Unlock:
		// The "unlock" state requires a password to change CCD
		// state to "open". But, if testlab was enabled, we could
		// directly make use of testlab.
		if testlab == "off" {
			return errors.New("found CCD unlock and testlab off, enable testlab before running test")
		}
		testing.ContextLog(ctx, "WARNING: CCD has password set, but testlab enabled")
		fallthrough
	case servo.Lock:
		// Attempt to open CCD.
		if testlab == "off" {
			if err := h.OpenCCDNoTestlab(ctx); err != nil {
				return errors.Wrap(err, "while opening CCD with no testlab")
			}
		} else {
			if err := h.Servo.SetString(ctx, servo.GSCTestlab, servo.Open); err != nil {
				return errors.Wrap(err, "while opening CCD with testlab")
			}

			if err := h.VerifyCCDIsOpen(ctx); err != nil {
				return errors.Wrap(err, "after attempting to open CCD with testlab")
			}
		}
	default:
		return errors.New("Unidentified ccd level: " + ccdLevel)
	}

	// By request, ensure testlab is enabled.
	if ensureTestlab && testlab != string(servo.Enable) {
		if err := h.Servo.SetTestlab(ctx, servo.Enable); err != nil {
			return errors.Wrap(err, "failed to enable testlab")
		}
	}

	// By request, reset capabilities to factory mode.
	if resetCCD {
		out, err := h.Servo.RunGSCCommandGetOutput(ctx, "ccd reset factory", []string{`[^>]*> `})
		if err != nil {
			return errors.Wrap(err, "failed resetting capabilities to factory mode")
		}
		// CR50 prints: Opening factory  settings.
		// Ti50 prints nothing
		if strings.Contains(out[0][0], "Access Denied") {
			return errors.Errorf("unexpected ccd reset factory output: %s", out[0][0])
		}
	}

	return nil
}

// GetTestlabState will try to get the current ccd testlab state by servo or gsctool command.
func (h *Helper) GetTestlabState(ctx context.Context) (string, error) {
	// Verify if cr50_testlab control exists.
	hasTestlab, err := h.Servo.HasControl(ctx, string(servo.GSCTestlab))
	if err != nil {
		return "", errors.Wrap(err, "failed while checking for cr50_testlab control")
	}

	var testlab string
	if hasTestlab {
		testlab, err = h.Servo.GetString(ctx, servo.GSCTestlab)
		if err != nil {
			testing.ContextLog(ctx, "WARNING: failed to get cr50_testlab: ", err)
		}
	}

	if testlab == "" {
		out, err := h.DUT.Conn().CommandContext(ctx, "gsctool", "-a", "-I").Output(ssh.DumpLogOnError)
		if err != nil {
			return "", errors.Wrap(err, "failed to run 'gsctool -a -I'")
		}
		re := regexp.MustCompile(`Flags:\D+(\w+)`)
		flags := re.FindStringSubmatch(string(out))
		if len(flags) != 2 {
			return "", errors.New("failed to find ccd flags from gsctool")
		}
		value, err := strconv.ParseInt(flags[1], 0, 64)
		if err != nil {
			return "", errors.Wrapf(err, "bad flag value from gsctool %s", flags)
		}
		// An odd number indicates testlab is enabled: b/172219984.
		if value&1 != 0 {
			testlab = "on"
		} else {
			testlab = "off"
		}
	}

	return testlab, nil
}

// GetCCDLevel will try to get the current ccd level by servo or gsctool command.
func (h *Helper) GetCCDLevel(ctx context.Context) (string, error) {
	// Verify if gsc_ccd_level control exists.
	hasCCDLevel, err := h.Servo.HasControl(ctx, string(servo.GSCCCDLevel))
	if err != nil {
		return "", errors.Wrap(err, "failed while checking for gsc_ccd_level control")
	}

	var ccdLevel string
	if hasCCDLevel {
		ccdLevel, err = h.Servo.GetStringTimeout(ctx, servo.GSCCCDLevel, 20*time.Second)
		if err == nil {
			return ccdLevel, nil
		}
		testing.ContextLog(ctx, "WARNING! failed to get gsc_ccd_level: ", err)
	}
	testing.ContextLog(ctx, "WARNING! using ssh to get ccd level")

	if !h.DUT.Connected(ctx) {
		if err := h.DUT.Connect(ctx); err != nil {
			return "", errors.Wrap(err, "failed to connect before gsctool")
		}
	}
	out, err := h.DUT.Conn().CommandContext(ctx, "gsctool", "-a", "-I").Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to run 'gsctool -a -I'")
	}

	re := regexp.MustCompile(`State:(\s*\w*)`)
	ccdLevel = re.FindString(string(out))
	if ccdLevel == "" {
		return "", errors.New("unable to tell ccd level")
	}
	levelSplit := strings.Split(string(ccdLevel), " ")
	ccdLevel = levelSplit[1]

	// Match the gsctool string output with the ones defined for servo.
	switch ccdLevel {
	case "Locked":
		ccdLevel = servo.Lock
	case "Opened":
		ccdLevel = servo.Open
	case "Unlocked":
		ccdLevel = servo.Unlock
	default:
		return "", errors.New("unhandled ccd level, found " + ccdLevel)
	}

	return ccdLevel, nil
}

// VerifyCCDIsOpen verifies if the current ccd level is open.
func (h *Helper) VerifyCCDIsOpen(ctx context.Context) error {
	ccdLevel, err := h.GetCCDLevel(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get CCD level")
	}

	if ccdLevel != servo.Open {
		return errors.New("CCD is not open")
	}
	return nil
}

// OpenCCDNoTestlab opens CCD when it's locked and testlab disabled.
// From observation, this process would generally take 11 minutes.
// Servo micro is required in order to open CCD without testlab enabled.
func (h *Helper) OpenCCDNoTestlab(ctx context.Context) (retErr error) {
	// Check if there is micro-servo connected.
	hasMicroOrC2D2, err := h.Servo.PreferDebugHeader(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to verify the prefered debug header")
	}

	// Record the initial DUT mode to reset it after opening ccd, if required.
	initMode, err := h.Reporter.CurrentBootMode(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to check boot mode")
	}

	ms, err := NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create new boot mode switcher")
	}

	// Restore DUT's boot mode to the initial mode if
	// it ends up in a different one after CCD is open.
	defer func(ctx context.Context, initMode fwCommon.BootMode) {
		currentMode, err := h.Reporter.CurrentBootMode(ctx)
		if err != nil {
			retErr = errors.Join(retErr, errors.Wrap(err, "failed to check boot mode after opening CCD'"))
		}

		if currentMode != initMode {
			testing.ContextLogf(ctx, "Current boot mode %q, switching back to %q", currentMode, initMode)
			if err := ms.RebootToMode(ctx, initMode); err != nil {
				retErr = errors.Join(retErr, errors.Wrap(err, "failed to reboot into initial mode"))
			}
		}
	}(ctx, initMode)

	// Verify OpenNoDevMode status.
	_, openNoDevMode, err := h.Servo.GetCCDCapability(ctx, servo.OpenNoDevMode)
	if err != nil {
		return errors.Wrap(err, "failed to get OpenNoDevMode capability")
	}

	// DUT needs to be in devMode to perform the CCD open procedure
	// if OpenNoDevMode is not accessible (i.e. !="Y").
	if openNoDevMode != "Y" && initMode != fwCommon.BootModeDev {
		testing.ContextLog(ctx, "Rebooting into DevMode to open CCD")
		if err := ms.RebootToMode(ctx, fwCommon.BootModeDev); err != nil {
			return errors.Wrap(err, "failed to reboot into dev mode to open CCD")
		}
	}

	// Verify OpenFromUSB status.
	_, openFromUSB, err := h.Servo.GetCCDCapability(ctx, servo.OpenFromUSB)
	if err != nil {
		return errors.Wrap(err, "failed to get OpenFromUSB capability")
	}

	// Verify OpenNoLongPP status.
	_, openNoLongPP, err := h.Servo.GetCCDCapability(ctx, servo.OpenNoLongPP)
	if err != nil {
		return errors.Wrap(err, "failed to get OpenNoLongPP capability")
	}

	// Verify OpenNoTPMWipe status.
	_, openNoTPMWipe, err := h.Servo.GetCCDCapability(ctx, servo.OpenNoTPMWipe)
	if err != nil {
		return errors.Wrap(err, "failed to get OpenNoTPMWipe capability")
	}

	// Enable stream for cr50 log, so that if the process for opening ccd
	// fails later, we could process the log for error messaging.
	closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.GSCUARTCapture)
	if err != nil {
		return errors.Wrap(err, "failed to enable capture Cr50 UART")
	}
	defer func() { retErr = errors.Join(retErr, closeUART(ctx)) }()

	// If OpenFromUSB is accessible (i.e. ="Y"), we can send the request through USB.
	// Otherwise, we need to send the request through the AP.
	if openFromUSB == "Y" {
		testing.ContextLog(ctx, "Opening CCD by USB")
		err = h.Servo.RunGSCCommand(ctx, "ccd open")
	} else {
		testing.ContextLog(ctx, "Opening CCD by gsctool")
		err = h.DUT.Conn().CommandContext(ctx, "gsctool", "-a", "-o").Start()
	}
	if err != nil {
		return errors.Wrap(err, "failed to run the command to open CCD")
	}

	// If OpenNoLongPP is not accessible (i.e. !="Y"), pressPowerSequence will be performed.
	// When both openNoLongPP and openNoTPMWipe are accessible (i.e. ="Y"),
	// opening CCD would not require power presses or rebooting DUT.
	if openNoLongPP != "Y" {
		if err := h.pressPowerSequenceToOpenCCD(ctx, openNoTPMWipe, hasMicroOrC2D2); err != nil {
			return errors.Wrap(err, "failed while pressing power button")
		}
	} else {
		if openNoTPMWipe != "Y" {
			// Some DUTs might take time to initiate the rebooting process
			// if OpenNoTPMWipe is not accessible (i.e. !="Y").
			testing.ContextLog(ctx, "Waiting for DUT to be unreachable")
			waitDisconnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
			defer cancelWaitConnect()

			if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
				// Based on Testhaus results, some devices weren't rebooting after 'gsctool -a -o',
				// and the cr50 console reports 'nvmem_erase_tpm_data_selective' failures. Document
				// this failure in the returned error message.
				errorMsg := errors.Wrap(err, "failed to wait for DUT to become unreachable")
				captureCr50Msg := `nvmem_erase_tpm_data_selective: adding var failed!`
				out, err := h.GetUartOutputAndSaveToFile(ctx, servo.GSCUARTStream, "open_ccd_cr50")
				if err != nil {
					return err
				}
				r := regexp.MustCompile(captureCr50Msg)
				matches := r.FindAllStringSubmatch(out, -1)
				if len(matches) > 0 {
					newErr := errors.Errorf("found %q appeared %d times", captureCr50Msg, len(matches))
					return errors.Join(errorMsg, newErr)
				}
				return errorMsg
			}
		}
	}

	if openNoTPMWipe != "Y" {
		// Wait for DUT to reconnect if OpenNoTPMWipe is not accessible (i.e. !="Y").
		testing.ContextLog(ctx, "Reconnecting to DUT")
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 6*time.Minute)
		defer cancelWaitConnect()

		if err := h.WaitConnect(waitConnectCtx); err != nil {
			// As reported on ticket b/276237637, some devices were found stuck in a loop
			// of multiple requests for the tpm reset, until reaching timeout in WaitConnect()
			// and failing the test. Document this failure in the returned error message.
			errorMsg := errors.Wrap(err, "failed to reconnect to DUT")
			captureCr50Msg := `tpm_reset_request: already scheduled`
			out, err := h.GetUartOutputAndSaveToFile(ctx, servo.GSCUARTStream, "open_ccd_cr50")
			if err != nil {
				return err
			}
			r := regexp.MustCompile(captureCr50Msg)
			matches := r.FindAllStringSubmatch(out, -1)
			if len(matches) > 0 {
				newErr := errors.Errorf("found %q appeared %d times", captureCr50Msg, len(matches))
				return errors.Join(errorMsg, newErr)
			}
			return errorMsg
		}
	}

	// Verify CCD is open.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err = h.VerifyCCDIsOpen(ctx); err != nil {
			return errors.Wrap(err, "while verifying CCD level at the end of OpenCCDNoTestlab")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 1 * time.Second}); err != nil {
		return err
	}
	return nil
}

// GetUartOutputAndSaveToFile reads from the given uart stream, saves the output,
// and returns it. You may need to call EnableUARTCapture() before.
func (h *Helper) GetUartOutputAndSaveToFile(ctx context.Context, stream servo.StringControl, fileName string) (string, error) {
	out, err := h.Servo.GetQuotedString(ctx, stream)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read %s", stream)
	}
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return "", errors.New("failed to get remote context directory")
	}
	destPath := filepath.Join(outDir, fileName+".log")
	if err := os.WriteFile(destPath, []byte(out), 0666); err != nil {
		return "", errors.Wrapf(err, "failed to write %s", destPath)
	}
	return out, nil
}

// pressPowerSequenceToOpenCCD will perform power presses for 5 minutes or until DUT disconnects.
func (h *Helper) pressPowerSequenceToOpenCCD(ctx context.Context, openNoTPMWipe string, hasMicroOrC2D2 bool) error {
	if !hasMicroOrC2D2 {
		return errors.New("micro-servo is required for valid power button presses")
	}

	testing.ContextLog(ctx, "Starting power button presses")
	ppTimeout := 5 * time.Minute
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
			return err
		}

		if h.DUT.Connected(ctx) {
			// Space an esc key between each press on the power button
			// to clear the power menu, preventing DUT from shutting down.
			if err := h.Servo.PressKey(ctx, "<esc>", servo.DurTab); err != nil {
				return errors.Wrap(err, "failed to press the esc key")
			}
			return errors.New("DUT still connected")
		}
		return nil
	}, &testing.PollOptions{Timeout: ppTimeout, Interval: 2 * time.Second}); err != nil {
		// When openNoTPMWipe is accessible (i.e. ="Y"), DUT would NOT
		// reboot at the end of the power pressing sequence. As a result,
		// DUT would still be connected.
		if !(openNoTPMWipe == "Y" && strings.Contains(err.Error(), "DUT still connected")) {
			return errors.Wrapf(err, "failed while pressing power button for %s with OpenNoTPMWipe status %v", ppTimeout, openNoTPMWipe)
		}
	}
	return nil
}

// CheckUSBOnServoHost checks if there is any usb device connected to the host and gets its path.
func (h *Helper) CheckUSBOnServoHost(ctx context.Context) (string, error) {
	if ok, err := h.Servo.HasControl(ctx, string(servo.ImageUSBKeyDirection)); err != nil {
		return "", errors.Wrap(err, "failed to talk to servod")
	} else if !ok {
		return "", errors.New("Servo with USB stick required")
	}
	testing.ContextLog(ctx, "Validating image usbkey on servo")
	// Power cycling the USB key helps to make it visible to the host.
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		return "", errors.Wrap(err, "failed to set dut_sees_usbkey")
	}
	// GoBigSleepLint: It takes some time for usb mux state to take effect.
	if err := testing.Sleep(ctx, UsbVisibleTime); err != nil {
		return "", errors.Wrap(err, "failed to sleep")
	}
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		return "", errors.Wrap(err, "failed to power off usbkey")
	}
	// GoBigSleepLint: It takes some time for usb mux state to take effect.
	if err := testing.Sleep(ctx, UsbDisableTime); err != nil {
		return "", errors.Wrap(err, "failed to sleep")
	}
	// This call is super slow.
	var err error
	usbdev, err := h.Servo.GetStringTimeout(ctx, servo.ImageUSBKeyDev, time.Second*90)
	if err != nil {
		return "", errors.Wrap(err, "servo call image_usbkey_dev failed")
	}
	if usbdev == "" {
		return "", errors.New("no USB key detected")
	}
	// Document usb model and serial numbers for debugging purposes.
	var modelName, serialNumber string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var errInPoll error
		if modelName, serialNumber, errInPoll = h.getUSBModelAndSerial(ctx, usbdev); errInPoll != nil {
			return errInPoll
		}
		testing.ContextLogf(ctx, "Got usb model: %s, serial number: %s", modelName, serialNumber)
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 1 * time.Second}); err != nil {
		testing.ContextLog(ctx, "Failed to get info about usb: ", err)
	}
	testing.ContextLog(ctx, "Sleeping 120 seconds before listing usb content")
	// GoBigSleepLint: Some USBs would drop connection after a short period of time.
	// Wait for 2 minutes before listing usb content to ensure that full information
	// would be captured. If the usb disconnected, running fdisk would likely output
	// an error similar to the following: 'fdisk: cannot open /dev/sdb: No such file or directory'.
	if err := testing.Sleep(ctx, 120*time.Second); err != nil {
		return "", errors.Wrap(err, "failed to sleep for 120 seconds")
	}
	var fdiskOutput, stderr []byte
	// Verify that the device really exists on the servo host.
	err = testing.Poll(ctx, func(ctx context.Context) error {
		fdiskOutput, stderr, err = h.ServoProxy.SeparatedOutputCommand(ctx, true, "fdisk", "-l", usbdev)
		return err
	}, &testing.PollOptions{
		Timeout:  10 * time.Second,
		Interval: 1 * time.Second,
	})
	if err != nil {
		if stderr != nil {
			stderrResult := regexp.MustCompile(`\n`).ReplaceAllString(string(stderr), " ")
			return "", errors.Errorf("validate usb key at %q, got stderr: %s, usb model: %s, serial number: %s", usbdev, strings.TrimSpace(stderrResult), modelName, serialNumber)
		}
		return "", errors.Wrapf(err, "validate usb key at %q", usbdev)
	}
	testing.ContextLogf(ctx, "Output from fdisk -l %q: %s", usbdev, fdiskOutput)
	// Following ChromiumOS Developer Guide, USB size should be bigger than 8GB:
	// https://chromium.googlesource.com/chromiumos/docs/+/HEAD/developer_guide.md#put-your-image-on-a-usb-disk
	// But the actual images appear to be 6.15GiB, so verify at 6.5 instead.
	if err := checkUSBStorage(ctx, string(fdiskOutput), 6.5); err != nil {
		return "", errors.Wrapf(err, "failed to verify usb storage, got usb model: %s, serial number: %s", modelName, serialNumber)
	}
	return usbdev, nil
}

// getUSBModelAndSerial accepts the device path of a usb, and
// returns its model name and serial numbers.
func (h *Helper) getUSBModelAndSerial(ctx context.Context, usbdev string) (string, string, error) {
	var model, serial = "unknown", "unknown"
	sysfsPath, err := h.ServoProxy.OutputCommand(ctx, true, "udevadm", "info", "-q", "path", "-n", usbdev)
	if err != nil {
		return model, serial, errors.Wrapf(err, "failed to get the sysfs path for %s", usbdev)
	}
	// Running udevadm test on a device path would cite all the rules involved,
	// for example, '60-persistent-storage.rules'. It would also print debug
	// information, including model name, serial numbers, vendor ID, etc.
	pathStr := strings.TrimSpace(string(sysfsPath))
	bOut, err := h.ServoProxy.OutputCommand(ctx, true, "udevadm", "test", "--action=add", pathStr)
	if err != nil {
		return model, serial, errors.Wrapf(err, "failed to run udevadm test on %s", usbdev)
	}
	var (
		reModel  = regexp.MustCompile(`(?i)ID_MODEL=(.+)`)
		reSerial = regexp.MustCompile(`(?i)ID_SERIAL_SHORT=(.+)`)
	)
	if foundModel := reModel.FindStringSubmatch(string(bOut)); foundModel != nil {
		model = foundModel[1]
	}
	if foundSerial := reSerial.FindStringSubmatch(string(bOut)); foundSerial != nil {
		serial = foundSerial[1]
	}
	return model, serial, nil
}

// WaitDUTConnectDuringBootFromUSB will check if the DUT boots from USB or not.
// If expBoot is true, DUT is expected to boot from USB successfully.
// If expBoot is false, DUT is expected to reach a specific firmware screen,
// as documented in the firmware test manual below:
// https://chromium.googlesource.com/chromiumos/docs/+/HEAD/firmware_test_manual.md#firmware-screen-names
func (h *Helper) WaitDUTConnectDuringBootFromUSB(ctx context.Context, expBoot bool) (retErr error) {
	if expBoot {
		defer func() {
			if errors.As(retErr, &context.DeadlineExceeded) {
				apPower, screenState, err := h.Servo.GetAPState(ctx)
				if err != nil {
					retErr = errors.Join(retErr, err)
				} else {
					retErr = errors.Join(retErr, errors.Errorf("found ap power state %s and screen state %s", apPower, screenState))
				}
			}
		}()
		if hasControl, err := h.Servo.HasControl(ctx, string(servo.GSCUARTCapture)); err != nil {
			return errors.Wrapf(err, "failed while checking for %s control", servo.GSCUARTCapture)
		} else if hasControl {
			// We found a few instances where the DUT reset in the middle of
			// booting the USB in recovery mode, for example, on crota and delbin.
			// Capture relevant Uart messages to report these instances.
			closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.GSCUARTCapture)
			if err != nil {
				return errors.Wrap(err, "failed to enable Cr50 uart capture")
			}
			defer func() {
				if retErr != nil {
					tpmRstAsserted := regexp.MustCompile(`tpm_rst_asserted|PLT_RST_L ASSERTED`)
					if found, err := h.Servo.PollForRegexp(ctx, servo.GSCUARTStream, tpmRstAsserted, 10*time.Second); found {
						retErr = errors.Join(retErr, errors.New("unexpected reset captured"))
					} else {
						retErr = errors.Join(retErr, err)
					}
				}
				retErr = errors.Join(retErr, closeUART(ctx))
			}()
			// Read the UART stream just to make sure there isn't buffered data.
			if _, err := h.Servo.GetQuotedString(ctx, servo.GSCUARTStream); err != nil {
				return errors.Wrap(err, "failed to read GSC UART")
			}
		}
	}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.USBImageBootTimeout)
	defer cancelWaitConnect()

	err := h.WaitConnect(waitConnectCtx, ResetEthernetDongle)
	switch err.(type) {
	case nil:
		if expBoot {
			testing.ContextLog(ctx, "Checking that DUT has booted from a removable device")
			fromRemovableDevice, err := h.Reporter.BootedFromRemovableDevice(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to check for the boot device type")
			}
			if !fromRemovableDevice {
				return errors.New("DUT did not boot from a removable device")
			}
			return nil
		}
		return errors.Wrap(err, "expected DUT at the firmware screen. But, DUT booted to the welcome page")
	default:
		if !expBoot {
			if !errors.As(err, &context.DeadlineExceeded) {
				return errors.Wrap(err, "expected dut disconnected")
			}
			return nil
		}
		return errors.Wrapf(err, "expected dut reconnected: %t", expBoot)
	}
}

// powerSupplyDeviceStates contains status info for ac and battery.
type powerSupplyDeviceStates struct {
	ACOnline      string
	BatteryStatus string
}

// NoBatteryInfoErr is the error returned by CheckPowerSupplyDeviceStates
// when there's no information found associated with the battery.
type NoBatteryInfoErr struct {
	*errors.E
}

// CheckPowerSupplyDeviceStates runs the command 'power_supply_info' and returns
// device status, specifically for ac and battery.
func (h *Helper) CheckPowerSupplyDeviceStates(ctx context.Context) (*powerSupplyDeviceStates, error) {
	type deviceInfoEmptyErr struct {
		*errors.E
	}
	devices := map[string]*regexp.Regexp{
		"ac":      regexp.MustCompile(`online:\s+(.+)`),
		"battery": regexp.MustCompile(`state:\s+(.+)`),
	}
	checkState := func(expMatch *regexp.Regexp) (string, error) {
		out, err := h.DUT.Conn().CommandContext(ctx, "power_supply_info").Output()
		if err != nil {
			return "", errors.Wrap(err, "failed to get power supply info")
		}
		matches := expMatch.FindStringSubmatch(string(out))
		if len(matches) != 2 {
			return "", &deviceInfoEmptyErr{
				E: errors.Errorf("failed to match regex %q in %q", expMatch, string(out))}
		}
		return matches[1], nil
	}
	var err error
	var data powerSupplyDeviceStates
	if data.ACOnline, err = checkState(devices["ac"]); err != nil {
		return nil, err
	}
	if data.BatteryStatus, err = checkState(devices["battery"]); err != nil {
		if _, ok := err.(*deviceInfoEmptyErr); ok {
			return nil, &NoBatteryInfoErr{E: errors.Wrap(err, "getting battery status")}
		}
		return nil, err
	}
	return &data, nil
}

// CheckBatteryAvailable checks if the DUT has a working battery.
func (h *Helper) CheckBatteryAvailable(ctx context.Context) (bool, error) {
	_, err := h.CheckPowerSupplyDeviceStates(ctx)
	if err != nil {
		if _, ok := err.(*NoBatteryInfoErr); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CheckChgFrmPwrSuppInfo compares the ac status from the remote
// command, 'power_supply_info', against an expected value. This
// is especially useful for devices that don't support servo control
// 'charger_attached', such as those in the Wilco family.
func (h *Helper) CheckChgFrmPwrSuppInfo(ctx context.Context, connectAC bool) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		deviceStates, err := h.CheckPowerSupplyDeviceStates(ctx)
		if err != nil {
			return err
		}
		switch connectAC {
		case true:
			if deviceStates.ACOnline != "yes" {
				return errors.Errorf("expected ac online, but got %s", deviceStates.ACOnline)
			}
		case false:
			if deviceStates.ACOnline != "no" {
				return errors.Errorf("expected ac offline, but got %s", deviceStates.ACOnline)
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 2 * time.Second}); err != nil {
		return err
	}
	return nil
}

// ResetServoEthernetDongle resets the ethernet adapter if it supports.
func (h *Helper) ResetServoEthernetDongle(ctx context.Context) error {
	ok, err := h.Servo.HasControl(ctx, string(servo.DutEthPwrEn))
	if err != nil {
		return errors.Wrapf(err, "failed to check control %v", servo.DutEthPwrEn)
	}
	if !ok {
		return nil
	}
	testing.ContextLog(ctx, "Resetting the ethernet adapter")
	return h.Servo.ToggleOffOn(ctx, servo.DutEthPwrEn)
}

// SaveCBMEMLogs saves the CBMEM logs from last boot-up.
func (h *Helper) SaveCBMEMLogs(ctx context.Context, saveLogPath string, logType ...reporters.CBMEMLogType) error {
	out, err := h.Reporter.GetCBMEMLogs(ctx, logType...)
	if err != nil {
		return errors.Wrap(err, "failed to run cbmem command")
	}

	if err := os.WriteFile(saveLogPath, []byte(out), 0666); err != nil {
		return errors.Wrapf(err, "failed to write firmware log to %s", saveLogPath)
	}
	return nil
}

// RestartUI logs out any potential chrome sessions.
func (h *Helper) RestartUI(ctx context.Context) error {
	cmd := h.DUT.Conn().CommandContext(ctx, "restart", "ui")
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to restart ui")
	}
	scanner := bufio.NewScanner(stderr)
	errMsg := ""
	for scanner.Scan() {
		errMsg = scanner.Text()
	}
	if err := cmd.Wait(); err != nil {
		testing.ContextLog(ctx, "Error while restarting ui: ", errMsg)
		if strings.Contains(errMsg, "Unknown instance") || strings.Contains(errMsg, "Job has already been stopped") {
			if err := h.DUT.Conn().CommandContext(ctx, "start", "ui").Run(); err != nil {
				return errors.Wrap(err, "failed to start ui")
			}
		}
	}
	return nil
}

// EnableDevBootUSB sets crossystem dev_boot_usb to 1.
func (h *Helper) EnableDevBootUSB(ctx context.Context) error {
	testing.ContextLog(ctx, "Enabling dev_boot_usb")
	return h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_boot_usb=1").Run(ssh.DumpLogOnError)
}

// DisableDevBootUSB sets crossystem dev_boot_usb to 0.
func (h *Helper) DisableDevBootUSB(ctx context.Context) error {
	testing.ContextLog(ctx, "Disabling dev_boot_usb")
	return h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_boot_usb=0").Run(ssh.DumpLogOnError)
}

// WaitFirmwareScreen waits until the DUT is in firmware with the keyboard
// ready by actively sending the ec command '8042 kbd', which is supported
// mostly on x86 devices. On platforms that don't support this command, such
// as arm devices, it sleeps for the full 'timeout' duration.
// Note: Ensure that the DUT was fully shut down first before calling
// WaitFirmwareScreen to wait for firmware screen from power-on.
func (h *Helper) WaitFirmwareScreen(ctx context.Context, timeout time.Duration) (retErr error) {
	type ecCmd8042NotFound struct {
		*errors.E
	}
	if err := h.RequireConfig(ctx); err != nil {
		return errors.Wrap(err, "failed to create config")
	}
	reKeyboardEnabled := `Enabled: (\d)`
	re8042NotFound := `(Command '8042'|8042: command) not found( or ambiguous)?`
	checkKeyboardEnabled := `(` + reKeyboardEnabled + `|` + re8042NotFound + `)`
	if h.Config.HasECCapability(ECX86) {
		defer func() {
			testing.ContextLog(ctx, "Restoring chan")
			if err := h.Servo.RestoreDUTConsoleChannelMask(ctx); err != nil {
				retErr = errors.Join(retErr, errors.Wrap(err, "failed to send 'chan restore'"))
			}
		}()
		if err := h.Servo.SaveDUTConsoleChannelMask(ctx); err != nil {
			return errors.Wrap(err, "failed to send 'chan save' to EC")
		}
		if err := h.Servo.SetDUTConsoleChannelMask(ctx, 0); err != nil {
			return errors.Wrap(err, "failed to send 'chan 0' to EC")
		}
		err := testing.Poll(ctx, func(ctx context.Context) error {
			out, err := h.Servo.RunECCommandGetOutput(ctx, "8042 kbd", []string{checkKeyboardEnabled})
			if err != nil {
				return err
			}
			if match := regexp.MustCompile(re8042NotFound).FindStringSubmatch(out[0][0]); match != nil {
				return testing.PollBreak(&ecCmd8042NotFound{E: errors.New("did not find ec command '8042'")})
			}
			if keyboardEnabled := regexp.MustCompile(reKeyboardEnabled).FindStringSubmatch(out[0][0]); keyboardEnabled != nil {
				if val, err := strconv.ParseInt(string(keyboardEnabled[1]), 0, 0); err != nil {
					return errors.Wrap(err, "failed to parse keyboard enabled flag")
				} else if val == 1 {
					// GoBigSleepLint: On some machines, for example
					// zork/gumboz and hatch/nightfury, we've seen that when
					// the keyboard was found enabled, there might still be a
					// slight delay until the firmware screen was displayed.
					// Sleeping for 3 seconds usually helped.
					if err := testing.Sleep(ctx, 3*time.Second); err != nil {
						return errors.Wrap(err, "failed to sleep")
					}
					return nil
				}
			}
			return errors.New("keyboard disabled")
		}, &testing.PollOptions{Interval: time.Millisecond * 200, Timeout: timeout})
		if err == nil {
			return nil
		}
		// We waited the full timeout, so it's fine.
		if strings.Contains(err.Error(), "context deadline exceeded during a poll") {
			testing.ContextLog(ctx, "Ignoring error: ", err)
			return nil
		}
		// Fall through to sleep if the error is ecCmd8042NotFound
		_, ok := err.(*ecCmd8042NotFound)
		if !ok {
			return err
		}
	}
	testing.ContextLogf(ctx, "Sleeping for %s", timeout)
	// GoBigSleepLint: Wait for firmware screen.
	if err := testing.Sleep(ctx, timeout); err != nil {
		return errors.Wrapf(err, "sleeping for %s", timeout)
	}
	return nil
}

// DetectFirmwareScreen detects the firmware screen from the ec console.
// Make sure to enable ec uart capture before reboot and close it
// after finishing reboot.
func (h *Helper) DetectFirmwareScreen(ctx context.Context, timeout time.Duration, screen fwCommon.FwScreenID) error {
	screenID := strconv.FormatInt(int64(screen), 16)
	var fwScreenRe *regexp.Regexp = regexp.MustCompile(`AP_FW ` + screenID)
	if found, err := h.Servo.PollForRegexp(ctx, servo.ECUARTStream, fwScreenRe, timeout); err != nil {
		return errors.Wrap(err, "gsc output parsing failed")
	} else if !found {
		return errors.Errorf("failed to find pattern %s in the output", fwScreenRe)
	}
	testing.ContextLog(ctx, "Found AP_FW ", screenID)
	// GoBigSleepLint: On some machines, there might still be a
	// slight delay until the firmware screen was displayed.
	// Sleeping for 3 seconds usually helped.
	if err := testing.Sleep(ctx, 3*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	return nil
}

// ECTabletLaptopModeCtrl contains the control name for setting DUT in tablet,
// or laptop mode.
type ECTabletLaptopModeCtrl int

// These are some available controls for SetECTabletLaptopMode.
const (
	SetECTabletMode ECTabletLaptopModeCtrl = iota
	SetECLaptopMode
)

// TabletLaptopModeCmds contains the ec commands for switching to tablet or
// laptop mode.
type TabletLaptopModeCmds struct {
	SetECTabletModeCmd string
	SetECLaptopModeCmd string
}

// GetECTabletLaptopModeCommand returns TabletLaptopModeCmds, containing the
// ec commands for switching to tablet and laptop mode, if they are supported.
func (h *Helper) GetECTabletLaptopModeCommand(ctx context.Context) (TabletLaptopModeCmds, error) {
	var cmds TabletLaptopModeCmds
	if err := h.RequireRPCUtils(ctx); err != nil {
		return cmds, errors.Wrap(err, "requiring RPC utils")
	}
	testing.ContextLog(ctx, "Checking if DUT is convertible")
	isConvertible, err := func(ctx context.Context) (bool, error) {
		// Run 'cros_config /hardware-properties is-lid-convertible' remotely to
		// check if DUT is a convertible device.
		checkConvertibleRequest := fwpb.CheckCrosConfigRequest{
			CrosConfigPath:     "/hardware-properties",
			CrosConfigProperty: "is-lid-convertible",
		}
		checkConvertible, err := h.RPCUtils.CheckCrosConfigProperty(ctx, &checkConvertibleRequest)
		if err != nil {
			return false, err
		}
		if checkConvertible.CrosConfigPropertyValue != "" {
			out, err := strconv.ParseBool(checkConvertible.CrosConfigPropertyValue)
			if err != nil {
				return false, errors.Wrapf(err, "failed to convert %s to bool", checkConvertible.CrosConfigPropertyValue)
			}
			return out, nil
		}
		// Just in case 'cros_config /hardware-properties is-lid-convertible'
		// doesn't exist, for example, on zork/gumboz, reconfirm with the
		// form-factor flag.
		checkFormFactorRequest := fwpb.CheckCrosConfigRequest{
			CrosConfigPath:     "/hardware-properties",
			CrosConfigProperty: "form-factor",
		}
		formFactor, err := h.RPCUtils.CheckCrosConfigProperty(ctx, &checkFormFactorRequest)
		if err != nil {
			return false, err
		}
		if formFactor.CrosConfigPropertyValue == "CONVERTIBLE" {
			return true, nil
		}
		return false, nil
	}(ctx)
	if err != nil {
		return cmds, errors.Wrap(err, "failed to check if DUT is a convertible")
	}
	testing.ContextLog(ctx, "Checking if DUT is detachable")
	isDetachable, err := func(ctx context.Context) (bool, error) {
		// Run 'cros_config /detachable-base usb-path' remotely to check if DUT is a
		// detachable device.
		checkDetachableRequest := fwpb.CheckCrosConfigRequest{
			CrosConfigPath:     "/detachable-base",
			CrosConfigProperty: "usb-path",
		}
		detachableUSBPath, err := h.RPCUtils.CheckCrosConfigProperty(ctx, &checkDetachableRequest)
		if err != nil {
			return false, err
		}
		if detachableUSBPath.CrosConfigPropertyValue != "" {
			return true, nil
		}
		return false, nil
	}(ctx)
	if err != nil {
		return cmds, errors.Wrap(err, "failed to check if DUT is a detachable")
	}
	// Switching between tablet and laptop mode should only be applicable on
	// convertible and detachable machines.
	if isConvertible {
		cmds.SetECTabletModeCmd = "tabletmode on"
		cmds.SetECLaptopModeCmd = "tabletmode off"
	}
	if isDetachable {
		cmds.SetECTabletModeCmd = "basestate detach"
		cmds.SetECLaptopModeCmd = "basestate attach"
	}
	return cmds, nil
}

// SetDefaultBootDisk sets crossystem dev_default_boot to disk.
func (h *Helper) SetDefaultBootDisk(ctx context.Context) error {
	testing.ContextLog(ctx, "Setting dev_default_boot")
	return h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_default_boot=disk").Run(ssh.DumpLogOnError)
}

// SetDefaultBootUSB sets crossystem dev_default_boot to usb.
func (h *Helper) SetDefaultBootUSB(ctx context.Context) error {
	testing.ContextLog(ctx, "Setting dev_default_boot")
	return h.DUT.Conn().CommandContext(ctx, "crossystem", "dev_default_boot=usb").Run(ssh.DumpLogOnError)
}

// SaveEventLog saves the event logs.
func (h *Helper) SaveEventLog(ctx context.Context, saveLogPath string) error {
	output, err := h.Reporter.GetRawEventLogs(ctx)
	if err != nil {
		return err
	}
	if err := os.WriteFile(saveLogPath, []byte(output), 0666); err != nil {
		return errors.Wrapf(err, "failed to write firmware log to %s", saveLogPath)
	}
	return nil
}

// GetECConsoleOutputWithComment returns EC console output with a comment message added.
func (h *Helper) GetECConsoleOutputWithComment(ctx context.Context, comment string) (string, error) {
	// Send invalid command to EC just so the message appears in the console output.
	if err := h.Servo.RunECCommand(ctx, "comment "+comment); err != nil {
		return "", errors.Wrap(err, "failed to send ec comment")
	}
	var output strings.Builder
	keepPollingErr := errors.New("keep polling")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := h.Servo.GetQuotedString(ctx, servo.ECUARTStream)
		output.WriteString(out)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to read EC output"))
		}
		return keepPollingErr
	}, &testing.PollOptions{Interval: 100 * time.Millisecond, Timeout: 500 * time.Millisecond}); !errors.Is(err, keepPollingErr) {
		return "", errors.Wrap(err, "failed to read EC output")
	}
	return output.String(), nil
}

// SetMiniOSPriority sets expected MiniOS priority.
func (h *Helper) SetMiniOSPriority(ctx context.Context, expectedPriority string) error {
	return h.Reporter.CrossystemSetParam(ctx, reporters.CrossystemParamMiniOSPriority, expectedPriority)
}

// checkHasBatteryWithServoTypeC checks whether the DUT has a battery,
// and whether the connected servo is a servo type-C. One scenario where
// this method can be called is before disconnecting the servo charger to
// make booting the USB more stable.
func (h *Helper) checkHasBatteryWithServoTypeC(ctx context.Context) (bool, error) {
	batteryExists, err := h.CheckBatteryAvailable(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to check if battery is available")
	}
	supportPDRole, err := h.Servo.IsServoTypeC(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to check the servo connection type")
	}
	return batteryExists && supportPDRole, nil
}

// CheckAndSetServoCharger contains the current servo charger state and
// the state whether the charger can be removed safely before booting from USB.
type CheckAndSetServoCharger struct {
	IsServoChargerConnected    bool
	RemoveServoChargerRequired bool
}

// CheckServoChargerBeforeBootingFromUSB returns current charger state and
// whether the charger can be removed safely before booting from USB.
func (h *Helper) CheckServoChargerBeforeBootingFromUSB(ctx context.Context) CheckAndSetServoCharger {
	ok, err := h.checkHasBatteryWithServoTypeC(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Unable to determine battery and servo connection type info: ", err)
	}
	attached, err := h.Servo.GetChargerAttached(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to check charger status")
	}
	return CheckAndSetServoCharger{IsServoChargerConnected: attached, RemoveServoChargerRequired: ok && attached}
}

// LaunchMiniOS sets minios_priority if needed, and launches minios.
func (h *Helper) LaunchMiniOS(ctx context.Context, kbShortcutBoot, miniOSOld bool) error {
	ms, err := NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create mode switcher")
	}
	if err := ms.EnableRecMode(ctx, servo.PowerStateRec, servo.USBMuxOff); err != nil {
		return errors.Wrap(err, "failed to enable recovery mode")
	}
	if kbShortcutBoot {
		newbp, err := NewBypasser(ctx, h)
		if err != nil {
			return errors.Wrap(err, "failed to create a new bypasser")
		}
		if err := newbp.TriggerRecToMiniOS(ctx); err != nil {
			return errors.Wrap(err, "failed to boot to MiniOS")
		}
	} else {
		menuOperator, err := NewMenuOperator(ctx, h)
		if err != nil {
			return errors.Wrap(err, "failed to create a new menu operator")
		}
		if err := menuOperator.TriggerRecToMiniOS(ctx, miniOSOld); err != nil {
			return errors.Wrap(err, "failed to boot to MiniOS")
		}
	}
	return nil
}

// ReadTPMC executes a tpmc command using provided arguments and returns the output.
// It also stops the TPM daemon and restarts it after executing the command.
func (h *Helper) ReadTPMC(ctx context.Context, tpmReadArgs ...string) (out string, reterr error) {
	cleanupCtx := ctx
	// Reserve a longer time to ensure enough time to restore the TPM daemon.
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	if err := h.RequireTPMServiceClient(ctx); err != nil {
		return "", errors.Wrap(err, "failed to create TPM service client")
	}

	if _, err := h.TPMServiceClient.NewHelper(ctx, &empty.Empty{}); err != nil {
		return "", errors.Wrap(err, "failed to create a TPM service helper")
	}
	defer func(ctx context.Context) {
		if _, err := h.TPMServiceClient.CloseHelper(ctx, &empty.Empty{}); err != nil {
			if reterr != nil {
				testing.ContextLog(ctx, "Failed to close TPM service helper")
			} else {
				reterr = errors.Wrap(err, "failed to close TPM service helper")
			}
		}
	}(cleanupCtx)

	if _, err := h.TPMServiceClient.StopDaemons(ctx, &empty.Empty{}); err != nil {
		return "", errors.Wrap(err, "failed to stop TPM daemons")
	}
	defer func(ctx context.Context) {
		if _, err := h.TPMServiceClient.StartDaemons(ctx, &empty.Empty{}); err != nil {
			if reterr != nil {
				testing.ContextLog(ctx, "Failed to restart TPM daemons")
			} else {
				reterr = errors.Wrap(err, "failed to restart TPM daemons")
			}
		}
	}(cleanupCtx)

	testing.ContextLog(ctx, "Read the specific space from the TPM with arguments ", tpmReadArgs)
	cmd := h.DUT.Conn().CommandContext(ctx, "tpmc", tpmReadArgs...)
	result, err := cmd.Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to read the specific space")
	}

	testing.ContextLogf(ctx, "TPMC OUTPUT: %s", result)

	return string(result), nil
}

// DeveloperUSBBoot checks if removing servo charger is required and performs a developer usb boot.
func (h *Helper) DeveloperUSBBoot(ctx context.Context, state *CheckAndSetServoCharger) (retErr error) {
	if err := h.CloseRPCConnection(ctx); err != nil {
		return errors.Wrap(err, "failed to close rpc connection")
	}
	ms, err := NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create mode switcher")
	}
	if h.HasAPFwState {
		closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
		if err != nil {
			return errors.Wrap(err, "failed to enable capture EC UART")
		}
		defer func() { retErr = errors.Join(retErr, closeUART(ctx)) }()
	}
	testing.ContextLog(ctx, "Removing the USB to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxOff); err != nil {
		return errors.Wrap(err, "failed to remove USB")
	}
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		return errors.Wrap(err, "failed to cold reset the DUT")
	}
	if h.HasAPFwState {
		if err := h.DetectFirmwareScreen(ctx, h.Config.FirmwareScreen, fwCommon.DeveloperMode); err != nil {
			return errors.Wrap(err, "failed to detect firmware screen")
		}
	} else {
		testing.ContextLog(ctx, "Waiting for DUT to reach the firmware screen before bypassing dev")
		if err := h.WaitFirmwareScreen(ctx, h.Config.FirmwareScreen); err != nil {
			return errors.Wrap(err, "failed to get to firmware screen")
		}
	}

	testing.ContextLog(ctx, "Resetting firmware screen timeout")
	if err := h.Servo.PressKey(ctx, " ", servo.DurTab); err != nil {
		return errors.Wrap(err, "failed to press space key to reset firmware screen timeout")
	}
	if state.RemoveServoChargerRequired {
		if err := h.SetDUTPower(ctx, false); err != nil {
			return errors.Wrap(err, "failed to remove charger")
		}
		state.IsServoChargerConnected = false
		// GoBigSleepLint: Wait for a while between removing the charger and
		// booting the DUT from USB to prevent USB disconnected issues.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
	}
	testing.ContextLog(ctx, "Setting DFP mode")
	if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
		testing.ContextLogf(ctx, "Failed to set pd data role to DFP: %.400s", err)
	}
	// On KeyboardDevSwitcher machines, pressing space triggers the
	// to_norm screen. Revert to the developer screen with the
	// esc key.
	if h.Config.ModeSwitcherType == KeyboardDevSwitcher {
		testing.ContextLog(ctx, "Returning back to developer screen")
		if err := h.Servo.PressKey(ctx, "<esc>", servo.DurTab); err != nil {
			return errors.Wrap(err, "failed to press esc")
		}
		// GoBigSleepLint: Sleep for model specific time.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			return errors.Wrapf(err, "failed to sleep for %s (KeypressDelay)", h.Config.KeypressDelay)
		}
	}
	testing.ContextLog(ctx, "Inserting the USB to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		return errors.Wrap(err, "failed to insert USB to DUT")
	}
	params := RunBypasser{BypasserMethod: ms.BypassDevBootUSB, RepeatBypasser: true, WaitUntilDUTConnected: h.Config.USBImageBootTimeout}
	if err := ms.RunBypasserUntilDUTConnected(ctx, params); err != nil {
		return errors.Wrap(err, "failed to transition from fw screen to usb boot")
	}
	testing.ContextLog(ctx, "Expecting that DUT booted from USB")
	bootedFromRemovableDevice, err := h.Reporter.BootedFromRemovableDevice(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to determine boot device type")
	}
	if !bootedFromRemovableDevice {
		return errors.New("dut did not boot from removable device")
	}
	return nil
}

// BootToRecoveryMode checks if removing servo charger is required and performs a recovery usb boot.
func (h *Helper) BootToRecoveryMode(ctx context.Context, state *CheckAndSetServoCharger, forceMRCBoot bool) error {
	ms, err := NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create mode switcher")
	}
	recType := servo.PowerStateRec
	if forceMRCBoot {
		recType = servo.PowerStateRecForceMRC
	}
	if err := ms.EnableRecMode(ctx, recType, servo.USBMuxOff); err != nil {
		return errors.Wrap(err, "failed to reboot the DUT into the recovery screen")
	}
	if state.RemoveServoChargerRequired {
		if err := h.SetDUTPower(ctx, false); err != nil {
			return errors.Wrap(err, "failed to remove charger")
		}
		state.IsServoChargerConnected = false
		// GoBigSleepLint: Wait for a while between removing the charger and
		// booting the DUT from USB to prevent USB disconnected issues.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
	}
	testing.ContextLog(ctx, "Setting DFP mode")
	if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
		testing.ContextLogf(ctx, "Failed to set pd data role to DFP: %.400s", err)
	}
	testing.ContextLog(ctx, "Inserting the USB to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		return errors.Wrap(err, "failed to insert USB to DUT")
	}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.USBImageBootTimeout)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, ResetEthernetDongle); err != nil {
		return errors.Wrap(err, "failed to reconnect to the DUT")
	}
	testing.ContextLog(ctx, "Checking that DUT has booted from a removable device")
	fromRemovableDevice, err := h.Reporter.BootedFromRemovableDevice(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to check for the boot device type")
	}
	if !fromRemovableDevice {
		return errors.New("DUT did not boot from a removable device")
	}
	return nil
}

// RebootWithSSHCommand sends a reboot command via SSH to reboot the DUT.
func (h *Helper) RebootWithSSHCommand(ctx context.Context, fromMode fwCommon.BootMode) (retErr error) {
	defer func() {
		if retErr != nil {
			currPowerState, getPowerStateErr := h.Servo.GetECSystemPowerState(ctx)
			if getPowerStateErr != nil {
				retErr = errors.Join(retErr, errors.Wrap(getPowerStateErr, "failed to get EC power state"))
			}
			retErr = errors.Join(retErr, errors.Errorf("got %v power state", currPowerState))
		}
	}()
	if err := h.CloseRPCConnection(ctx); err != nil {
		return errors.Wrap(err, "failed to close rpc connection")
	}
	testing.ContextLog(ctx, "Rebooting the DUT")
	if err := h.DUT.Conn().CommandContext(ctx, "reboot").Run(); err != nil && !errors.As(err, &context.DeadlineExceeded) {
		return errors.Wrap(err, "failed to run reboot command")
	}
	waitDisconnectCtx, cancelWaitDisconnect := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelWaitDisconnect()
	if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
		return errors.Wrap(err, "failed to wait for DUT to become unreachable")
	}
	reconnectTimeout := h.Config.DelayRebootToPing
	if fromMode == fwCommon.BootModeDev {
		reconnectTimeout = h.Config.FirmwareScreen + DevScreenTimeout + h.Config.DelayRebootToPing
	}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, reconnectTimeout)
	defer cancelWaitConnect()
	if err := h.WaitConnect(waitConnectCtx, ResetEthernetDongle); err != nil {
		return errors.Wrap(err, "failed to reconnect to the DUT")
	}
	return nil
}

// KeyVersOptions defines the options for retrieving RWA's or RWB's key version information.
type KeyVersOptions struct {
	Section bios.ImageSection
	Type    KeyType
}

// KeyType represents the type of key version.
type KeyType int

const (
	// FwDataKey indicates the firmware data key version.
	FwDataKey KeyType = iota
	// KernelSubkey indicates the kernel subkey version.
	KernelSubkey
	// FWVersion indicates the firmware version.
	FWVersion
)

// GetCurrentKeyVersion retrieves the RWA's or RWB's key version.
func (h *Helper) GetCurrentKeyVersion(ctx context.Context, opts KeyVersOptions) (uint16, error) {
	if err := h.RequireRPCClient(ctx); err != nil {
		return 0, errors.Wrap(err, "failed to require RPC client")
	}

	fs := dutfs.NewClient(h.RPCClient.Conn)
	tempDir, err := fs.TempDir(ctx, "", "")
	if err != nil {
		return 0, errors.Wrap(err, "failed to create tempDir")
	}
	defer fs.RemoveAll(ctx, tempDir)

	biosBin := filepath.Join(tempDir, "bios.bin")

	futilityInstance, err := futility.NewLocalBuilder(h.DUT).Build()
	if err != nil {
		return 0, errors.Wrap(err, "failed to create futility instance")
	}

	readOpts := futility.NewReadAPOptions(biosBin)
	if log, err := futilityInstance.ReadAP(ctx, readOpts); err != nil {
		return 0, errors.Errorf("failed to read AP firmware: %v, got futility log: %s", err, string(log))
	}

	showOpts := futility.NewShowOptions(biosBin)
	out, err := futilityInstance.Show(ctx, showOpts)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to use futility to show %v, got futility log: %s", biosBin, string(out))
	}

	var versionPattern string
	switch opts.Type {
	case FwDataKey:
		versionPattern = fmt.Sprintf("bios::%v::keyblock::data_key::version::", opts.Section)
	case KernelSubkey:
		versionPattern = fmt.Sprintf("bios::%v::preamble::kernel_subkey::version::", opts.Section)
	case FWVersion:
		versionPattern = fmt.Sprintf("bios::%v::preamble::firmware_version::", opts.Section)
	default:
		return 0, errors.New("invalid key version type")
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, versionPattern) {
			parts := strings.Split(line, "::")
			versionTmp, err := strconv.ParseUint(parts[len(parts)-1], 10, 16)
			if err != nil {
				return 0, errors.Wrapf(err, "failed to parse string %v to uint64", parts[len(parts)-1])
			}
			version := uint16(versionTmp)
			testing.ContextLogf(ctx, "The current key version of section %s is %d", opts.Section, version)
			return version, nil
		}
	}
	return 0, errors.Errorf("failed to find the key version pattern %q. The output of 'futility show': %s", versionPattern, out)
}

// MakeKeysOption defines the options used for generating keys using a predefined shell script.
type MakeKeysOption struct {
	VersionToSign  string
	KeysDir        string
	MakeKeyFileDir string
	CommonFileDir  string
	ShellScript    map[string]string
}

// PrepareKeysWithScript uses the script to prepare the key files with the new key version.
func (h *Helper) PrepareKeysWithScript(ctx context.Context, opts MakeKeysOption) error {
	workDir, err := os.MkdirTemp("", "work-dir-*")
	if err != nil {
		return errors.Wrap(err, "failed to create temporary directory")
	}
	defer os.RemoveAll(workDir)

	fs := dutfs.NewClient(h.RPCClient.Conn)
	if err := fs.RemoveAll(ctx, opts.KeysDir); err != nil {
		return errors.Wrapf(err, "failed to remove the dir %v", opts.KeysDir)
	}
	if err := fs.CopyDir(ctx, "/usr/share/vboot/devkeys", opts.KeysDir); err != nil {
		return errors.Wrapf(err, "failed to copy the key files from %v to %v", "/usr/share/vboot/devkeys", opts.KeysDir)
	}

	if opts.MakeKeyFileDir != "" && opts.CommonFileDir != "" {
		fileMap := map[string]string{
			opts.ShellScript[opts.MakeKeyFileDir]: filepath.Join(workDir, opts.MakeKeyFileDir),
			opts.ShellScript[opts.CommonFileDir]:  filepath.Join(workDir, opts.CommonFileDir),
		}
		for key := range fileMap {
			if _, err := os.Stat(key); os.IsNotExist(err) {
				delete(fileMap, key)
			}
		}
		if _, err := linuxssh.PutFiles(ctx, h.DUT.Conn(), fileMap, linuxssh.DereferenceSymlinks); err != nil {
			return errors.Wrap(err, "failed syncing Tast files from test server onto DUT")
		}
		testing.ContextLog(ctx, "Generating the keys by executing make_keys.sh")
		if err := h.DUT.Conn().CommandContext(ctx, "/bin/bash", filepath.Join(workDir, opts.MakeKeyFileDir), fmt.Sprint(opts.VersionToSign)).Run(ssh.DumpLogOnError); err != nil {
			return errors.Wrapf(err, "failed to execute %v on DUT", filepath.Join(workDir, opts.MakeKeyFileDir))
		}
	}
	return nil
}

// isHWWPEnabled checks the hardware write protect state via servo.
func isHWWPEnabled(ctx context.Context, h *Helper) (bool, error) {
	state, err := h.Servo.GetString(ctx, servo.FWWPState)
	if err != nil {
		return false, errors.Wrap(err, "failed to get write protect state")
	}
	switch servo.FWWPStateValue(state) {
	case servo.FWWPStateOn, servo.FWWPStateForceOn:
		return true, nil
	case servo.FWWPStateOff, servo.FWWPStateForceOff:
		return false, nil
	default:
		return false, errors.New("invalid FW WP state: " + state)
	}
}

// SetECWriteProtect sets the EC's write protection and reboots the DUT with a cold reset.
func (h *Helper) SetECWriteProtect(ctx context.Context, enable bool) error {
	ms, err := NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "creating mode switcher")
	}

	/*
		Some ITE ECs can only clear their WP status on a power-on reset,
		so changing HW WP requires hard ec reboot to take effect, possible cases:
		init HW | init SW | desired | Steps
		    0   |    0    |    0    |  Return
		    0   |    0    |    1    |  SW on -> HW on -> Reboot -> Return
		    0   |    1    |    0    |  SW off -> Return
		    0   |    1    |    1    |  HW on -> Reboot -> Return
		    1   |    0    |    0    |  HW off -> Reboot -> Return
		    1   |    0    |    1    |  HW off -> Reboot -> SW on -> HW on -> Reboot -> Return
		    1   |    1    |    0    |  HW off -> Reboot -> SW off -> Return
		    1   |    1    |    1    |  Return
		Case 6 would require 2 reboots here, but since hw and sw wp should always match,
		and both should be disabled at test start/end, we assume they always correspond.
		Then, we only need to consider cases 1, 2, 7, 8.
	*/
	initialState, err := isHWWPEnabled(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to get initial write protect state")
	}
	if initialState == enable {
		testing.ContextLogf(ctx, "WP State already %v, no action required", initialState)
		return nil
	} else if enable { // Need to enable WP from disabled state.
		// Enable SW WP before hardware WP.
		testing.ContextLog(ctx, "Setting sw wp to enable")
		if err := h.Servo.RunECCommand(ctx, "flashwp enable"); err != nil {
			return errors.Wrap(err, "failed to set flashwp enable")
		}
		if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateForceOn); err != nil {
			return errors.Wrap(err, "failed to enable hw write protect")
		}
		testing.ContextLog(ctx, "Rebooting the DUT")
		if err := ms.ModeAwareReboot(ctx, ColdReset, AllowGBBForce); err != nil {
			return errors.Wrap(err, "failed to perform mode aware reboot")
		}
	} else { // Need to disable WP from enabled state.
		if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateForceOff); err != nil {
			return errors.Wrap(err, "failed to disable hw write protect")
		}
		// Reboot after deasserting hardware write protect pin to deactivate
		// write protect. And then remove software write protect flag.
		testing.ContextLog(ctx, "Rebooting the DUT")
		if err := ms.ModeAwareReboot(ctx, ColdReset, AllowGBBForce); err != nil {
			return errors.Wrap(err, "failed to perform mode aware reboot")
		}
		// Disable SW WP after hardware WP.
		testing.ContextLog(ctx, "Setting sw wp to disable")
		if err := h.Servo.RunECCommand(ctx, "flashwp disable"); err != nil {
			return errors.Wrap(err, "failed to set flashwp disable")
		}
	}

	currState, err := isHWWPEnabled(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to get new write protect state")
	}

	if currState != enable {
		return errors.Errorf("fw wp state after reboot was %v, want %v", currState, enable)
	}
	testing.ContextLog(ctx, "FW write protect state has been successfully set to ", currState)
	return nil
}

// ByPassDevBootTimeout resets the timeout on the developer screen.
func (h *Helper) ByPassDevBootTimeout(ctx context.Context) error {
	if h.Config.ModeSwitcherType == TabletDetachableSwitcher {
		testing.ContextLog(ctx, "Pressing tab to restart fw screen timeout")
		if err := h.Servo.PressKey(ctx, "<tab>", servo.DurTab); err != nil {
			return errors.Wrap(err, "failed to press tab key")
		}
	} else {
		testing.ContextLog(ctx, "Pressing space to restart fw screen timeout")
		if err := h.Servo.PressKey(ctx, " ", servo.DurTab); err != nil {
			return errors.Wrap(err, "failed to press space key")
		}
	}
	// GoBigSleepLint: Simulate a specific speed of key presses.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to sleep for %v second", h.Config.KeypressDelay)
	}
	return nil
}

// ReturnToDeveloperScreen switch the DUT back to the developer screen.
func (h *Helper) ReturnToDeveloperScreen(ctx context.Context) error {
	if h.Config.ModeSwitcherType == MenuSwitcher {
		return nil
	}
	testing.ContextLog(ctx, "Pressing esc to return to the developer screen")
	if err := h.Servo.PressKey(ctx, "<esc>", servo.DurTab); err != nil {
		return errors.Wrap(err, "failed to press esc key")
	}

	if h.HasAPFwState {
		if err := h.DetectFirmwareScreen(ctx, h.Config.FirmwareScreen, fwCommon.DeveloperToNorm); err != nil {
			return errors.Wrap(err, "failed to detect firmware screen")
		}
	} else {
		// GoBigSleepLint: Simulate a specific speed of key presses.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			return errors.Wrapf(err, "failed to sleep for %v second", h.Config.KeypressDelay)
		}
	}

	return nil
}

// GetAllECCrashFiles finds all files in /var/spool/crash that start with embedded_controller*.
// Since there are multiple files with the same name/different extensions,
// it maps the base name (without ext) to all filepaths with that base name.
func (h *Helper) GetAllECCrashFiles(ctx context.Context) (map[string][]string, error) {
	if h.DUT == nil {
		return nil, nil
	}
	out, err := h.DUT.Conn().CommandContext(ctx, "ls", "-1t", ECCrashBaseDir).Output(ssh.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %s dir on DUT", ECCrashBaseDir)
	}
	files := strings.Split(string(out), "\n")

	var fileMap = make(map[string][]string)
	for _, f := range files {
		file := strings.TrimSpace(f)
		if strings.HasPrefix(file, "embedded_controller") {
			fileWithoutExt := strings.TrimSuffix(file, filepath.Ext(file))
			filePath := ECCrashBaseDir + file
			fileMap[fileWithoutExt] = append(fileMap[fileWithoutExt], filePath)
		}
	}

	return fileMap, nil
}

// UpdateECCrashCache updates the ecCrashFileCache to include all the current existing ec crash files on the DUT.
func (h *Helper) UpdateECCrashCache(ctx context.Context) error {
	crashFiles, err := h.GetAllECCrashFiles(ctx)
	if err != nil {
		return err
	}

	if h.ecCrashFileCache == nil {
		h.ecCrashFileCache = make(map[string]bool)
	}
	for file := range crashFiles {
		h.ecCrashFileCache[file] = true
	}
	return nil
}

// GetNewECCrashes gets the most recent ec crashes in /var/spool/crash on the DUT that aren't in the cache.
// It returns a map of the crash and list of files associated with that crash.
func (h *Helper) GetNewECCrashes(ctx context.Context) (map[string][]string, error) {
	crashFiles, err := h.GetAllECCrashFiles(ctx)
	if err != nil {
		return nil, err
	}

	// Remove cached files only keep new ones.
	for file := range h.ecCrashFileCache {
		delete(crashFiles, file)
	}

	return crashFiles, err
}

// ErrorHandler is normally testing.FixtTestState or testing.TestState
type ErrorHandler interface {
	Error(...interface{})
	Fatal(...interface{})
	TestName() string
	OutDir() string
}

// SaveECCrash saves one crash to the test out dir and Errors the test
func (h *Helper) SaveECCrash(ctx context.Context, crashName string, listOfCrashFiles []string, s ErrorHandler) {
	logFilePath := ECCrashBaseDir + crashName + ".eccrash"
	out, err := h.Reporter.CatFile(ctx, logFilePath)
	if err != nil {
		msg := fmt.Sprintf("failed to read .eccrash file %s to print in log", logFilePath)
		testing.ContextLog(ctx, logECCrash(msg, s, err))
	}
	crashLog := string(out)
	if ok, err := checkECCrashType(crashLog, "watchdogWarning"); err != nil {
		s.Error(logECCrash("failed to parse ec crash log", s, err))
	} else if ok {
		testing.ContextLog(ctx, logECCrash("found watchdog warning (dead6668), not logging crash", s))
		return
	} // Otherwise, crash is important, save logs and raise error.

	crashSaveDir := filepath.Join(s.OutDir(), crashName)
	if err := os.MkdirAll(crashSaveDir, os.ModePerm); err != nil {
		s.Fatal(logECCrash("found crashes but failed to create dir to save crash logs", s, err))
	}
	for _, f := range listOfCrashFiles {
		saveFilePath := filepath.Join(crashSaveDir, strings.TrimPrefix(f, ECCrashBaseDir))
		if err := linuxssh.GetFile(ctx, h.DUT.Conn(), f, saveFilePath, linuxssh.DereferenceSymlinks); err != nil {
			msg := fmt.Sprintf("failed to save log for file %s", f)
			s.Error(logECCrash(msg, s))
		}
	}
	// The .eccrash file is of predictable length (and not too long), just print it out.
	msg := fmt.Sprintf("found unexpected EC Crash, saved ec crash log to %s: %s)", crashSaveDir, crashLog)
	s.Error(logECCrash(msg, s))

}

// logECCrash returns a log string with a searchable prefix to find EC crash check issues in testhaus.
// Allows optional error(s) to be passed in.
func logECCrash(msg string, s ErrorHandler, errs ...error) string {
	res := fmt.Sprintf("EC crash error in %s: %s", s.TestName(), msg)
	for _, err := range errs {
		res = fmt.Sprintf("%s, with error: %v", res, err)
	}
	return res
}

func checkECCrashType(crashLog string, crashIDs ...string) (bool, error) {
	var ecCrashID = map[string]string{
		"divBy0":          "dead6660",
		"stackOverflow":   "dead6661",
		"pdCrash":         "dead6662",
		"assert":          "dead6663",
		"watchdog":        "dead6664",
		"badRng":          "dead6665",
		"pmicFault":       "dead6666",
		"exit":            "dead6667",
		"watchdogWarning": "dead6668",
	}

	for _, crashID := range crashIDs {
		crashStr, ok := ecCrashID[crashID]
		if !ok {
			return false, errors.Errorf("crashID %s not recognized as crash ID in ecCrashID map", crashID)
		}
		if strings.Contains(crashLog, crashStr) {
			return true, nil
		}
	}
	return false, nil
}

// SupportAPFwState checks whether DUT supports the host command EC_CMD_AP_FW_STATE. Sets h.HasAPFwState to true if supported.
func (h *Helper) SupportAPFwState(ctx context.Context, dutFeatures *protocol.DUTFeatures) error {
	if h.HasAPFwState {
		return nil
	}

	var roMajorVersion int
	if dutFeatures != nil {
		roMajorVersion = int(dutFeatures.GetHardware().GetHardwareFeatures().GetFwConfig().GetFwRoVersion().GetMajorVersion())
	}
	if roMajorVersion <= 0 && h.Reporter != nil {
		roVersion, err := h.Reporter.GetFWVersion(ctx, reporters.CrossystemParamRoFwid)
		if err != nil {
			return errors.Wrap(err, "failed to get RO firmware version")
		}
		splitout := strings.Split(roVersion, ".")
		if len(splitout) < 3 {
			return errors.Wrapf(err, "got invalid firmware version: %v", roVersion)
		}
		roMajorVersion, err = strconv.Atoi(splitout[0])
		if err != nil {
			return errors.Wrap(err, "failed to convert firmware major version value to integer value")
		}
	}
	if roMajorVersion <= 0 {
		testing.ContextLog(ctx, "Cannot find AP RO Firmware version. Maybe set -hwdeps='hardware_features<fw_config<fw_ro_version<major_version:12345>>>'")
	}
	// CL:5020949 laned in 15683.0.0
	if roMajorVersion >= 15683 {
		h.HasAPFwState = true
	}
	return nil
}

// GSCResetAfterWPEnable triggers a GSC reboot by shutting down the AP after WP has been enabled.
func (h *Helper) GSCResetAfterWPEnable(ctx context.Context, dutFeatures *protocol.DUTFeatures) error {
	// Don't do anything if the board isn't running a Ti50 image that resets after WP is enabled.
	if !h.Servo.ExpectTi50WPEventReboot(ctx) {
		return nil
	}

	// Ti50 should reboot when it sees the AP shut down.
	if err := h.DUT.Conn().CommandContext(ctx, "shutdown", "-P", "now").Run(); err != nil && !errors.As(err, &context.DeadlineExceeded) {
		return errors.Wrap(err, "failed to run shutdown command")
	}

	h.Servo.WaitForGSCReset(ctx, 10*time.Second)

	switch dutFeatures.GetHardware().GetHardwareFeatures().GetFormFactor().GetFormFactor() {
	case api.HardwareFeatures_FormFactor_CHROMEBOX:
		testing.ContextLog(ctx, "The DUT is probably off for AP_IDLE")
		// GoBigSleepLint: wait 5s for EC_RST released and EC finished init
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
		testing.ContextLog(ctx, "Powering on the DUT by pressing power button for ", h.Config.HoldPwrButtonPowerOn)
		h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur(h.Config.HoldPwrButtonPowerOn))
	}

	waitConnectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancel()
	if err := h.WaitConnect(waitConnectCtx, ResetEthernetDongle); err != nil {
		currPowerState, stateErr := h.Servo.GetECSystemPowerState(ctx)
		if stateErr != nil {
			return errors.Wrap(stateErr, "failed to reconnect to DUT, failed to check powerstate")
		}
		return errors.Wrapf(err, "failed to reconnect to DUT, got power state: %v", currPowerState)
	}
	return nil
}
