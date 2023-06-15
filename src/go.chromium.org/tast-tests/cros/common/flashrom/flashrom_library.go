// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/*
Package flashrom is Flashrom Tast library to be used by all Tast tests.

Instructions:
1) Use the Config to construct the Instance of flashrom.
2) Run Probe() method on the Config to complete initialisation of your instance
and probe the chip. Probe() method returns Instance which is ready to use.
3) Invoke methods of your Instance to run flashrom operations.
4) Shutdown your instance at the end.

Sample usage:

	var flashromConfig flashrom.Config
	flashromInstance, ctx, fullShutdown, out, err := flashromConfig.
		FlashromInit(flashrom.VerbosityDebug).
		ProgrammerInit(flashrom.ProgrammerHost, "").
		SetDut(s.DUT).
		Probe(ctx)

	defer func() {
		if err := fullShutdown(); err != nil {
			s.Error("Failed to shutdown flashromInstance: ", err)
		}
	}()
	if err != nil {
		s.Error("Failed to probe flashrom instance: ", err)
	}

	retCode, err := instance.Read(ctx, "tmp/dump.bin")
*/
package flashrom

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

const (
	// Full path to flashrom binary on DUT.
	dutFlashromPath = "/usr/sbin/flashrom"

	// Message in the output which means the chip has been found.
	chipFoundMessage = `Found .* flash chip`

	// Message in the output which indicates that software WP status is enabled.
	wpStatusEnabled = `WP: write protect is enabled.`

	// Message in the output which indicates that software WP status is disabled.
	wpStatusDisabled = `WP: write protect is disabled.`

	// Error message for software WP status.
	wpStatusErrorMessage = `Failed to get WP status: (.*)\n`

	// Success message for non-WP operations.
	nonwpSuccessMessage = `SUCCESS`
)

// runCommandLineRemote creates command context from given connection and runs command line with given arguments.
//
// It returns joint slice of stderr and stdout from command line execution.
// When any error happened during command execution (including non-zero exit status in remote), non-nil error
// is returned.
func runCommandLineRemote(ctx context.Context, conn *ssh.Conn, args []string) ([]byte, error) {
	testing.ContextLog(ctx, "Running command line remotely with arguments: ", args)
	cmd := conn.CommandContext(ctx, args[0], args[1:]...)

	var outbuf, errbuf bytes.Buffer
	cmd.Stdout = &outbuf
	cmd.Stderr = &errbuf

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	if err := cmd.Wait(); err != nil {
		return bytes.Join([][]byte{errbuf.Bytes(), outbuf.Bytes()}, []byte("\n")), errors.Wrapf(err, "command %q failed", strings.Join(cmd.Args, " "))
	}

	// TODO(b:247668196) implement full logging if test gives a file?

	return bytes.Join([][]byte{errbuf.Bytes(), outbuf.Bytes()}, []byte("\n")), nil
}

// runCommandLineLocal runs command line using testexec command context.
//
// It returns joint slice of stderr and stdout from command line execution.
// When any error happened during command execution non-nil error is returned.
func runCommandLineLocal(ctx context.Context, args []string) ([]byte, error) {
	testing.ContextLog(ctx, "Running command line locally with arguments: ", args)
	cmd := testexec.CommandContext(ctx, args[0], args[1:]...)

	var outbuf, errbuf bytes.Buffer
	cmd.Stdout = &outbuf
	cmd.Stderr = &errbuf

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	if err := cmd.Wait(); err != nil {
		return bytes.Join([][]byte{errbuf.Bytes(), outbuf.Bytes()}, []byte("\n")), errors.Wrapf(err, "command %q failed", strings.Join(cmd.Args, " "))
	}

	return bytes.Join([][]byte{errbuf.Bytes(), outbuf.Bytes()}, []byte("\n")), nil
}

// Programmer is Flashrom programmer, one of the values below.
type Programmer string

// Programmers currently supported for tast tests.
const (
	ProgrammerDummyflasher   Programmer = "dummy"
	ProgrammerEc             Programmer = "ec"
	ProgrammerFt2232spi      Programmer = "ft2232_spi"
	ProgrammerHost           Programmer = "host"
	ProgrammerRaidenDebugSpi Programmer = "raiden_debug_spi"
)

