// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	common "go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/dutcontrol"
	"go.chromium.org/tast/core/errors"
)

const (
	// A value that is big enough so that console data from the raw uart shouldn't have to be broken up to multiple messages in most cases.
	consoleDataLen = 1024
)

var (
	// gpioOutput specifies the output format of OpenTitanTool. Quotes are part of the output
	// after we started passing in --format=json flag. The ? for the quotes could be dropped once
	// the newer docker images are used everywhere
	gpioOutput = regexp.MustCompile("\"?value\"?: (true|false)")
)

// DUTControlAndreiboard controls an Andreiboard through dutcontrol grpc..
type DUTControlAndreiboard struct {
	client     dutcontrol.DutControlClient
	gscConsole *common.BufferedConsole
}

// NewDUTControlAndreiboard creates a DUTControlAndreiboard.
//
// grpcConn should be to a host running the dutcontrol service.
// bufSize should be set to the max outstanding chars waiting to be read.
// readTimeout should be set to the max expected duration between char outputs.
//
//	This option provides several advantages:
//	  1) Reduces the wait time before declaring test failure from a read operation
//	     that does not match an expected pattern.
//	  2) Avoids having to use shortened context timeouts for each individual
//	     read call.
//	  3) Still allows calling code to optimize by setting a shorter timeout
//	     in the context if it knows that a particular operation has a shorter
//	     time bound.
//	  4) Each test or fixture can adjust the timeout to suit the testcase or
//	     board under test.
//	ReadSerialSubmatch will return serial.ErrReadTimeout when this timeout is
//	exceeded.
//
// Example:
// conn, err := grpc.DialContext(ctx, hostPort, grpc.WithInsecure())
//
//	if err != nil {
//	    return nil, err
//	}
//
// defer conn.Close(ctx)
// board := NewDUTControlAndreiboard(conn, 4096, 200 * time.Millisecond)
// defer board.Close(ctx)
func NewDUTControlAndreiboard(grpcConn *grpc.ClientConn, bufSize int, readTimeout time.Duration) *DUTControlAndreiboard {
	dutControlClient := dutcontrol.NewDutControlClient(grpcConn)

	gscOpener := &DUTControlRawUARTPortOpener{
		Client:      dutControlClient,
		Uart:        ConsoleUart,
		Baud:        UartBaud,
		DataLen:     consoleDataLen,
		ReadTimeout: readTimeout,
	}
	gscConsole := common.NewBufferedConsole("gsc.log", bufSize, gscOpener)

	return &DUTControlAndreiboard{
		client:     dutControlClient,
		gscConsole: gscConsole,
	}
}

// TestbedProperties states aspects of the testbed controlled by this instance of devboardservice,
// such as what kind of board/chip it has.
type TestbedProperties struct {
	TestbedType common.TestbedType
}

// Query will return an instance of TestbedProperties, stating aspects of the testbed controlled
// by this instance of devboardservice, such as what kind of board/chip it has.
func (a *DUTControlAndreiboard) Query(ctx context.Context) (props TestbedProperties, err error) {
	resp, err := a.client.Query(ctx, &dutcontrol.QueryRequest{})
	if err != nil {
		return TestbedProperties{}, errors.Wrap(err, "Query request")
	}
	if resp.Err != "" {
		return TestbedProperties{}, errors.Errorf("Query operation failed: %s", resp.Err)
	}
	return TestbedProperties{
		TestbedType: common.TestbedType(resp.TestbedType),
	}, nil
}

