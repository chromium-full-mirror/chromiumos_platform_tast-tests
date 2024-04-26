// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	common "go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/dutcontrol"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// A value that is big enough so that console data from the raw uart shouldn't have to be broken up to multiple messages in most cases.
	consoleDataLen = 1024

	// The number of pending console request and responses we can have pending a tast read before
	// running out of space.
	consoleQueueSize = 10000

	// Number of times to attempt start session due to operation failures (ex:  ott communication errors)
	startSessionTrys = 3
)

var (
	// gpioOutput specifies the output format of OpenTitanTool. Quotes are part of the output
	// after we started passing in --format=json flag. The ? for the quotes could be dropped once
	// the newer docker images are used everywhere
	gpioOutput = regexp.MustCompile("\"?value\"?: (true|false)")

	reGsctoolUpdateSuccess  = regexp.MustCompile(`image updated`)
	reGsctoolUpdateNotReady = regexp.MustCompile(`Can't find device`)
	reGsctoolFoundDevice    = regexp.MustCompile(`Found device\.`)
)

// DUTControlAndreiboard controls an Andreiboard through dutcontrol grpc..
type DUTControlAndreiboard struct {
	client dutcontrol.DutControlClient
}

// NewDUTControlAndreiboard creates a DUTControlAndreiboard.
func NewDUTControlAndreiboard(grpcConn *grpc.ClientConn) *DUTControlAndreiboard {
	dutControlClient := dutcontrol.NewDutControlClient(grpcConn)
	return &DUTControlAndreiboard{
		client: dutControlClient,
	}
}

// TestbedProperties states aspects of the testbed controlled by this instance of devboardservice,
// such as what kind of board/chip it has.
type TestbedProperties struct {
	TestbedType common.TestbedType
	UsbSerial   string
}

func writeServerLogs(ctx context.Context, logs string) {
	dir, ok := testing.ContextOutDir(ctx)
	if !ok {
		testing.ContextLog(ctx, "Could not write devboardsvc_logs.txt, no context out dir")
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "devboardsvc_logs.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		testing.ContextLog(ctx, "Could not write devboardsvc_logs.txt: ", err)
		return
	}
	defer f.Close()
	f.WriteString(logs)
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
		UsbSerial:   resp.UsbSerial,
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
	defer writeServerLogs(ctx, resp.ServerLogs)
	if resp.Err != "" {
		return errors.Errorf("Setup operation failed: %s", resp.Err)
	}
	return nil
}

// GSCVersionInfo is version information of the currently running image.
type GSCVersionInfo struct {
	Image     string
	ROVersion string
	RWVersion string
	Branch    string
	Rev       uint32
	SHA       string
}

// GSCVersionInfo returns the GSC version information of the currently running image.
func (a *DUTControlAndreiboard) GSCVersionInfo(ctx context.Context) (ver GSCVersionInfo, err error) {
	req := &dutcontrol.GscVersionRequest{}

	resp, err := a.client.GscVersion(ctx, req)
	if err != nil {
		return GSCVersionInfo{}, errors.Wrap(err, "GscVersion request")
	}
	if resp.Err != "" {
		return GSCVersionInfo{}, errors.Errorf("GscVersion operation failed: %s", resp.Err)
	}
	log.Printf("GSC version info: %v", resp)
	return GSCVersionInfo{
		Image:     resp.Image,
		ROVersion: resp.RoVersion,
		RWVersion: resp.RwVersion,
		Branch:    resp.Branch,
		Rev:       resp.Rev,
		SHA:       resp.Sha,
	}, nil
}

// LogResultTag appends a tag=value line to result_tags.txt.
func (a *DUTControlAndreiboard) LogResultTag(ctx context.Context, tag ti50.ResultTag, value string) error {
	dir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("could not write result_tags.txt, no context out dir")
	}
	f, err := os.OpenFile(filepath.Join(dir, "result_tags.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Wrap(err, "could not write result_tags.txt")
	}
	defer f.Close()
	f.WriteString(fmt.Sprintf("%s=%s\n", tag, value))
	return nil
}

// StartSession will initialize the devboard and debugger to a known state.
func (a *DUTControlAndreiboard) StartSession(ctx context.Context, gpioStrap ti50.GpioStrap) (err error) {
	req := &dutcontrol.StartSessionRequest{}

	req.GpioStrap = string(gpioStrap)

	var resp *dutcontrol.StartSessionResponse
	for i := 0; i < startSessionTrys; i++ {
		testing.ContextLogf(ctx, "StartSession attempt %d/%d", i+1, startSessionTrys)
		resp, err = a.client.StartSession(ctx, req)
		if err != nil {
			err = errors.Wrap(err, "StartSession request")
			return
		}
		if resp.Err == "" {
			return
		}
		err = errors.Errorf("StartSession operation failed: %s", resp.Err)
		testing.ContextLogf(ctx, "StartSession attempt %d/%d: %v", i+1, startSessionTrys, err)
	}
	return
}