func (p *Programmer) isSupported() bool {
	if *p == ProgrammerDummyflasher ||
		*p == ProgrammerEc ||
		*p == ProgrammerFt2232spi ||
		*p == ProgrammerHost ||
		*p == ProgrammerRaidenDebugSpi {
		return true
	}
	return false
}

// VerbosityLevel is Flashrom native logging verbosity level, one of the values below.
type VerbosityLevel string

// Supported verbosity levels, corresponding to the levels defined in libflashrom.h.
const (
	VerbosityInfo   VerbosityLevel = ""     // FLASHROM_MSG_INFO (not verbose)
	VerbosityDebug  VerbosityLevel = "-V"   // FLASHROM_MSG_DEBUG (a little verbose)
	VerbosityDebug2 VerbosityLevel = "-VV"  // FLASHROM_MSG_DEBUG2 (medium verbosity)
	VerbositySpew   VerbosityLevel = "-VVV" // FLASHROM_MSG_SPEW (high verbosity)
)

func (v *VerbosityLevel) isSupported() bool {
	if *v == VerbosityInfo ||
		*v == VerbosityDebug ||
		*v == VerbosityDebug2 ||
		*v == VerbositySpew {
		return true
	}
	return false
}

// Params are common parameters required for all library methods.
type Params struct {
	verbosity       VerbosityLevel
	programmer      Programmer
	programmerParam string
	commandRunner   func(ctx context.Context, cmdArgs []string) ([]byte, error)
	servo           *servo.Servo
	servoSPIControl servo.OnOffControl
}

// Config configures flashrom instance.
type Config struct {
	params Params
}

// Instance of flashrom to run operations.
type Instance struct {
	params Params
}

// programmerWithParamsArg constructs programmer param command line argument (to be used in command line
// invocation) out of given programmer and programmer param string.
func (i *Instance) programmerWithParamsArg() string {
	programmerWithParams := string(i.params.programmer)
	if i.params.programmerParam != "" {
		programmerWithParams = fmt.Sprintf("%v:%v", programmerWithParams, i.params.programmerParam)
	}
	return programmerWithParams
}

// appendVerbosityArg qppends command line argument for verbosity level to the array of given command line
// arguments.
func (i *Instance) appendVerbosityArg(cmdArgs []string) []string {
	if i.params.verbosity != VerbosityInfo {
		return append(cmdArgs, string(i.params.verbosity))
	}
	return cmdArgs
}

// appendFileAndRegionNamesArgs appends command line arguments for fullSizeFilePath and/or region names
// to the array of given command line arguments.
func appendFileAndRegionNamesArgs(cmdArgs []string, fullSizeFilePath string, regionNames []string) []string {
	if fullSizeFilePath != "" {
		cmdArgs = append(cmdArgs, fullSizeFilePath)
	}
	for _, regionName := range regionNames {
		// Only non-empty string is allowed after -i.
		if regionName != "" {
			cmdArgs = append(cmdArgs, "-i", regionName)
		}
	}
	return cmdArgs
}

// runCommandLine detects whether flashrom instance is set up for a local or remote test
// and runs command line with given command line arguments accordingly.
// It returns the output from command line execution.
// If an error happens during command line execution, a non-nil error is returned.
func (i *Instance) runCommandLine(ctx context.Context, cmdArgs []string) ([]byte, error) {
	// TODO(b:247668196) make sure errors are informative for the caller.
	return i.params.commandRunner(ctx, cmdArgs)
}

// FlashromInit sets verbosity level.
// If verbosity is not given, it is set to flashromMsgInfo.
func (c *Config) FlashromInit(verbosity VerbosityLevel) *Config {
	c.params.verbosity = verbosity
	if verbosity == "" {
		c.params.verbosity = VerbosityInfo
	}
	return c
}

// ProgrammerInit initialises flashrom programmer with given params.
// If programmer is not given, it defaults to ProgrammerHost.
// Programmer params can be empty string, if no params are needed to initialise given programmer..
func (c *Config) ProgrammerInit(programmer Programmer, programmerParams string) *Config {
	c.params.programmer = programmer
	if programmer == "" {
		c.params.programmer = ProgrammerHost
	}
	c.params.programmerParam = programmerParams
	return c
}

