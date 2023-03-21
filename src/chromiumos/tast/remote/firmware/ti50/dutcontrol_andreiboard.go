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

	common "chromiumos/tast/common/firmware/ti50"
	"chromiumos/tast/remote/firmware/ti50/dutcontrol"
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
	client dutcontrol.DutControlClient
	*common.Andreiboard
}

// NewDUTControlAndreiboard creates a DUTControlAndreiboard.
//
// grpcConn should be to a host running the dutcontrol service.
// bufSize should be set to the max outstanding chars waiting to be read.
// readTimeout should be set to the max expected duration between char outputs.
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
	opener := &DUTControlRawUARTPortOpener{
		Client:      dutControlClient,
		Uart:        ConsoleUart,
		Baud:        ConsoleBaud,
		DataLen:     consoleDataLen,
		ReadTimeout: readTimeout,
	}

	ab := common.NewAndreiboard(bufSize, opener, "")
	return &DUTControlAndreiboard{client: dutControlClient, Andreiboard: ab}
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

// FlashImage flashes image at the specified path on localhost to the board.
func (a *DUTControlAndreiboard) FlashImage(ctx context.Context, image string) (err error) {
	imageBytes, err := ioutil.ReadFile(image)
	if err != nil {
		return errors.Wrapf(err, "reading image file %q", image)
	}

	// Close and Re-open the port because opentitantool console occupies the UART that rescue uses.
	wasOpen := a.IsOpen()
	err = a.Close(ctx)
	if err != nil {
		return errors.Wrap(err, "close console before rescue")
	}
	defer func() {
		if wasOpen {
			if e := a.Open(ctx); e != nil && err == nil {
				err = e
			}
		}
	}()

	var args []*dutcontrol.CommandArg
	args = append(args, &dutcontrol.CommandArg{Type: &dutcontrol.CommandArg_Plain{Plain: "-p"}})
	args = append(args, &dutcontrol.CommandArg{Type: &dutcontrol.CommandArg_Plain{Plain: "Rescue"}})
	args = append(args, &dutcontrol.CommandArg{Type: &dutcontrol.CommandArg_File{File: imageBytes}})
	req := &dutcontrol.CommandRequest{Command: "bootstrap", Args: args}

	resp, err := a.client.Command(ctx, req)
	if err != nil {
		return errors.Wrap(err, "bootstrap request")
	}
	if resp.Err != "" {
		return errors.Errorf("bootstrap operation failed: %s, stdout: %s, stderr: %s", resp.Err, resp.Output, resp.ErrOutput)
	}
	return nil
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
func (a *DUTControlAndreiboard) RunTcgTests(ctx context.Context, outdir string, test_suite string) (err error) {
	req := &dutcontrol.RunTcgTestsRequest{Bus: dutcontrol.TpmInterface_SPI, TestSuite: test_suite}
	stream, err := a.client.RunTcgTests(ctx, req)

	test_err, err := os.Create(filepath.Join(outdir, "test_stderr.log"))
	if err != nil {
		return errors.Wrapf(err, "creating log file")
	}
	test_out, err := os.Create(filepath.Join(outdir, "test_stdout.log"))
	if err != nil {
		return errors.Wrapf(err, "creating log file")
	}
	tpm_err, err := os.Create(filepath.Join(outdir, "tpm_server_stderr.log"))
	if err != nil {
		return errors.Wrapf(err, "creating log file")
	}
	tpm_out, err := os.Create(filepath.Join(outdir, "tpm_server_stdout.log"))
	if err != nil {
		return errors.Wrapf(err, "creating log file")
	}
	defer func() {
		test_err.Sync()
		test_out.Sync()
		tpm_err.Sync()
		tpm_out.Sync()

		test_err.Close()
		test_out.Close()
		tpm_err.Close()
		tpm_out.Close()

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
				test_err.WriteString(resp.Logs.TestStderr)
			}
			if resp.Logs.TestStdout != "" {
				log.Printf("%s", resp.Logs.TestStdout)
				test_out.WriteString(resp.Logs.TestStdout)
			}
			if resp.Logs.TpmServerStderr != "" {
				log.Printf("%s", resp.Logs.TpmServerStderr)
				tpm_err.WriteString(resp.Logs.TpmServerStderr)
			}
			if resp.Logs.TpmServerStdout != "" {
				log.Printf("%s", resp.Logs.TpmServerStdout)
				tpm_out.WriteString(resp.Logs.TpmServerStdout)
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
