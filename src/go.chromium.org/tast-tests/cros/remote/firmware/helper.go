// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast-tests/cros/remote/firmware/rpm"
	fwpb "go.chromium.org/tast-tests/cros/services/cros/firmware"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast/core/lsbrelease"
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
}

// WaitConnectOption includes situations to wait to connect from.
type WaitConnectOption string

const (
	// FromHibernation alerts WaitConnect to skip
	// on setting servo control while DUT is still
	// in the process of waking up from hibernation.
	FromHibernation WaitConnectOption = "hibernation"

	// ResetEthernetDongle resets the ethernet dongle
	// for speed-up in ssh connection.
	ResetEthernetDongle WaitConnectOption = "reset_ethernet"
)

// SetupUSBOption includes options for setting up a USB device.
type SetupUSBOption int

const (
	// DontFlashIfSameMilestone indicates that flashing the usb
	// is not required if it contains the same ChromeOS milestone
	// as that running currently on the DUT.
	DontFlashIfSameMilestone SetupUSBOption = iota
)

// NewHelper creates a new Helper object with info from testing.State.
// For tests that do not use a certain Helper aspect (e.g. RPC or Servo), it is OK to pass null-values (nil or "").
func NewHelper(d *dut.DUT, rpcHint *testing.RPCHint, cfgFilepath, servoHostPort, dutHostname, powerunitHostname, powerunitOutlet, hydraHostname string) *Helper {
	return &Helper{
		cfgFilepath:       cfgFilepath,
		DUT:               d,
		keyFile:           d.KeyFile(),
		keyDir:            d.KeyDir(),
		Reporter:          reporters.New(d),
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
		if err := h.Servo.SetString(ctx, servo.CR50Testlab, servo.Open); err != nil {
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
	testing.ContextLog(ctx, "Opening RPCClient connection")
	var cl *rpc.Client
	const rpcConnectTimeout = 5 * time.Minute
	if err := testing.Poll(ctx, func(innerCtx context.Context) error {
		if !h.DUT.Connected(innerCtx) {
			if err := h.DUT.Connect(innerCtx); err != nil {
				return err
			}
		}
		var err error
		cl, err = rpc.Dial(ctx, h.DUT, h.rpcHint)
		return err
	}, &testing.PollOptions{Timeout: rpcConnectTimeout}); err != nil {
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
	tmpDir, err := ioutil.TempDir("", "tast-host-files-copy")
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
	testing.ContextLog(ctx, "Checking ChromeOS image name on usbkey")
	mountPath := fmt.Sprintf("/media/servo_usb/%d", h.ServoProxy.GetPort())
	// Unmount whatever might be mounted.
	h.ServoProxy.RunCommandQuiet(ctx, true, "umount", "-q", mountPath)

	// ChromeOS kernel is at /dev/sdx2.
	kernelPart := usbdev + "2"
	// ChromeOS root fs is in /dev/sdx3.
	mountSrc := usbdev + "3"
	if err := h.ServoProxy.RunCommand(ctx, true, "mkdir", "-p", mountPath); err != nil {
		return false, errors.Wrapf(err, "mkdir failed at %q", mountPath)
	}
	var lsb map[string]string
	// Failures here are a bad USB image, so don't fail, just write the new image.
	var err error
	err = func() error {
		if output, err := h.ServoProxy.OutputCommand(ctx, true, "dd", fmt.Sprintf("if=%s", kernelPart), "bs=8", "count=1"); err != nil {
			return errors.Wrap(err, "failed to read kernel magic")
		} else if bytes.Compare(output, []byte("CHROMEOS")) != 0 {
			return errors.Errorf("incorrect kernel magic string got %v want %v", output, []byte("CHROMEOS"))
		}
		if err = h.ServoProxy.RunCommand(ctx, true, "mount", "-o", "ro", mountSrc, mountPath); err != nil {
			return errors.Wrapf(err, "failed to mount %q at %q", mountSrc, mountPath)
		}
		defer h.ServoProxy.RunCommand(ctx, true, "umount", mountPath)
		output, err := h.ServoProxy.OutputCommand(ctx, true, "cat", fmt.Sprintf("%s/etc/lsb-release", mountPath))
		if err != nil {
			return errors.Wrap(err, "failed to read lsb-release")
		}
		lsb, err = lsbrelease.Parse(bytes.NewReader(output))
		if err != nil {
			return errors.Wrap(err, "failed to parse lsb-release")
		}
		return nil
	}()
	if err != nil {
		if cloudStorage == nil {
			return false, errors.Wrap(err, "bad USB image, and requested no USB image download")
		}
		testing.ContextLog(ctx, "Bad USB image: ", err)
	}

	releaseBuilderPath := lsb[lsbrelease.BuilderPath]
	dutBuilderPath, err := h.Reporter.BuilderPath(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get DUT builder path")
	}
	if strings.Contains(dutBuilderPath, "-postsubmit") {
		testing.ContextLogf(ctx, "Current build on DUT (%s) is not a release image, using %s from USB stick", dutBuilderPath, releaseBuilderPath)
		if releaseBuilderPath == "" {
			return false, errors.New("did not find release image path on the USB")
		}
		return true, nil
	}

	if !strings.Contains(lsb[lsbrelease.ReleaseTrack], "test") {
		if cloudStorage == nil {
			return false, errors.New("the image on usbkey is not a test image")
		}
		testing.ContextLog(ctx, "The image on usbkey is not a test image")
		releaseBuilderPath = ""
	}

	for _, opt := range opts {
		if opt == DontFlashIfSameMilestone {
			releaseMilestone := lsb[lsbrelease.Milestone]
			dutMilestone, err := h.Reporter.Milestone(ctx)
			if err != nil {
				return false, errors.Wrap(err, "failed to get DUT milestone")
			}
			if releaseMilestone == dutMilestone {
				testing.ContextLog(ctx, "USB image contains the same milestone as the one running on the DUT")
				return true, nil
			}
		}
	}

	if releaseBuilderPath == dutBuilderPath {
		testing.ContextLogf(ctx, "Current build on USB (%s) matches DUT (%s), no need to download", releaseBuilderPath, dutBuilderPath)
		return true, nil
	}

	if cloudStorage == nil {
		testing.ContextLogf(ctx, "User requested no USB image download, using %s even though it differs from DUT %s", releaseBuilderPath, dutBuilderPath)
		return true, nil
	}
	testing.ContextLogf(ctx, "Current build on USB (%s) differs from DUT (%s), proceed with download", releaseBuilderPath, dutBuilderPath)
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

	if valid, err := h.validateUSBImage(ctx, usbdev, cloudStorage, opts...); valid && err == nil {
		return nil
	} else if err != nil {
		return errors.Wrap(err, "failed to validate USB image")
	}

	// Sometimes servod loses the CCD connection while we are flashing the USB drive.
	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		return errors.Wrap(err, "failed to remove ccd watchdog")
	}

	// TODO if needed, recovery images are at .../recovery_image.tar.xz.
	testImageURL := "build-artifact:///chromiumos_test_image.tar.xz"
	// TODO(b/217635723): Revisit later when we have a solution for accessing dev servers on non-DUT machines.
	// It would be nicer if CloudStorage had a method ExtractURL(ctx, gsPath, filename) that returned the extract url.
	dataURL, err := cloudStorage.Stage(ctx, testImageURL)
	if err != nil {
		return errors.Wrapf(err, "failed to download test image from %s", testImageURL)
	}
	if dataURL.Scheme != "http" && dataURL.Scheme != "https" {
		return errors.Errorf("CloudStorage url is not http(s): %q", dataURL)
	}
	gsBucket := dataURL.Query().Get("gs_bucket")
	if gsBucket != "" {
		dataURL.Path = strings.Replace(dataURL.Path, "/static/", fmt.Sprintf("/extract/%s/", gsBucket), 1)
		query := dataURL.Query()
		query.Del("gs_bucket")
		query.Set("file", "chromiumos_test_image.bin")
		dataURL.RawQuery = query.Encode()
	} else {
		dataURL.Path = strings.Replace(dataURL.Path, "chromiumos_test_image.tar.xz", "chromiumos_test_image.bin", 1)
	}

	testing.ContextLog(ctx, "Cleaning usb before flashing a new test OS image")
	if err := h.FormatUSB(ctx, usbdev); err != nil {
		return errors.Wrap(err, "failed to format the usb device")
	}
	testing.ContextLogf(ctx, "Flashing test OS image to USB from %q", dataURL.String())

	// If it did have tast files, it won't shortly.
	h.dutUsbHasTastFiles = false

	if err = h.Servo.SetStringTimeout(ctx, servo.DownloadImageToUSBDev, dataURL.String(), 2*time.Hour); err != nil {
		if strings.Contains(string(err.Error()), "Read-only file system") {
			modelName, serialNumber, err := h.getUSBModelAndSerial(ctx, usbdev)
			if err != nil {
				testing.ContextLog(ctx, "Failed to get info about usb: ", err)
			}
			return errors.Errorf("failed to flash os image and found usb device as read-only file system, got usb model: %s, serial number: %s", modelName, serialNumber)
		}
		return errors.Wrapf(err, "failed to flash os image %q to USB %q from url %q", testImageURL, usbdev, dataURL.String())
	}

	// ensure that image was successfully flashed by reading back OS version
	if valid, err := h.validateUSBImage(ctx, usbdev, cloudStorage, opts...); valid && err == nil {
		testing.ContextLogf(ctx, "Successfully flashed %q from %q", usbdev, testImageURL)
		return nil
	}
	return errors.Wrap(err, "failed to validate USB image after flashing")
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
		if !comparePowerStates(currPowerState, powerStates...) {
			return errors.Errorf("Power state = %s", currPowerState)
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
		if currState == state {
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
	testing.ContextLogf(ctx, "Waiting for %s to connect", h.DUT.HostName())
	dfpFailures := 0
	for {
		// SetDUTPDDataRole would fail when DUT is still in the process
		// of waking up from hibernation.
		if !wcOptsContain(opts, FromHibernation) {
			if err := h.Servo.SetDUTPDDataRole(ctx, servo.DFP); err != nil {
				testing.ContextLogf(ctx, "Failed to set pd data role to DFP: %.400s", err)
				dfpFailures++
				if dfpFailures == 3 {
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
			break
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
	// Try servo SetPDRole (servo v4 type C). The servo is slightly evil though, and will report that it has this control even for Type-A.
	connectionType := ""
	hasControl, err := h.Servo.HasControl(ctx, string(servo.PDRole))
	if err != nil {
		return errors.Wrap(err, "checking for control")
	}
	if hasControl {
		connectionType, err = h.Servo.GetString(ctx, servo.DUTConnectionType)
		if err != nil {
			return errors.Wrap(err, "getting connection type")
		}
	}
	if connectionType == "type-c" {
		role := servo.PDRoleSnk
		if powerOn {
			role = servo.PDRoleSrc
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
	return errors.New("servo does not support pd role and no rpm vars provided")
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
			if err := h.Servo.SetString(ctx, servo.CR50Testlab, servo.Open); err != nil {
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
		out, err := h.Servo.RunCR50CommandGetOutput(ctx, "ccd reset factory", []string{`[^>]*> `})
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
	hasTestlab, err := h.Servo.HasControl(ctx, string(servo.CR50Testlab))
	if err != nil {
		return "", errors.Wrap(err, "failed while checking for cr50_testlab control")
	}

	var testlab string
	if hasTestlab {
		testlab, err = h.Servo.GetString(ctx, servo.CR50Testlab)
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
		ccdLevel, err = h.Servo.GetString(ctx, servo.GSCCCDLevel)
		if err != nil {
			testing.ContextLog(ctx, "WARNING! failed to get gsc_ccd_level: ", err)
		}
	}

	if ccdLevel == "" {
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
func (h *Helper) OpenCCDNoTestlab(ctx context.Context) error {
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

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Minute)
	defer cancel()

	// Restore DUT's boot mode to the initial mode if
	// it ends up in a different one after CCD is open.
	defer func(ctx context.Context, initMode fwCommon.BootMode) error {
		currentMode, err := h.Reporter.CurrentBootMode(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to check boot mode after opening CCD")
		}

		if currentMode != initMode {
			testing.ContextLogf(ctx, "Current boot mode %q, switching back to %q", currentMode, initMode)
			if err := ms.RebootToMode(ctx, initMode); err != nil {
				return errors.Wrap(err, "failed to reboot into initial mode")
			}
		}
		return nil
	}(cleanupCtx, initMode)

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
	if err := h.Servo.SetOnOff(ctx, servo.CR50UARTCapture, servo.On); err != nil {
		return errors.Wrap(err, "failed to enable GSC UART capture")
	}
	defer func() {
		if err := h.Servo.SetOnOff(ctx, servo.CR50UARTCapture, servo.Off); err != nil {
			testing.ContextLog(ctx, "Failed to disable GSC UART capture: ", err)
		}
	}()

	// If OpenFromUSB is accessible (i.e. ="Y"), we can send the request through USB.
	// Otherwise, we need to send the request through the AP.
	if openFromUSB == "Y" {
		testing.ContextLog(ctx, "Opening CCD by USB")
		err = h.Servo.RunCR50Command(ctx, "ccd open")
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
				return errors.Wrap(err, "failed to wait for DUT to become unreachable")
			}
		}
	}

	if openNoTPMWipe != "Y" {
		// Wait for DUT to reconnect if OpenNoTPMWipe is not accessible (i.e. !="Y").
		testing.ContextLog(ctx, "Reconnecting to DUT")
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 6*time.Minute)
		defer cancelWaitConnect()

		if err := h.WaitConnect(waitConnectCtx); err != nil {
			checkTpmRequest := func() (int, error) {
				out, err := h.Servo.GetQuotedString(ctx, servo.CR50UARTStream)
				if err != nil {
					return 0, errors.Wrap(err, "failed to read cr50")
				}
				// Deposit cr50 log in the remote context dir.
				outDir, ok := testing.ContextOutDir(ctx)
				if !ok {
					return 0, errors.New("failed to get remote context dir")
				}
				destPath := filepath.Join(outDir, "open_ccd_cr50.log")
				if err := ioutil.WriteFile(destPath, []byte(out), 0666); err != nil {
					return 0, errors.Wrapf(err, "failed to write %s", destPath)
				}
				// Some DUTs were stuck in a loop with multiple requests for tpm reset
				// when they failed to reconnect. Document this scene in the returned
				// error message.
				r := regexp.MustCompile(`tpm_reset_request: already scheduled`)
				matches := r.FindAllStringSubmatch(out, -1)
				if len(matches) == 0 {
					return 0, errors.New("did not find any matches for tpm reset request")
				}
				return len(matches), nil
			}
			if tpmRequestCounts, err := checkTpmRequest(); err != nil {
				testing.ContextLog(ctx, "Error in collecting cr50 info: ", err)
			} else {
				return errors.Wrapf(err, "failed to reconnect to DUT and found tpm reset request scheduled for %d times", tpmRequestCounts)
			}
			return errors.Wrap(err, "failed to reconnect to DUT")
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

// FormatUSB will format the usb device to create an invalid usb device.
// WARNING: Do not use this for tests that verify non-boot on USB, use CorruptUSBKey instead.
func (h *Helper) FormatUSB(ctx context.Context, usbdev string) error {
	if usbdev == "" {
		return errors.New("no USB key detected. Please run CheckUSBOnServoHost")
	}
	// Document usb model and serial numbers for debugging purposes,
	// before formatting the usb.
	modelName, serialNumber, err := h.getUSBModelAndSerial(ctx, usbdev)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get info about usb: ", err)
	}
	testing.ContextLog(ctx, "Formatting the USB device")
	if _, stderr, err := h.ServoProxy.SeparatedOutputCommand(ctx, true, "dd", "if=/dev/zero", fmt.Sprintf("of=%s", usbdev), "bs=1M", "count=16", "conv=fdatasync"); err != nil {
		if strings.Contains(string(stderr), "Read-only file system") {
			return errors.Errorf("found usb device as read-only file system, got usb model: %s, serial number: %s", modelName, serialNumber)
		}
		if err := h.validateUSBConn(ctx); err != nil {
			return errors.Wrap(err, "failed while verifying usb connection to DUT")
		}
		if stderr != nil {
			return errors.Errorf("found stderr %s", stderr)
		}
		return errors.Wrap(err, "failed to format the usb device, but usb connection verified")
	}
	return nil
}

// validateUSBConn checks for usb connection stability by comparing
// the before and after outputs of 'lsusb' on DUT.
func (h *Helper) validateUSBConn(ctx context.Context) error {
	// From Stainless logs, one repetitive error was about not being able to open the USB.
	// When we ssh'ed into their DUTs, it appeared that some USBs would drop connection
	// after a short period of time. Run validateUSBConn to verify if the usb device disappears
	// without any intervention.
	defer func() error {
		// Ensure that the usb is powered on at the end.
		if err := h.Servo.SetString(ctx, "usb3_pwr_en", "on"); err != nil {
			return errors.Wrap(err, "failed to power on USB")
		}
		return nil
	}()
	testing.ContextLog(ctx, "Enabling USB connection to DUT")
	if err := h.Servo.SetUSBMuxState(ctx, servo.USBMuxDUT); err != nil {
		return errors.Wrap(err, "failed to set USBMuxDUT")
	}
	testing.ContextLog(ctx, "Power cycling the USB")
	if err := h.Servo.SetString(ctx, "usb3_pwr_en", "off"); err != nil {
		return errors.Wrap(err, "failed to power off USB")
	}
	if err := h.Servo.SetString(ctx, "usb3_pwr_en", "on"); err != nil {
		return errors.Wrap(err, "failed to power on USB")
	}
	testing.ContextLog(ctx, "Waiting for a short delay")
	// GoBigSleepLint: It takes some time for usb mux state to take effect.
	if err := testing.Sleep(ctx, UsbVisibleTime); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	// The bash commands would attempt steps as follows:
	// 1. Save the lsusb result to a temporary file.
	// 2. Sleep for 2 minutes.
	// 3. Compare the current lsusb output with the previous file.
	testing.ContextLog(ctx, "Attempting to validate USB connection")
	workPath := "/tmp/lsusb.1"
	checkUSBCmd := fmt.Sprintf(
		"lsusb > %[1]s && "+
			"sleep 120s && "+
			"echo $(lsusb | diff - %[1]s -c | grep -h '^[+-][[:blank:]]') && "+
			"rm %[1]s",
		workPath)
	out, err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", checkUSBCmd).Output(ssh.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to check for lsusb")
	}
	deviceDiff := strings.TrimSpace(string(out))
	if strings.Contains(deviceDiff, "+") {
		return errors.Errorf("Device %s disappeared after a short delay", strings.TrimPrefix(deviceDiff, "+ "))
	}
	reUSBDriverName := regexp.MustCompile(`Driver=usb-storage`)
	testing.ContextLog(ctx, "Checking if usb driver really exists")
	driverOutput, err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", "lsusb -t").Output(ssh.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to check for lsusb")
	}
	matches := reUSBDriverName.FindAllStringSubmatch(string(driverOutput), -1)
	if len(matches) == 0 {
		return errors.New("usb device does not exist")
	}
	testing.ContextLogf(ctx, "%d usb device(s) found", len(matches))
	return nil
}

// WaitDUTConnectDuringBootFromUSB will check if the DUT boots from USB or not.
// If expBoot is true, DUT is expected to boot from USB successfully.
// If expBoot is false, DUT is expected to reach a specific firmware screen,
// as documented in the firmware test manual below:
// https://chromium.googlesource.com/chromiumos/docs/+/HEAD/firmware_test_manual.md#firmware-screen-names
func (h *Helper) WaitDUTConnectDuringBootFromUSB(ctx context.Context, expBoot bool) error {
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.USBImageBootTimeout)
	defer cancelWaitConnect()

	err := h.WaitConnect(waitConnectCtx)
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
			if !errors.Is(err, context.DeadlineExceeded) {
				return errors.Wrap(err, "expected dut disconnected")
			}
			return nil
		}
		return errors.Wrapf(err, "expected dut reconnected: %t", expBoot)
	}
}

// CheckBrokenScreen checks if the DUT reaches Broken Screen.
func (h *Helper) CheckBrokenScreen(ctx context.Context) error {
	testing.ContextLog(ctx, "Setting crossystem recovery_request to 1")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := h.DUT.Conn().CommandContext(ctx, "crossystem", "recovery_request=1").Run(); err != nil {
			return errors.Wrap(err, "failed to set crossystem recovery_request")
		}

		recoveryRequest, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamRecoveryRequest)
		if err != nil {
			return errors.Wrap(err, "failed to get crossystem recovery_request")
		} else if recoveryRequest != "1" {
			return errors.Errorf("expected crossystem recovery_request to be 1, got %s", recoveryRequest)
		}
		return nil
	}, &testing.PollOptions{Timeout: 20 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to set the crossystem recovery_request to 1")
	}

	testing.ContextLog(ctx, "Rebooting the DUT with warm reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset); err != nil {
		return errors.Wrap(err, "failed to reboot the DUT with warm reset")
	}
	waitDisconnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWaitConnect()
	if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
		testing.ContextLog(ctx, "DUT is still connected. Attempting the local reboot command on DUT")
		if err := h.DUT.Conn().CommandContext(ctx, "reboot").Run(); err != nil {
			return errors.Wrap(err, "failed to run the reboot cmd")
		}
	}
	testing.ContextLogf(ctx, "Sleeping %s (FirmwareScreen)", h.Config.FirmwareScreen)
	// GoBigSleepLint: Sleeping for model specific time.
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		return errors.Wrapf(err, "sleeping for %s (FirmwareScreen) to wait for firmware screen", h.Config.FirmwareScreen)
	}

	testing.ContextLog(ctx, "Checking if it reaches Broken Screen")
	if err := h.WaitDUTConnectDuringBootFromUSB(ctx, false); err != nil {
		return errors.Wrap(err, "failed to reach Broken Screen")
	}
	return nil
}

// powerSupplyDeviceStates contains status info for ac and battery.
type powerSupplyDeviceStates struct {
	ACOnline      string
	BatteryStatus string
}

// CheckPowerSupplyDeviceStates runs the command 'power_supply_info' and returns
// device status, specifically for ac and battery.
func (h *Helper) CheckPowerSupplyDeviceStates(ctx context.Context) (*powerSupplyDeviceStates, error) {
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
			return "", errors.Errorf("failed to match regex %q in %q", expMatch, string(out))
		}
		return matches[1], nil
	}
	var err error
	var data powerSupplyDeviceStates
	if data.ACOnline, err = checkState(devices["ac"]); err != nil {
		return nil, err
	}
	if data.BatteryStatus, err = checkState(devices["battery"]); err != nil {
		return nil, err
	}
	return &data, nil
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
func (h *Helper) SaveCBMEMLogs(ctx context.Context, saveLogPath string) error {
	out, err := h.Reporter.GetCBMEMLogs(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to run cbmem command")
	}

	if err := ioutil.WriteFile(saveLogPath, []byte(out), 0666); err != nil {
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
