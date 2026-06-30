// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/mod/semver"

	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	fwpb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: BaseECUpdate,
		Desc: "Check that detachable base notification appears upon firmware update",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_ec", "firmware_stressed", "firmware_meets_kpi", "firmware_ec_ro", "firmware_ec_rw"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.FormFactor(hwdep.Detachable), hwdep.Keyboard()),
		Fixture:      fixture.DevModeGBB,
		Timeout:      15 * time.Minute,
	})
}

// baseECInfo contains information about base ec.
type baseECInfo struct {
	name        string
	version     string
	roProtected bool
}

// modifiedFileDir contains paths defined as follows,
// modifiedBin: path on DUT, where the modified base ec bin file will be copied to.
// versionFile: path on DUT that stores the modified firmware version string
type modifiedFileDir struct {
	modifiedBin string
	versionFile string
}

// hammerRequiredVariables contains the required values for hammer command.
type hammerRequiredVariables struct {
	pid     string
	vid     string
	usbPath string
	i2cPath string
}

type baseStateSetter interface {
	SetBaseState(ctx context.Context, state firmware.ECToolBaseState) error
}

type gpioBaseStateSetter struct {
	name   string
	ecTool *firmware.ECTool
}

func (g *gpioBaseStateSetter) SetBaseState(ctx context.Context, state firmware.ECToolBaseState) error {
	cmdList := []string{"gpioset", g.name}

	switch state {
	case firmware.BaseAttach, firmware.BaseAuto:
		cmdList = append(cmdList, "1")
	case firmware.BaseDetach:
		cmdList = append(cmdList, "0")
	default:
		return errors.Errorf("unsupported state: %s", state)
	}

	return g.ecTool.Command(ctx, cmdList...).Run(testexec.DumpLogOnError)
}

func BaseECUpdate(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	if err := h.RequireRPCUtils(ctx); err != nil {
		s.Fatal("Requiring RPC utils: ", err)
	}

	s.Log("Logging in to Chrome")
	if _, err := h.RPCUtils.NewChrome(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to create a new instance of Chrome: ", err)
	}

	dut := s.DUT()
	ecTool := firmware.NewECTool(dut, firmware.ECToolNameMain)
	utilServiceClient := fwpb.NewUtilsServiceClient(h.RPCClient.Conn)

	hammerConfigs, err := getHammerConfig(ctx, h, utilServiceClient)
	if err != nil {
		s.Fatal("Failed to get hammer config: ", err)
	}

	dutfsClient := dutfs.NewClient(h.RPCClient.Conn)
	tempDir, err := dutfsClient.TempDir(ctx, "", "BaseECUpdate")
	if err != nil {
		s.Fatal("Failed to create a temp dir on DUT")
	}

	// fileDir creates paths to save the modified base ec bin file at respective locations.
	fileDir := modifiedFileDir{
		modifiedBin: filepath.Join(tempDir, "modifiedBaseEC.bin"),
		versionFile: filepath.Join(tempDir, "version.txt"),
	}

	s.Log("Saving the base ec firmware version before flashing an old image")
	originalBaseEC, err := getBaseECInfo(ctx, dut)
	if err != nil {
		s.Fatal("Failed to check base ec's version: ", err)
	}

	if err := modifyBaseEC(ctx, dut, originalBaseEC, &fileDir); err != nil {
		s.Fatal("Failed to modify base-ec: ", err)
	}

	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	s.Log("Flashing an old image to detachable-base ec")
	if err := flashAnOldImgToDetachableBaseEC(ctx, dut, hammerConfigs, fileDir.modifiedBin); err != nil {
		// If flashing an edited image fails, check whether the version
		// has changed. Sometimes, this failure might relate to the protection
		// pipeline designed to guarantee a file's integrity, like so:
		// Error message: libminijail[9206]: child process 9207 exited
		// with status 14.
		s.Log("Failed to flash base ec to an old version: ", err)
		flashedBaseEC, err := getBaseECInfo(ctx, dut)
		if err != nil {
			s.Fatal("Failed to get base ec info after flash: ", err)
		}

		s.Logf("Base ec version: %q [Before] v.s. %q [After]", originalBaseEC.version, flashedBaseEC.version)
		if baseECVersionUnchanged(originalBaseEC.version[len(originalBaseEC.name)+1:], flashedBaseEC.version[len(flashedBaseEC.name)+1:]) {
			s.Fatalf("Found base ec version unchanged, got before: %q, and after: %q", originalBaseEC.version, flashedBaseEC.version)
		}
	}

	// Given that DUT's base ec is running an old firmware,
	// detaching then re-attaching base would trigger an update
	// notification window to pop up in a logged in session.
	if err := triggerAndFindNotification(ctx, ecTool, utilServiceClient, dut, originalBaseEC.roProtected); err != nil {
		s.Fatal("Failed to trigger and find notification window: ", err)
	}

	// Restore the firmware to the original version
	if err := dut.Conn().CommandContext(
		ctx, "start", "hammerd", "UPDATE_IF=mismatch", "AT_BOOT=true",
	).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal(err, "unable to run the hammerd command")
	}

	s.Log("Saving the current base ec firmware version")
	newBaseEC, err := getBaseECInfo(ctx, dut)
	if err != nil {
		s.Fatal("Failed to check base ec's version: ", err)
	}

	s.Log("Verifying that base ec restored after a reboot")
	if !baseECVersionUnchanged(originalBaseEC.version[len(originalBaseEC.name)+1:], newBaseEC.version[len(newBaseEC.name)+1:]) {
		s.Fatal("Failed to update the base ec back to default version")
	}
}