// SetDut sets dut for test run. nil indicates local run.
func (c *Config) SetDut(testDut *dut.DUT) *Config {
	c.params.commandRunner = func(ctx context.Context, cmdArgs []string) ([]byte, error) {
		return runCommandLineRemote(ctx, testDut.Conn(), cmdArgs)
	}
	return c
}

// SetServoProxy sets servo proxy to run commands on the servo host. Don't call ProgrammerInit if you call this.
// The Probe() function will setup the servo and DUT for flashing, and the shutdown function returned from Probe()
// will restore the servo to it's normal state, and reboot the DUT, so be sure to call the shutdown function as soon as possible
// after flashing.
func (c *Config) SetServoProxy(proxy *servo.Proxy) *Config {
	c.params.servo = proxy.Servo()
	c.params.commandRunner = func(ctx context.Context, cmdArgs []string) ([]byte, error) {
		return proxy.OutputCommand(ctx, true, cmdArgs[0], cmdArgs[1:]...)
	}
	return c
}

// isReady returns true if config has all params set and can create an Instance, false otherwise.
func (c *Config) isReady() error {
	if !c.params.verbosity.isSupported() {
		return errors.Errorf("invalid config: verbosity level %q is unsupported", c.params.verbosity)
	}
	if !c.params.programmer.isSupported() {
		return errors.Errorf("invalid config: programmer %q is unsupported", c.params.programmer)
	}
	return nil
}

// Probe gets a flashrom instance that will probe the chip.
//
// Returns flashrom instance which is ready to use, or nil instance and error.
// Returns a context derived from ctx with time reserved for cleanup.
// Returns a closure to perform full cleanup of flashrom instance.
// Returns the output from command line execution, so that the caller can handle it if needed,
// for example store in log file.
//
// ctx, func and []byte are always returned, even in the case of error, and func must always be called.
// Instance is only returned when error = nil.
func (c *Config) Probe(ctx context.Context) (*Instance, context.Context, func() error, []byte, error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	shutdown := func() error {
		err := c.fullShutdown(cleanupCtx)
		cancel()
		return err
	}

	if c.params.servo != nil {
		if c.params.programmer != "" {
			return nil, ctx, shutdown, nil, errors.New("cannot call both SetServoProxy and ProgrammerInit")
		}
		devices, err := c.params.servo.GetStringList(ctx, servo.Devices)
		if err != nil {
			return nil, ctx, shutdown, nil, errors.Wrap(err, "failed to read servo devices")
		}

		for _, device := range devices {
			stringType, err := servo.PropertyToString(device, "type")
			if err != nil {
				return nil, ctx, shutdown, nil, errors.Wrapf(err, "no type in %v", device)
			}
			deviceSerial, err := servo.PropertyToString(device, "serial")
			if err != nil {
				return nil, ctx, shutdown, nil, errors.Wrapf(err, "no serial in %v", device)
			}
			if strings.HasPrefix(stringType, "ccd") {
				c.params.servoSPIControl = servo.CCDCPUFWSPI
				c.params.programmer = ProgrammerRaidenDebugSpi
				c.params.programmerParam = fmt.Sprintf("target=AP,custom_rst=true,serial=%s", deviceSerial)
				// Don't break, continue to see if we can find a better choice than CCD.
			} else if strings.HasPrefix(stringType, "c2d2") || strings.HasPrefix(stringType, "servo_micro") {
				c.params.servoSPIControl = servo.CPUFWSPI
				c.params.programmer = ProgrammerRaidenDebugSpi
				c.params.programmerParam = fmt.Sprintf("serial=%s", deviceSerial)
				break
			} else if strings.HasPrefix(stringType, "servo_v2") {
				c.params.servoSPIControl = servo.CPUFWSPI
				c.params.programmer = ProgrammerFt2232spi
				c.params.programmerParam = fmt.Sprintf("type=google-servo-v2,serial=%s", deviceSerial)
				break
			}
		}

		if c.params.servoSPIControl == "" {
			return nil, ctx, shutdown, nil, errors.Errorf("failed to find servo programmer, got %v", devices)
		}
		testing.ContextLogf(ctx, "Setting %s:on", c.params.servoSPIControl)
		if err := c.params.servo.SetOnOff(ctx, c.params.servoSPIControl, servo.On); err != nil {
			return nil, ctx, shutdown, nil, errors.Wrapf(err, "failed to enable %s", c.params.servoSPIControl)
		}
	}
	if err := c.isReady(); err != nil {
		return nil, ctx, shutdown, nil, errors.Wrap(err, "config missing required data")
	}

	var instance Instance
	instance.params = c.params
	if instance.params.commandRunner == nil {
		instance.params.commandRunner = runCommandLineLocal
	}

	cmdArgs := []string{dutFlashromPath, "-p", instance.programmerWithParamsArg()}
	cmdArgs = instance.appendVerbosityArg(cmdArgs)

	out, err := instance.runCommandLine(ctx, cmdArgs)

	if err != nil {
		return nil, ctx, shutdown, out, errors.Wrapf(err, "error while probing flashrom with arguments %v", cmdArgs)
	}

	re := regexp.MustCompile(chipFoundMessage)
	chipsFound := re.FindAllString(string(out), -1)
	if chipsFound == nil || len(chipsFound) == 0 {
		return nil, ctx, shutdown, out, errors.Errorf("Flashrom probe fails to find a chip, cmdArgs=%v", cmdArgs)
	}
	if len(chipsFound) > 1 && chipsFound[0] != chipsFound[1] {
		return nil, ctx, shutdown, out, errors.Errorf("Flashrom probe with cmdArgs=%v found multiple chips (%v), %s VS %s",
			cmdArgs, len(chipsFound), chipsFound[0], chipsFound[1])
	}

	testing.ContextLog(ctx, "Flashrom probe successful: ", chipsFound[0])

	return &instance, ctx, shutdown, out, nil
}

