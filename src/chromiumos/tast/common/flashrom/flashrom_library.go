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
flashromInstance, out, err := flashromConfig.

	FlashromInit(flashrom.VerbosityDebug).
	ProgrammerInit(flashrom.ProgrammerHost, "").
	SetDut(s.DUT).
	Probe(ctx)

defer instance.FullShutdown(ctx)

retCode, err := instance.Read(ctx, "tmp/dump.bin")
*/
package flashrom

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/dut"
	"chromiumos/tast/errors"
	"chromiumos/tast/ssh"
	"chromiumos/tast/testing"
)

const (
	// Full path to flashrom binary on DUT.
	dutFlashromPath = "/usr/sbin/flashrom"

	// Message in the output which means the chip has been found.
	chipFoundMessage = `Found .* flash chip`
)

// runCommandLineRemote creates command context from given connection and runs command line with given arguments.
//
// Returns:
// When the command execution succeeded, the byte slice contains the output to stdout and the error is nil.
// When any error happened during command execution (including non-zero exit status in remote), non-nil error
// is returned and the byte slice data is invalid.
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
		return nil, errors.Wrapf(err, "command %q failed", strings.Join(cmd.Args, " "))
	}

	// TODO(b:247668196) implement full logging if test gives a file?

	return bytes.Join([][]byte{outbuf.Bytes(), errbuf.Bytes()}, []byte("\n")), nil
}

// runCommandLineLocal runs command line using testexec command context.
func runCommandLineLocal(ctx context.Context, args []string) ([]byte, error) {
	testing.ContextLog(ctx, "Running command line locally with arguments: ", args)
	cmd := testexec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.Output(testexec.DumpLogOnError)

	if err != nil {
		err = errors.Wrapf(err, "command %q failed", strings.Join(cmd.Args, " "))
		return nil, err
	}

	return output, err
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
	testDut         *dut.DUT // Remote DUT or nil for local run
}

// Config configures flashrom instance.
type Config struct {
	params Params
}

// Instance of flashrom to run operations.
type Instance struct {
	params Params
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
	c.params.testDut = testDut
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
// Returns flashrom instance which is ready to use, or nil instance and error.
func (c *Config) Probe(ctx context.Context) (*Instance, []byte, error) {
	if err := c.isReady(); err != nil {
		return nil, nil, errors.Wrap(err, "config missing required data")
	}

	var instance Instance
	instance.params = c.params

	programmerWithParams := string(instance.params.programmer)
	if instance.params.programmerParam != "" {
		programmerWithParams = fmt.Sprintf("%v:%v", programmerWithParams, instance.params.programmerParam)
	}
	cmdArgs := []string{dutFlashromPath, "-p", programmerWithParams}
	cmdArgs = append(cmdArgs, string(instance.params.verbosity))

	var out []byte
	var err error
	if instance.params.testDut != nil {
		out, err = runCommandLineRemote(ctx, instance.params.testDut.Conn(), cmdArgs)
	} else {
		out, err = runCommandLineLocal(ctx, cmdArgs)
	}

	// TODO(b:247668196) make sure errors are informative for the caller.

	// TODO(b:247668196) implement full logging if test gives a file?
	// Note stderr from runCommandLineRemote available inside that functions.

	if err != nil {
		return nil, out, errors.Wrapf(err, "error while probing flashrom with arguments %v", cmdArgs)
	}

	re := regexp.MustCompile(chipFoundMessage)
	chipsFound := re.FindAllString(string(out), -1)
	if chipsFound == nil || len(chipsFound) == 0 {
		return nil, out, errors.Errorf("Flashrom probe fails to find a chip, cmdArgs=%v", cmdArgs)
	}
	if len(chipsFound) > 1 && chipsFound[0] != chipsFound[1] {
		return nil, out, errors.Errorf("Flashrom probe with cmdArgs=%v found multiple chips (%v), %s VS %s",
			cmdArgs, len(chipsFound), chipsFound[0], chipsFound[1])
	}

	testing.ContextLog(ctx, "Flashrom probe successful: ", chipsFound[0])

	return &instance, out, nil
}

// SoftwareWriteProtectStatus requests software write-protect status of the chip.
func (i *Instance) SoftwareWriteProtectStatus(ctx context.Context) (bool, error) {
	// TODO(b:247668196) implement

	return true, nil
}

// Read reads the chip into the file provided by filePath.
// If optional parameter regionNames is provided, only given regions are read.
// nil as regionNames indicates entire chip.
func (i *Instance) Read(ctx context.Context, filePath string, regionNames []string) (int, error) {
	// TODO(b:247668196) implement

	return 0, nil
}

// SoftwareWriteProtectRegion enables software write protect for the specified region.
func (i *Instance) SoftwareWriteProtectRegion(ctx context.Context, wpRegionName string) (int, error) {
	// TODO(b:247668196) implement

	return 0, nil
}

// SoftwareWriteProtectSet sets the value of software write-protect on the chip.
// Enables write-protect if enable parameter is true, disables otherwise.
// Sets the range to 0,0 on disable, and set the range to 0,chip_length on enable.
func (i *Instance) SoftwareWriteProtectSet(ctx context.Context, enable bool) (int, error) {
	// TODO(b:247668196) implement

	return 0, nil
}

// SoftwareWriteProtectEnableWithRange enables software write-protect and sets write-protect range on the chip.
func (i *Instance) SoftwareWriteProtectEnableWithRange(ctx context.Context, wpRange string) (int, error) {
	// TODO(b:247668196) implement

	return 0, nil
}

// Write writes on chip.
func (i *Instance) Write(ctx context.Context, filePath string, noverifyAll, noverify bool, flashcontentsImage string) (int, error) {
	// TODO(b:247668196) implement

	return 0, nil
}

// FullShutdown shuts down flashrom programmer, cleans up all resources and shuts down flashrom.
func (i *Instance) FullShutdown(ctx context.Context) (int, error) {
	// TODO Implement when switching the library to use libflashrom.
	// Shutdown is called implicitly for command line invocations.

	return 0, nil
}