func flashAnOldImgToDetachableBaseEC(ctx context.Context, dut *dut.DUT, hammerConfigs hammerRequiredVariables, dstImg string) error {
	path := ""
	if hammerConfigs.usbPath != "" {
		path = "--usb_path=" + hammerConfigs.usbPath
	} else if hammerConfigs.i2cPath != "" {
		path = "--i2c_path=" + hammerConfigs.i2cPath
	}

	if err := dut.Conn().CommandContext(
		ctx,
		"/sbin/minijail0", "-e", "-N", "-p", "-l", "-u",
		"hammerd", "-g", "hammerd", "-c", "0002", "/usr/bin/hammerd",
		"--ec_image_path="+dstImg,
		"--product_id="+hammerConfigs.pid,
		"--vendor_id="+hammerConfigs.vid,
		path,
		"--update_if=always",
	).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "unable to run the hammerd command")
	}
	return nil
}

func baseECVersionUnchanged(old, new string) bool {
	// semver.Compare requires a 'v' prefix
	if !strings.HasPrefix(old, "v") {
		old = "v" + old
	}
	if !strings.HasPrefix(new, "v") {
		new = "v" + new
	}
	return semver.Compare(old, new) == 0
}

func getBaseECInfo(ctx context.Context, dut *dut.DUT) (baseECInfo, error) {
	var baseEC baseECInfo

	outputHammerInfo := ""
	// Poll `hammer_info` until the keyboard is ready, as the first few iterations
	// might run into the 'can't find device' error.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		output, err := dut.Conn().CommandContext(ctx, "hammer_info.py").Output(testexec.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to run hammer_info.py command in the dut")
		}
		outputHammerInfo = string(output)
		return nil
	}, &testing.PollOptions{Interval: 1 * time.Second, Timeout: 10 * time.Second}); err != nil {
		return baseEC, errors.Wrap(err, "failed to get the info from hammer_info.py")
	}

	baseECInfoMap := map[string]*regexp.Regexp{
		"name":             regexp.MustCompile(`rw_version="(\w+)(?:_v|-)`),
		"version":          regexp.MustCompile(`rw_version="([\w-.]+)"`),
		"protection flags": regexp.MustCompile(`wp_all="(\w+)`),
	}

	for k, re := range baseECInfoMap {
		match := re.FindStringSubmatch(outputHammerInfo)
		if len(match) < 2 {
			return baseEC, errors.Errorf("did not match regex %q in %q", re, outputHammerInfo)
		}
		value := strings.TrimSpace(match[1])

		switch k {
		case "name":
			baseEC.name = value
		case "version":
			baseEC.version = value
		case "protection flags":
			baseEC.roProtected = (value == "True")
		}
	}

	return baseEC, nil
}