// EndSession will tear down host emulation (probably do nothing for devboards).
func (a *DUTControlAndreiboard) EndSession(ctx context.Context) (err error) {
	req := &dutcontrol.EndSessionRequest{}

	resp, err := a.client.EndSession(ctx, req)
	if err != nil {
		return errors.Wrap(err, "EndSession request")
	}
	defer writeServerLogs(ctx, resp.ServerLogs)
	if resp.Err != "" {
		return errors.Errorf("EndSession operation failed: %s", resp.Err)
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
	if _, err := a.PlainCommand(ctx, "gpio", "apply", string(ti50.StrapReset)); err != nil {
		return err
	}
	if _, err := a.PlainCommand(ctx, "gpio", "remove", string(ti50.StrapReset)); err != nil {
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
		return output, errors.Errorf("operation %s: %s: %s", strings.Join(args, " "), resp.Err, output)
	}
	return output, nil
}

// GSCToolWaitUntilReady waits until board is ready to be read by gsctool.
func (a *DUTControlAndreiboard) GSCToolWaitUntilReady(ctx context.Context, timeoutInterval ...time.Duration) error {
	timeout := 10 * time.Second
	interval := time.Second
	if len(timeoutInterval) > 2 {
		return errors.New("can only specify timeout and interval")
	}
	if len(timeoutInterval) > 0 {
		timeout = timeoutInterval[0]
	}
	if len(timeoutInterval) > 1 {
		interval = timeoutInterval[1]
	}
	return testing.Poll(ctx, func(ctx context.Context) error {
		if out, _ := a.GSCToolCommand(ctx, "", "--board_id"); reGsctoolUpdateNotReady.Match(out) {
			return errors.New("gsctool update not ready: " + string(out))
		} else if reGsctoolFoundDevice.Match(out) {
			return nil
		} else {
			return testing.PollBreak(errors.New("gsctool error: " + string(out)))
		}
	}, &testing.PollOptions{Timeout: timeout, Interval: interval})
}

// GSCToolUpdate uses gsctool to update the image.
func (a *DUTControlAndreiboard) GSCToolUpdate(ctx context.Context, i *common.CrOSImage, imagePath string) error {
	out, err := a.GSCToolCommand(ctx, imagePath)
	if reGsctoolUpdateSuccess.Match(out) {
		return i.WaitUntilBooted(ctx)
	}
	return errors.Wrap(err, "gsctool update")
}

// GSCVersion represents the epic.major.minor version tuple of GSC fw.
type GSCVersion [3]int

// Less returns true if version is lower than other version.
func (v GSCVersion) Less(other GSCVersion) bool {
	for i := 0; i < 3; i++ {
		if v[i] < other[i] {
			return true
		} else if v[i] > other[i] {
			return false
		}
	}
	return false
}

func (v GSCVersion) String() string {
	var vs []string
	for _, i := range v {
		vs = append(vs, strconv.Itoa(i))
	}
	return strings.Join(vs, ".")
}

var (
	reBoardFwVersion = regexp.MustCompile(`(?s)Current versions:.*RO ((\d+)\.(\d+)\.(\d+)).*RW ((\d+)\.(\d+)\.(\d+))`)
	reBinVersion     = regexp.MustCompile(`(?s)RO_A:((-?\d+)\.(-?\d+)\.(-?\d+)) RW_A:((-?\d+)\.(-?\d+)\.(-?\d+)).*RO_B:((-?\d+)\.(-?\d+)\.(-?\d+)) RW_B:((-?\d+)\.(-?\d+)\.(-?\d+))`)
)

// GSCToolCurrentFwVersion uses gsctool --fwver to get the RO and RW versions.
func (a *DUTControlAndreiboard) GSCToolCurrentFwVersion(ctx context.Context) (ro, rw GSCVersion, err error) {
	out, err := a.GSCToolCommand(ctx, "", "--fwver")
	if err != nil {
		return
	}

	defer func() {
		if err != nil {
			err = errors.New("unable to parse version: " + string(out))
		}
	}()

	m := reBoardFwVersion.FindSubmatch(out)

	if m == nil {
		err = errors.New("unable to parse version: " + string(out))
		return
	}

	for i := 0; i < 3; i++ {
		ro[i], err = strconv.Atoi(string(m[i+2]))
		if err != nil {
			return
		}
	}

	for i := 0; i < 3; i++ {
		rw[i], err = strconv.Atoi(string(m[i+6]))
		if err != nil {
			return
		}
	}
	return
}

// GSCToolBinVersion uses gsctool --binvers to get the RO and RW versions for both slots.
func (a *DUTControlAndreiboard) GSCToolBinVersion(ctx context.Context, imagePath string) (roa, rwa, rob, rwb GSCVersion, err error) {
	out, err := a.GSCToolCommand(ctx, imagePath, "--binvers")
	if err != nil {
		return
	}

	m := reBinVersion.FindSubmatch(out)

	if m == nil {
		err = errors.New("unable to parse version: " + string(out))
		return
	}

	// extract epoch, major, and minor versions from matched fields using
	// starting offsets and corresponding image regions.
	for offset, v := range map[int]*GSCVersion{2: &roa, 6: &rwa, 10: &rob, 14: &rwb} {
		for i := 0; i < 3; i++ {
			v[i], err = strconv.Atoi(string(m[i+offset]))
			if err != nil {
				err = errors.Wrap(err, "unable to parse version: "+string(out))
				return
			}
		}
	}

	return
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
func (a *DUTControlAndreiboard) PhysicalUart(name common.UartName) common.SerialChannel {
	readTimeout := common.UartTimeoutDefault
	if name == common.UartConsole {
		readTimeout = common.UartConsoleTimeout
	}
	// Record all of the raw uarts as "uart_<NAME>.log" except main console as "gsc.log"
	logName := "uart_" + string(name) + ".log"
	if name == common.UartConsole {
		logName = "gsc.log"
	}
	uartOpener := &DUTControlRawUARTPortOpener{
		Client:      a.client,
		Uart:        string(name),
		Baud:        UartBaud,
		DataLen:     consoleDataLen,
		ReadTimeout: readTimeout,
		LogName:     logName,
	}
	return common.NewBufferedConsole(consoleQueueSize, uartOpener)
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
		LogName:     "ccd_" + string(name) + ".log",
	}
	return common.NewBufferedConsole(consoleQueueSize, uartOpener)
}

// CCDFlashromRead reads the SPI flash chip via CCD.
func (a *DUTControlAndreiboard) CCDFlashromRead(ctx context.Context) (contents []byte, durationMs uint32, err error) {
	var cArgs []*dutcontrol.CommandArg
	cArgs = append(cArgs, &dutcontrol.CommandArg{Type: &dutcontrol.CommandArg_Plain{Plain: "--read"}})
	cArgs = append(cArgs, &dutcontrol.CommandArg{Type: &dutcontrol.CommandArg_ResponseFile{ResponseFile: "read.bin"}})
	req := &dutcontrol.CommandRequest{Args: cArgs}
	resp, err := a.client.FlashromCommand(ctx, req)
	if err != nil {
		return nil, 0, errors.Wrap(err, "CCDFlashromRead request")
	}
	if resp.Err != "" {
		return nil, 0, errors.Errorf("CCDFlashromRead response: %s", resp.Err)
	}
	return resp.Files[0], resp.DurationMs, nil
}

// CCDFlashromWrite writes the SPI flash chip via CCD.
func (a *DUTControlAndreiboard) CCDFlashromWrite(ctx context.Context, contents []byte) (durationMs uint32, err error) {
	var cArgs []*dutcontrol.CommandArg
	cArgs = append(cArgs, &dutcontrol.CommandArg{Type: &dutcontrol.CommandArg_Plain{Plain: "--noverify"}})
	cArgs = append(cArgs, &dutcontrol.CommandArg{Type: &dutcontrol.CommandArg_Plain{Plain: "--write"}})
	cArgs = append(cArgs, &dutcontrol.CommandArg{Type: &dutcontrol.CommandArg_File{File: contents}})
	req := &dutcontrol.CommandRequest{Args: cArgs}
	resp, err := a.client.FlashromCommand(ctx, req)
	if err != nil {
		return 0, errors.Wrap(err, "CCDFlashromWrite request")
	}
	if resp.Err != "" {
		return 0, errors.Errorf("CCDFlashromWrite response: %s", resp.Err)
	}
	return resp.DurationMs, nil
}

// CCDFlashromErase erases the SPI flash chip via CCD.
func (a *DUTControlAndreiboard) CCDFlashromErase(ctx context.Context) (durationMs uint32, err error) {
	var cArgs []*dutcontrol.CommandArg
	cArgs = append(cArgs, &dutcontrol.CommandArg{Type: &dutcontrol.CommandArg_Plain{Plain: "--erase"}})
	req := &dutcontrol.CommandRequest{Args: cArgs}
	resp, err := a.client.FlashromCommand(ctx, req)
	if err != nil {
		return 0, errors.Wrap(err, "CCDFlashromErase request")
	}
	if resp.Err != "" {
		return 0, errors.Errorf("CCDFlashromErase response: %s", resp.Err)
	}
	return resp.DurationMs, nil
}

type andreiboardApFlash struct {
	ab *DUTControlAndreiboard
}

// NewApFlash creates a new object that can be used to interact the AP SPI flash
func NewApFlash(ab *DUTControlAndreiboard) ti50.ApFlash {
	return &andreiboardApFlash{ab: ab}
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

// WriteApFlash writes `contents` to the SPI flash chip connected to the devboard.
func (a *andreiboardApFlash) WriteApFlash(ctx context.Context, contents []byte) error {
	resp, err := a.ab.client.WriteApFlash(ctx, &dutcontrol.WriteApFlashRequest{
		RegionContents: []*dutcontrol.ApFlashRegionContents{
			&dutcontrol.ApFlashRegionContents{
				Contents: contents,
			},
		},
	})
	if err != nil {
		return errors.Wrap(err, "WriteApFlash request")
	}
	if resp.Err != "" {
		return errors.Errorf("WriteApFlash response: %s", resp.Err)
	}
	return nil
}

// EnableApWriteProtect enables HW write protection on the SPI flash chip with the given
// start address and length.
func (a *andreiboardApFlash) EnableApWriteProtect(ctx context.Context, startAddr, len uint32) error {
	resp, err := a.ab.client.EnableApWriteProtect(ctx, &dutcontrol.EnableApWriteProtectRequest{
		Start: startAddr,
		Len:   len,
	})
	if err != nil {
		return errors.Wrap(err, "EnableApWriteProtect request")
	}
	if resp.Err != "" {
		return errors.Errorf("EnableApWriteProtect response: %s", resp.Err)
	}
	return nil
}

// GscUsbI2cInterfaceTransaction write the requested bytes to the GSC I2C USB
// interface and returns the response. This method only supports small data
// sizes for simple interactions.
func (a *DUTControlAndreiboard) GscUsbI2cInterfaceTransaction(ctx context.Context, request []byte) (response []byte, err error) {
	req := &dutcontrol.GscUsbTransactionRequest{Iface: dutcontrol.GscUsbInterface_I2C_USB_INTERFACE, RequestData: request}
	rsp, err := a.client.GscUsbTransaction(ctx, req)
	if err != nil {
		return nil, err
	}

	return rsp.ResponseData, nil
}

// GscUsbSpiInterfaceTransaction write the requested bytes to the GSC SPI USB
// interface and returns the response. This method only supports small data
// sizes for simple interactions. An inner context is used with a 3 second
// timeout since we expect responses quickly.
func (a *DUTControlAndreiboard) GscUsbSpiInterfaceTransaction(ctx context.Context, request []byte) (response []byte, err error) {
	req := &dutcontrol.GscUsbTransactionRequest{Iface: dutcontrol.GscUsbInterface_SPI_USB_INTERFACE, RequestData: request}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(time.Second*3))
	defer cancel()
	rsp, err := a.client.GscUsbTransaction(ctx, req)
	if err != nil {
		return nil, err
	}

	return rsp.ResponseData, nil
}

// GscUsbSpiBridge configures the GSC AP USB interface to bridge to AP flash or
// EC flash. This method needs to be called first before transacting with the
// SPI interface.
func (a *DUTControlAndreiboard) GscUsbSpiBridge(ctx context.Context, operation common.GscUsbSpiBridgeOperation) error {
	var op dutcontrol.GscUsbSpiRequest
	switch operation {
	case common.EnableApSpiBridge:
		op = dutcontrol.GscUsbSpiRequest_ENABLE_AP_GSC_USB_SPI
	case common.EnableEcSpiBridge:
		op = dutcontrol.GscUsbSpiRequest_ENABLE_EC_GSC_USB_SPI
	default:
		op = dutcontrol.GscUsbSpiRequest_DISABLE_GSC_USB_SPI
	}
	req := &dutcontrol.GscUsbApSpiBridgeRequest{Operation: op}
	_, err := a.client.GscUsbApSpiBridge(ctx, req)
	return err
}