// fullShutdown shuts down flashrom programmer, cleans up all resources and shuts down flashrom.
// Returns error if error happened during shutdown, or nil otherwise.
func (c *Config) fullShutdown(ctx context.Context) error {
	// TODO Implement when switching the library to use libflashrom.
	// Shutdown is called implicitly for command line invocations.
	if c.params.servo != nil {
		testing.ContextLogf(ctx, "Setting %s:off", c.params.servoSPIControl)
		if err := c.params.servo.SetOnOff(ctx, c.params.servoSPIControl, servo.Off); err != nil {
			return errors.Wrapf(err, "failed to disable %s", c.params.servoSPIControl)
		}
	}

	return nil
}

// SoftwareWriteProtectStatus requests software write-protect status of the chip.
//
// Returns:
// boolean value of WP status, true means WP enabled, false means WP disabled
// output from command line execution, so that the caller can handle it if needed
// error if it happened or nil
func (i *Instance) SoftwareWriteProtectStatus(ctx context.Context) (bool, []byte, error) {
	cmdArgs := []string{dutFlashromPath, "-p", i.programmerWithParamsArg(), "--wp-status"}
	cmdArgs = i.appendVerbosityArg(cmdArgs)

	out, err := i.runCommandLine(ctx, cmdArgs)
	if err != nil {
		return false, out, errors.Wrapf(err, "error while checking software write-protect status with arguments %v", cmdArgs)
	}

	strOut := string(out)
	if strings.Contains(strOut, wpStatusEnabled) {
		return true, out, nil
	}
	if strings.Contains(strOut, wpStatusDisabled) {
		return false, out, nil
	}

	re := regexp.MustCompile(wpStatusErrorMessage)
	wpError := re.FindString(strOut)
	if wpError == "" {
		return false, out, errors.Errorf("Flashrom WP status is unknown, cmdArgs=%v", cmdArgs)
	}

	return false, out, errors.Errorf("%s", wpError)
}

// Read reads the chip into the file provided by filePath. Note the filePath is a path on the DUT,
// which will not be local in case of remote test.
// If optional parameter regionNames is provided, only given regions are read.
// nil as regionNames indicates entire chip.
//
// At least one of fullSizeFilePath or regionNames argument must be provided non-empty.
// Both fullSizeFilePath and regionNames can be provided non-empty.
//
// If an error happened during read operation, a non-nil error is returned.
// Returns the output from command line execution, so that the caller can handle it if needed,
// for example store in log file.
func (i *Instance) Read(ctx context.Context, fullSizeFilePath string, regionNames []string) ([]byte, error) {
	if fullSizeFilePath == "" && len(regionNames) == 0 {
		return nil, errors.New("Flashrom cannot do read: both filePath and regionNames arguments are empty")
	}

	cmdArgs := []string{dutFlashromPath, "-p", i.programmerWithParamsArg(), "-r"}
	cmdArgs = appendFileAndRegionNamesArgs(cmdArgs, fullSizeFilePath, regionNames)
	cmdArgs = i.appendVerbosityArg(cmdArgs)

	out, err := i.runCommandLine(ctx, cmdArgs)

	if err != nil {
		return out, errors.Wrapf(err, "error while reading flashrom with arguments %v", cmdArgs)
	}

	if !strings.Contains(string(out), nonwpSuccessMessage) {
		return out, errors.Errorf("Flashrom read operation did not produce success message, cmdArgs=%v", cmdArgs)
	}

	testing.ContextLog(ctx, "Flashrom read successful: ", cmdArgs)

	return out, nil
}