func triggerAndFindNotification(ctx context.Context, ecTool *firmware.ECTool, utilSvcClient fwpb.UtilsServiceClient, dut *dut.DUT, roProtected bool) error {
	hammerdLog := "/var/log/hammerd.log"
	originalHammerdID, err := hammerdProcessID(ctx, hammerdLog, dut)
	if err != nil {
		return errors.Wrap(err, "failed to get hammerd process id")
	}

	setter, err := getBaseStateSetter(ctx, ecTool)
	if err != nil {
		errors.Wrap(err, "failed to get base state setter")
	}
	// Detach then re-attach detachable's base to trigger update
	// notification.
	// We assume the base keyboard is always physically attached to the
	// device during testing, and we use `TabletAuto` to revert forcing base
	// state, so the state is back to attached.
	for _, state := range []firmware.ECToolBaseState{firmware.BaseDetach, firmware.BaseAuto} {
		if err := setter.SetBaseState(ctx, state); err != nil {
			return errors.Wrap(err, "failed to switch the base state")
		}
		// GoBigSleepLint: Allow some delay to ensure base attached/detached by setting the gpio.
		if err := testing.Sleep(ctx, 10*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep for 10 seconds for the command to fully propagate to the DUT")
		}
	}

	newHammerdID, err := hammerdProcessID(ctx, hammerdLog, dut)
	if err != nil {
		return errors.Wrap(err, "failed to get hammerd process id")
	}
	testing.ContextLogf(ctx, "Hammerd process ids: %s [before re-attach], %s [after re-attach]", originalHammerdID, newHammerdID)

	testing.ContextLog(ctx, "Finding notification window")
	if _, err := utilSvcClient.FindSingleNode(ctx, &fwpb.NodeElement{Name: "Your detachable keyboard needs a critical update"}); err != nil {
		if roProtected && strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
			// When RO locked, broken RW would get restored by hammerd silently.
			testing.ContextLog(ctx, "Found RO locked, skip verifying pop-up window")
			return nil
		}
		if originalHammerdID == newHammerdID {
			return errors.Wrap(err, "hammerd did not restart following base power-cycle")
		}
		return errors.Wrap(err, "failed to find notification of detachable keyboard update")

	}
	return nil
}