// Setup will tell the devboard service which binary image and configuration we want to use.
func (a *DUTControlAndreiboard) Setup(ctx context.Context, image string, fwConfs []string) (err error) {
	req := &dutcontrol.SetupRequest{}
	if image != "" {
		imageBytes, err := ioutil.ReadFile(image)
		if err != nil {
			return errors.Wrapf(err, "reading image file %q", image)
		}
		req.FlashImage = &dutcontrol.File{FileName: image, Contents: imageBytes}
	}

	req.ConfFiles = []*dutcontrol.File{}
	for _, conf := range fwConfs {
		confBytes, err := ioutil.ReadFile(conf)
		if err != nil {
			return errors.Wrapf(err, "reading conf file %q", conf)
		}
		req.ConfFiles = append(req.ConfFiles, &dutcontrol.File{FileName: conf, Contents: confBytes})
	}

	resp, err := a.client.Setup(ctx, req)
	if err != nil {
		return errors.Wrap(err, "Setup request")
	}
	if resp.Err != "" {
		return errors.Errorf("Setup operation failed: %s", resp.Err)
	}
	return nil
}

// StartSession will initialize the devboard and debugger to a known state.
func (a *DUTControlAndreiboard) StartSession(ctx context.Context) (err error) {
	req := &dutcontrol.StartSessionRequest{}

	resp, err := a.client.StartSession(ctx, req)
	if err != nil {
		return errors.Wrap(err, "StartSession request")
	}
	if resp.Err != "" {
		return errors.Errorf("StartSession operation failed: %s", resp.Err)
	}
	return nil
}

// EndSession will tear down host emulation (probably do nothing for devboards).
func (a *DUTControlAndreiboard) EndSession(ctx context.Context) (err error) {
	req := &dutcontrol.EndSessionRequest{}

	resp, err := a.client.EndSession(ctx, req)
	if err != nil {
		return errors.Wrap(err, "EndSession request")
	}
	if resp.Err != "" {
		return errors.Errorf("EndSession operation failed: %s", resp.Err)
	}
	return nil
}

// Open opens the ti50 console.
func (a *DUTControlAndreiboard) Open(ctx context.Context) error {
	return a.gscConsole.Open(ctx)
}

// ReadSerialSubmatch reads gsc console output from port until regex is matched.
func (a *DUTControlAndreiboard) ReadSerialSubmatch(ctx context.Context, re *regexp.Regexp) (output [][]byte, err error) {
	return a.gscConsole.ReadSerialSubmatch(ctx, re)
}

// WriteSerial writes to gsc console.
func (a *DUTControlAndreiboard) WriteSerial(ctx context.Context, bytes []byte) error {
	return a.gscConsole.WriteSerial(ctx, bytes)
}

// ClearInput clears any pending input that hasn't been read yet.
func (a *DUTControlAndreiboard) ClearInput(ctx context.Context) error {
	return a.gscConsole.ClearInput(ctx)
}

// Close closes the gsc consoles.
func (a *DUTControlAndreiboard) Close(ctx context.Context) error {
	return a.gscConsole.Close(ctx)
}

// PlainCommand executes a opentitantool subcommand that uses no file arguments.
func (a *DUTControlAndreiboard) PlainCommand(ctx context.Context, cmd string, args ...string) (output []byte, err error) {
	var cArgs []*dutcontrol.CommandArg
	for _, a := range args {
		cArgs = append(cArgs, &dutcontrol.CommandArg{Type: &dutcontrol.CommandArg_Plain{Plain: a}})
	}
	req := &dutcontrol.CommandRequest{Command: cmd, Args: cArgs}
	resp, err := a.client.Command(ctx, req)
	if err != nil {
		return nil, errors.Wrapf(err, "PlainCommand: %s %s", cmd, strings.Join(args, " "))
	}
	if resp.Err != "" {
		return resp.Output, errors.Errorf("operation %s %s: %s, stderr: %s", cmd, strings.Join(args, " "), resp.Err, resp.ErrOutput)
	}
	return resp.Output, nil
}

// OpenTitanToolCommand runs an arbitrary OpenTitan tool command (without up-/downloading any files).
func (a *DUTControlAndreiboard) OpenTitanToolCommand(ctx context.Context, cmd string, args ...string) (output map[string]interface{}, err error) {
	data, err := a.PlainCommand(ctx, cmd, args...)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return make(map[string]interface{}), nil
	}
	var val map[string]interface{}
	err = json.Unmarshal(data, &val)
	return val, err
}