// Size returns the flash size.
//
// If an error happened, a non-nil error is returned.
// Returns the output from command line execution, so that the caller can handle it if needed,
// for example store in log file.
func (i *Instance) Size(ctx context.Context) (int, []byte, error) {
	cmdArgs := []string{dutFlashromPath, "-p", i.programmerWithParamsArg(), "--flash-size"}
	// Verbosity cannot be used with --flash-size, as there is no way to
	// distinguish which line of output is the size if verbosity is used.
	out, err := i.runCommandLine(ctx, cmdArgs)
	if err != nil {
		return -1, out, err
	}
	// The size is printed as the last line, for example `8388608\n`.
	lastLineStart := bytes.LastIndexByte(out[:len(out)-1], '\n') + 1
	lastLine := string(out[lastLineStart : len(out)-1])
	size, err := strconv.ParseInt(lastLine, 10, 0)
	if err != nil {
		return -1, out, errors.Wrapf(err, "could not parse flash size to int: %q", lastLine)
	}
	return int(size), out, nil
}

// SoftwareWriteProtectRegion enables software write protect for the specified region.
func (i *Instance) SoftwareWriteProtectRegion(ctx context.Context, wpRegionName string) (int, error) {
	// TODO(b:247668196) implement

	return 0, nil
}

// SoftwareWriteProtectEnableWithRange enables software write-protect and sets write-protect range on the chip.
func (i *Instance) SoftwareWriteProtectEnableWithRange(ctx context.Context, wpRange string) (int, error) {
	// TODO(b:247668196) implement

	return 0, nil
}

// SoftwareWriteProtectEnable enables software write-protect for the entire chip.
// Warning: this may cause the DUT to crash, or fail to boot. Use with caution.
// Returns output from command line execution, so that the caller can handle it if needed.
// Returns error if it happened or nil.
func (i *Instance) SoftwareWriteProtectEnable(ctx context.Context) ([]byte, error) {
	flashSize, out, err := i.Size(ctx)
	if err != nil {
		return out, err
	}
	cmdArgs := []string{dutFlashromPath, "-p", i.programmerWithParamsArg()}
	cmdArgs = append(cmdArgs, "--wp-enable", fmt.Sprintf("--wp-range=0,%d", flashSize))
	cmdArgs = i.appendVerbosityArg(cmdArgs)

	out, err = i.runCommandLine(ctx, cmdArgs)
	if err != nil {
		return out, errors.Wrapf(err, "error while enabling software write-protect with arguments %v", cmdArgs)
	}
	return out, nil
}

// SoftwareWriteProtectDisable disables software write-protect and sets the range to 0,0.
//
// Returns output from command line execution, so that the caller can handle it if needed.
// Returns error if it happened or nil.
func (i *Instance) SoftwareWriteProtectDisable(ctx context.Context) ([]byte, error) {
	cmdArgs := []string{dutFlashromPath, "-p", i.programmerWithParamsArg(), "--wp-disable", "--wp-range=0,0"}
	cmdArgs = i.appendVerbosityArg(cmdArgs)

	out, err := i.runCommandLine(ctx, cmdArgs)
	if err != nil {
		return out, errors.Wrapf(err, "error while disabling software write-protect with arguments %v", cmdArgs)
	}

	return out, nil
}