// modifyBaseEC copies the /lib/firmware/base-ec.fw to local,
// modifies its version and puts it back to /tmp/ folder in DUT.
func modifyBaseEC(ctx context.Context, dut *dut.DUT, boardInfo baseECInfo, fileDir *modifiedFileDir) error {
	originalBaseECBinFile := fmt.Sprintf("/lib/firmware/%s.fw", boardInfo.name)

	testing.ContextLog(ctx, "Current base-ec version: ", boardInfo.version)
	testing.ContextLog(ctx, "Starting to modify base-ec.bin")

	if err := linuxssh.WriteFile(ctx, dut.Conn(), fileDir.versionFile, []byte(boardInfo.name+"_v999.0.0\x00"), 0644); err != nil {
		return errors.Wrap(err, "failed to create version file on DUT")
	}

	futilityInstance, err := futility.NewLocalBuilder(dut).Debug(true).Build()
	if err != nil {
		return errors.Wrap(err, "failed to setup futility instance")
	}

	futilityOutput, err := futilityInstance.LoadFmap(ctx, originalBaseECBinFile, fileDir.modifiedBin, map[string]string{
		"RW_FWID": fileDir.versionFile,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to modify version string: %s", futilityOutput)
	}

	return nil
}

func getBaseStateSetter(ctx context.Context, ecTool *firmware.ECTool) (baseStateSetter, error) {
	// Included in baseGpioNames are a list of possible gpios available for
	// controlling the base state. The first one found from the list would
	// be used in setting base state attached/detached.
	// If no GPIO matched, use ECTool as base state setter.
	baseGpioNames := []firmware.GpioName{firmware.ENBASE, firmware.ENPP3300POGO, firmware.PP3300DXBASE, firmware.BASEPWREN}
	foundNames, err := ecTool.FindGPIOs(ctx, baseGpioNames)
	if err != nil {
		return nil, errors.Wrap(err, "FindGPIOs failed")
	}

	for _, name := range baseGpioNames {
		if _, ok := foundNames[name]; ok {
			return &gpioBaseStateSetter{name: string(name), ecTool: ecTool}, nil
		}
	}
	return ecTool, nil
}

func hammerdProcessID(ctx context.Context, hammerdLog string, dut *dut.DUT) (string, error) {
	cmd := fmt.Sprintf("tail -1 %s | cut -d ' ' -f3 | grep -o '[[:digit:]]*'", hammerdLog)
	processID, err := dut.Conn().CommandContext(ctx, "bash", "-c", cmd).Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrapf(err, "failed to run %s", cmd)
	}
	return strings.TrimSpace(string(processID)), nil
}

func getHammerConfig(ctx context.Context, h *firmware.Helper, utilServiceClient fwpb.UtilsServiceClient) (hammerRequiredVariables, error) {
	// hammerConfigsMap contains information about pid, vid, and usbPath values of a
	// detachable base for different models. This information was derived from manual
	// testing and the following hammer file:
	// https://chromium.googlesource.com/chromiumos/platform/ec/+/HEAD/board/hammer/variants.h
	hammerConfigsMap := map[string]hammerRequiredVariables{
		"coachz":        {pid: "20556", vid: "6353", usbPath: "1-1.4"},
		"nocturne":      {pid: "20528", vid: "6353", usbPath: "1-7"},
		"soraka":        {pid: "20523", vid: "6353", usbPath: "1-2"},
		"krane":         {pid: "20540", vid: "6353", usbPath: "1-1.1"},
		"kakadu":        {pid: "20548", vid: "6353", usbPath: "1-1.1"},
		"katsu":         {pid: "20560", vid: "6353", usbPath: "1-1.1"},
		"homestar":      {pid: "20562", vid: "6353", usbPath: "1-1.1"},
		"wormdingler":   {pid: "20567", vid: "6353", usbPath: "1-1.3"},
		"quackingstick": {pid: "20571", vid: "6353", usbPath: "1-1.1"},
	}

	var hammerConfigs hammerRequiredVariables
	assignConfigs := func() error {
		testing.ContextLog(ctx, "Attempting detachable base attributes from the hammer file")
		modelName, err := h.Reporter.Model(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get the dut's model")
		}
		hammerConfigs.pid = hammerConfigsMap[modelName].pid
		hammerConfigs.vid = hammerConfigsMap[modelName].vid
		hammerConfigs.usbPath = hammerConfigsMap[modelName].usbPath
		return nil
	}

	crosCfgRes, err := utilServiceClient.GetDetachableBaseValue(ctx, &empty.Empty{})
	if err != nil {
		testing.ContextLog(ctx, "Failed to get detachable-base attribute values: ", err)
		if err := assignConfigs(); err != nil {
			return hammerConfigs, errors.Wrap(err, "usnable to set attributes")
		}
	} else {
		hammerConfigs.pid = crosCfgRes.ProductId
		hammerConfigs.vid = crosCfgRes.VendorId
		hammerConfigs.usbPath = crosCfgRes.UsbPath
		hammerConfigs.i2cPath = crosCfgRes.I2CPath
	}
	return hammerConfigs, nil
}