// Reset the chip by asking opentitantool to toggle the reset pin.
func (a *DUTControlAndreiboard) Reset(ctx context.Context) error {
	if _, err := a.PlainCommand(ctx, "gpio", "write", "RESET", "false"); err != nil {
		return err
	}
	if _, err := a.PlainCommand(ctx, "gpio", "write", "RESET", "true"); err != nil {
		return err
	}
	return nil
}

// GSCToolCommand executes gsctool via the DutControl service.
func (a *DUTControlAndreiboard) GSCToolCommand(ctx context.Context, image string, args ...string) (output []byte, err error) {
	var cArgs []*dutcontrol.CommandArg
	for _, a := range args {
		cArgs = append(cArgs, &dutcontrol.CommandArg{Type: &dutcontrol.CommandArg_Plain{Plain: a}})
	}
	if image != "" {
		imageBytes, err := ioutil.ReadFile(image)
		if err != nil {
			return nil, errors.Wrapf(err, "reading image file %q", image)
		}
		cArgs = append(cArgs, &dutcontrol.CommandArg{Type: &dutcontrol.CommandArg_File{File: imageBytes}})
	}
	req := &dutcontrol.CommandRequest{Args: cArgs}
	resp, err := a.client.GSCToolCommand(ctx, req)
	if err != nil {
		return nil, errors.Wrapf(err, "request %s", strings.Join(args, " "))
	}
	output = append(resp.Output, resp.ErrOutput...)
	if resp.Err != "" {
		return output, errors.Errorf("operation %s: %s", strings.Join(args, " "), resp.Err)
	}
	return output, nil
}

// RunTcgTests executes TCG tests via the DutControl service.
func (a *DUTControlAndreiboard) RunTcgTests(ctx context.Context, outdir, testSuite string) (err error) {
	req := &dutcontrol.RunTcgTestsRequest{Bus: dutcontrol.TpmInterface_SPI, TestSuite: testSuite}
	stream, err := a.client.RunTcgTests(ctx, req)

	testErr, err := os.Create(filepath.Join(outdir, "test_stderr.log"))
	if err != nil {
		return errors.Wrap(err, "creating log file")
	}
	testOut, err := os.Create(filepath.Join(outdir, "test_stdout.log"))
	if err != nil {
		return errors.Wrap(err, "creating log file")
	}
	tpmErr, err := os.Create(filepath.Join(outdir, "tpm_server_stderr.log"))
	if err != nil {
		return errors.Wrap(err, "creating log file")
	}
	tpmOut, err := os.Create(filepath.Join(outdir, "tpm_server_stdout.log"))
	if err != nil {
		return errors.Wrap(err, "creating log file")
	}
	defer func() {
		testErr.Sync()
		testOut.Sync()
		tpmErr.Sync()
		tpmOut.Sync()

		testErr.Close()
		testOut.Close()
		tpmErr.Close()
		tpmOut.Close()

	}()

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		switch resp := resp.Response.(type) {
		case *dutcontrol.RunTcgTestsResponse_Logs:
			if resp.Logs.TestStderr != "" {
				log.Printf("%s", resp.Logs.TestStderr)
				testErr.WriteString(resp.Logs.TestStderr)
			}
			if resp.Logs.TestStdout != "" {
				log.Printf("%s", resp.Logs.TestStdout)
				testOut.WriteString(resp.Logs.TestStdout)
			}
			if resp.Logs.TpmServerStderr != "" {
				log.Printf("%s", resp.Logs.TpmServerStderr)
				tpmErr.WriteString(resp.Logs.TpmServerStderr)
			}
			if resp.Logs.TpmServerStdout != "" {
				log.Printf("%s", resp.Logs.TpmServerStdout)
				tpmOut.WriteString(resp.Logs.TpmServerStdout)
			}
		case *dutcontrol.RunTcgTestsResponse_Results:
			for _, file := range resp.Results.Results {
				log.Printf("Writing result file %s", file.FileName)
				path := filepath.Join(outdir, file.FileName)
				os.WriteFile(path, file.Contents, 0644)
			}
			if resp.Results.Err != "" {
				return errors.Errorf("Tests failed: %s", resp.Results.Err)
			}
		}
	}

	return nil
}