// Write writes data on chip from the file provided by filePath. Note filePath is a path on the DUT,
// which will not be local in case of remote test.
// If optional parameter regionNames is provided, only given regions are written.
// nil as regionNames indicates entire chip.
// noverifyAll==true adds `--noverify-all` argument to command line
// noverify==true adds `--noverify` argument to command line
// Providing flashcontentsImage path adds `--flash-contents flashcontentsImage` to command line.
// Note that flashcontentsImage is a path on the DUT, which will not be local in case of remote test.
//
// At least one of fullSizeFilePath or regionNames argument must be provided non-empty.
// Both fullSizeFilePath and regionNames can be provided non-empty.
//
// If an error happened during write operation, a non-nil error is returned.
// Returns the output from command line execution, so that the caller can handle it if needed,
// for example store in log file.
func (i *Instance) Write(ctx context.Context, fullSizeFilePath string, noverifyAll, noverify bool, flashcontentsImage string, regionNames []string) ([]byte, error) {
	if fullSizeFilePath == "" && len(regionNames) == 0 {
		return nil, errors.New("Flashrom cannot do write: both fullSizeFilePath and regionNames are empty")
	}

	cmdArgs := []string{dutFlashromPath, "-p", i.programmerWithParamsArg(), "-w"}
	cmdArgs = appendFileAndRegionNamesArgs(cmdArgs, fullSizeFilePath, regionNames)
	if flashcontentsImage != "" {
		cmdArgs = append(cmdArgs, "--flash-contents", flashcontentsImage)
	}
	if noverifyAll {
		cmdArgs = append(cmdArgs, "--noverify-all")
	}
	if noverify {
		cmdArgs = append(cmdArgs, "--noverify")
	}
	cmdArgs = i.appendVerbosityArg(cmdArgs)

	out, err := i.runCommandLine(ctx, cmdArgs)

	if err != nil {
		return out, errors.Wrapf(err, "error while writing flashrom with arguments %v", cmdArgs)
	}

	if !strings.Contains(string(out), nonwpSuccessMessage) {
		return out, errors.Errorf("Flashrom write operation did not produce success message, cmdArgs=%v", cmdArgs)
	}

	testing.ContextLog(ctx, "Flashrom write successful: ", cmdArgs)

	return out, nil
}

// Verify compares data on chip from the file provided by filePath. Note filePath is a path on the DUT,
// which will not be local in case of remote test.
// If optional parameter regionNames is provided, only given regions are verified.
// nil as regionNames indicates entire chip.
// If an error happened during verify operation, a non-nil error is returned.
// Returns the output from command line execution, so that the caller can handle it if needed,
// for example store in log file.
func (i *Instance) Verify(ctx context.Context, filePath string, regionNames ...string) ([]byte, error) {
	if filePath == "" {
		return nil, errors.New("Flashrom cannot do verify: empty filePath argument")
	}

	cmdArgs := []string{dutFlashromPath, "-p", i.programmerWithParamsArg(), "--verify", filePath}
	cmdArgs = appendFileAndRegionNamesArgs(cmdArgs, "", regionNames)
	cmdArgs = i.appendVerbosityArg(cmdArgs)

	out, err := i.runCommandLine(ctx, cmdArgs)
	if err != nil {
		return out, errors.Wrapf(err, "error while verifying flashrom with arguments %v", cmdArgs)
	}

	testing.ContextLog(ctx, "Flashrom verify successful: ", cmdArgs)

	return out, nil
}

// Erase erases the flash memory.
//
// If an error happened, a non-nil error is returned.
// Returns the output from command line execution, so that the caller can handle it if needed.
func (i *Instance) Erase(ctx context.Context, regionNames []string) ([]byte, error) {
	cmdArgs := []string{dutFlashromPath, "-p", i.programmerWithParamsArg(), "-E"}
	cmdArgs = appendFileAndRegionNamesArgs(cmdArgs, "", regionNames)

	out, err := i.runCommandLine(ctx, cmdArgs)
	if err != nil {
		return out, errors.Wrapf(err, "error while erase flashrom with arguments %v", cmdArgs)
	}

	testing.ContextLog(ctx, "Flashrom erase successful: ", cmdArgs)
	return out, nil
}

// NoOp runs flashrom with no arguments other than the verbosity.
//
// flashrom prints some useful info (more useful with -V).
// Returns the output of flashrom.
func (i *Instance) NoOp(ctx context.Context) (string, error) {
	cmdArgs := []string{dutFlashromPath}
	cmdArgs = i.appendVerbosityArg(cmdArgs)
	out, err := i.runCommandLine(ctx, cmdArgs)
	return string(out), err
}