// PhysicalUart opens a handle for communication to/from a physical UART on the chip under test.
func (a *DUTControlAndreiboard) PhysicalUart(name common.UartName, readTimeout time.Duration) common.SerialChannel {
	uartOpener := &DUTControlRawUARTPortOpener{
		Client:      a.client,
		Uart:        string(name),
		Baud:        UartBaud,
		DataLen:     consoleDataLen,
		ReadTimeout: readTimeout,
	}
	return common.NewBufferedConsole("", 2048, uartOpener)
}

// CcdSerialInterface opens a handle for communication to/from a USB interface on the chip under
// test.
func (a *DUTControlAndreiboard) CcdSerialInterface(name common.UartName, readTimeout time.Duration) common.SerialChannel {
	var ep dutcontrol.CCDSerialEndPoint
	switch name {
	case common.UartAP:
		ep = dutcontrol.CCDSerialEndPoint_AP
		break
	case common.UartEC:
		ep = dutcontrol.CCDSerialEndPoint_EC
		break
	case common.UartFPMCU:
		ep = dutcontrol.CCDSerialEndPoint_FPMCU
		break
	default:
		ep = dutcontrol.CCDSerialEndPoint_UNKNOWN_SERIAL
		break
	}
	uartOpener := &DUTControlCCDPortOpener{
		Client:      a.client,
		Ep:          ep,
		Baud:        UartBaud,
		DataLen:     consoleDataLen,
		ReadTimeout: readTimeout,
	}
	return common.NewBufferedConsole("", 2048, uartOpener)
}

type andreiboardApFlash struct {
	ab *DUTControlAndreiboard
}

// WithApFlashAccess runs `f` with the proper setup and teardown to access the SPI flash chip.
// This function asserts the SuzyQ strapping and leaves it in that state, so `gsctool` should work immediately.
func (a *DUTControlAndreiboard) WithApFlashAccess(ctx context.Context, holdReset ti50.HoldReset, f func(ti50.ApFlash)) error {
	i, err := ti50.NewCrOSImage(ctx, a)
	if err != nil {
		return err
	}
	if _, err = a.PlainCommand(ctx, "gpio", "apply", string(ti50.CcdSuzyQ)); err != nil {
		return err
	}
	if err := i.WaitUntilMatch(ctx, regexp.MustCompile(`USB:\s+Connected`), 20*time.Second); err != nil {
		return errors.Wrap(err, "expected to see Ti50 connect CCD USB")
	}
	flash := &andreiboardApFlash{ab: a}
	if _, err := flash.FetchApFlashInfo(ctx); err != nil {
		return err
	}
	// TODO(kupiakos): consider warning/erroring if an unrecognized chip is seen
	// on the andreishield, or pass this info to `f` so it can check itself.
	f(flash)
	// Reset the GSC
	if _, err := a.PlainCommand(ctx, "gpio", "write", "RESET", "false"); err != nil {
		return err
	}
	if !holdReset {
		// Turn the GSC back on
		if _, err := a.PlainCommand(ctx, "gpio", "write", "RESET", "true"); err != nil {
			return err
		}
		if err := i.WaitUntilBooted(ctx); err != nil {
			return err
		}
	}
	return nil
}

// FetchApFlashInfo fetches the name and vendor of the SPI flash chip connected to the devboard.
func (a *andreiboardApFlash) FetchApFlashInfo(ctx context.Context) (info *common.ApFlashInfo, err error) {
	resp, err := a.ab.client.GetApFlashInfo(ctx, &dutcontrol.GetApFlashInfoRequest{})
	if err != nil {
		return nil, errors.Wrap(err, "GetApFlashInfo request")
	}
	if resp.Err != "" {
		return nil, errors.Errorf("GetApFlashInfo response: %s", resp.Err)
	}
	return &common.ApFlashInfo{Name: resp.ChipName, Vendor: resp.ChipVendor}, nil
}
