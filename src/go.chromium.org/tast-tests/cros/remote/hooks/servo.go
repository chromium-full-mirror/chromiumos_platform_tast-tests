// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hooks contains code for support adding custom hooks to root fixture.
package hooks

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"

	"go.chromium.org/chromiumos/config/go/test/api"
	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
	"go.chromium.org/chromiumos/infra/proto/go/satlabrpcserver"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/servers"
	"go.chromium.org/tast-tests/cros/common/servo"
)

func init() {
	addHook(&Hook{
		Name:         "servoHook",
		Desc:         "Check servo health and download servo logs",
		Contacts:     []string{"tast-core@google.com", "seewaifu@google.com"},
		BugComponent: "b:1034754", // ChromeOS > Test > Harness > Tast > Framework
		Impl:         &servoHook{},
	})
}

const (
	maxLogSize = 20 * 1024 * 1024 //20mb
	sshTimeout = 10 * time.Second // max time for establishing SSH connection
)

// SetUp extracts servo information from runtime variables, make sure servo
// is running, set up connection with servo.
func (h *servoHook) SetUp(ctx context.Context, s *HookState) error {
	h.servoHost = func() string {
		if servoHost, err := servers.Server(servers.Servo, ""); err == nil && servoHost != "" {
			return servoHost
		}
		// Remove after we replace the use of the variable "servo".
		if servoHost, ok := s.Var("servo"); ok {
			return servoHost
		}
		return ""
	}()
	dut := s.DUT()
	if h.servoHost == "" {
		return nil
	}
	dt, err := s.ChromeOSDUTLabConfig("")
	if err != nil {
		testing.ContextLog(ctx, "DUT Topology is not available")
	}
	h.dutTopology = dt

	h.keyFile = dut.KeyFile()
	h.keyDir = dut.KeyDir()
	testing.ContextLog(ctx, "servoHook Setup")

	connInfo, err := servo.SplitHostPort(h.servoHost)
	if err != nil {
		return errors.Wrapf(err, "cannot parse servo host port from %q", h.servoHost)
	}

	if connInfo.DockerContainer != "" {
		h.connector, err = newContainerConnector(ctx, h.servoHost, h.keyFile,
			h.keyDir, h.dutTopology, connInfo)
	} else if isLabStation(connInfo) {
		h.connector, err = newSSHConnector(ctx, h.servoHost, h.keyFile,
			h.keyDir, h.dutTopology, connInfo)
	} else {
		h.connector = nil
		testing.ContextLog(ctx, "Do not support localhost servo")
		return nil
	}
	if err != nil {
		h.connector = nil
		testing.ContextLog(ctx, "Failed to connect to servo: ", err)
		return nil
	}

	// If we can get a new proxy, we know servod is running.
	if err := h.connector.startServo(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to connect to servod: ", err)
	}

	return nil
}

// Reset does not do anything in this hook.
func (h *servoHook) Reset(ctx context.Context) error {
	return nil
}

// PreTest will check if start servo if servo is not running.
func (h *servoHook) PreTest(ctx context.Context, s *HookTestState) error {
	if h.connector == nil {
		return nil
	}
	// Make sure servo is running.
	if err := h.connector.startServo(ctx); err != nil {
		testing.ContextLogf(ctx, "Failed to connect to servod before running test %s: %v",
			s.TestName(), err)
	}
	return nil
}

// PostTest will check if servo is running.
func (h *servoHook) PostTest(ctx context.Context, s *HookTestState) error {
	if h.connector == nil {
		return nil
	}
	if !h.connector.servoRunning(ctx) {
		testing.ContextLogf(ctx, "Failed to connect to servod after running test %s", s.TestName())
	}
	return nil
}

// TearDown will be called during root fixture TearDown.
func (h *servoHook) TearDown(ctx context.Context, s *HookState) error {
	if h.connector == nil {
		return nil
	}
	testing.ContextLog(ctx, "servoHook TearDown")
	h.connector.collectLogs(ctx, s.OutDir())
	h.connector.cleanup(ctx)
	return nil
}

// servoHook check servo health and download servo logs.
type servoHook struct {
	servoHost   string
	keyFile     string
	keyDir      string
	dutTopology *labapi.Dut
	connector   connector
}

type connector interface {
	startServo(ctx context.Context) error
	servoRunning(ctx context.Context) bool
	cleanup(ctx context.Context)
	collectLogs(ctx context.Context, dst string) error
}

type sshConnector struct {
	servoHost   string
	keyFile     string
	keyDir      string
	dutTopology *labapi.Dut
	connInfo    *servo.ConnectInfo
	hst         *ssh.Conn
	proxy       *servo.Proxy
}

func newSSHConnector(ctx context.Context, servoHost, keyFile, keyDir string,
	dutTopology *labapi.Dut, connInfo *servo.ConnectInfo) (*sshConnector, error) {
	sshPort := connInfo.ServoSSHPort
	host := connInfo.Hostname

	// If the servod instance isn't running locally, assume that we need to connect to it via SSH.
	isLabStation := sshPort > 0 && ((host != "localhost" && host != "127.0.0.1" && host != "::1") || sshPort != 22)
	if !isLabStation {
		return nil, errors.New("do not support localhost servo")
	}

	sopt := ssh.Options{
		KeyFile:        keyFile,
		KeyDir:         keyDir,
		ConnectTimeout: sshTimeout,
		WarnFunc: func(msg string) {
			testing.ContextLog(ctx, msg)
		},
		Hostname: net.JoinHostPort(host, fmt.Sprint(sshPort)),
		User:     "root",
	}

	testing.ContextLogf(ctx, "Opening Servo SSH connection to %s", sopt.Hostname)
	hst, err := ssh.New(ctx, &sopt)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open SSH connection to labstation")
	}

	return &sshConnector{
		servoHost:   servoHost,
		keyFile:     keyFile,
		keyDir:      keyDir,
		dutTopology: dutTopology,
		connInfo:    connInfo,
		hst:         hst,
	}, nil
}

func (sc *sshConnector) startServo(ctx context.Context) error {
	if sc == nil {
		return nil
	}
	hst := sc.hst
	port := sc.connInfo.ServoPort
	sshPort := sc.connInfo.ServoSSHPort
	host := sc.connInfo.Hostname

	inUseFile := fmt.Sprintf("/var/lib/servod/%d_in_use", port)
	if out, err := hst.CommandContext(ctx, "touch", inUseFile).CombinedOutput(); err != nil {
		testing.ContextLogf(ctx, "Failed to touch %s: %s: %v", inUseFile, string(out), err)
	}
	if sc.servoRunning(ctx) {
		testing.ContextLog(ctx, "Servo has already been running")
		return nil
	}

	testing.ContextLog(ctx, "Attempt to start servod")
	args := []string{"servod"}
	args = append(args, fmt.Sprintf("PORT=%d", port))
	if board := sc.dutTopology.GetChromeos().GetDutModel().GetBuildTarget(); board != "" {
		args = append(args, fmt.Sprintf("BOARD=%s", board))
	}
	if model := sc.dutTopology.GetChromeos().GetDutModel().GetModelName(); model != "" {
		args = append(args, fmt.Sprintf("MODEL=%s", model))
	}
	if serial := sc.dutTopology.GetChromeos().GetServo().GetSerial(); serial != "" {
		args = append(args, fmt.Sprintf("SERIAL=%s", serial))
	} else if serial := sc.dutTopology.GetDevboard().GetServo().GetSerial(); serial != "" {
		args = append(args, fmt.Sprintf("SERIAL=%s", serial))
	}
	if out, err := hst.CommandContext(ctx, "start", args...).CombinedOutput(); err != nil {
		// Don't return error so that we can log servod logs later.
		testing.ContextLogf(ctx, "Failed to start servod: %s: %v", string(out), err)
		return nil
	}
	testing.ContextLogf(ctx, "Started servod at port %d at servo host %s:%d", port, host, sshPort)
	// Provide servod up to 120 second to prepare, otherwise it will time out.
	testing.ContextLog(ctx, "Wait for servod to be ready")
	if out, err := hst.CommandContext(ctx, "servodtool", "instance", "wait-for-active", "--timeout", "120", "-p", strconv.Itoa(port)).Output(); err != nil {
		// Don't return error so that we can log servod logs later.
		testing.ContextLogf(ctx, "Failed to check if servod is ready: %s: %v", string(out), err)
	}

	return nil
}

func (sc *sshConnector) servoRunning(ctx context.Context) bool {
	if sc == nil {
		return false
	}
	if sc.proxy != nil {
		return proxyRunning(ctx, sc.proxy)
	}
	var err error
	sc.proxy, err = servo.NewProxy(ctx, sc.servoHost, sc.keyFile, sc.keyDir)
	if err == nil {
		// if we can create a proxy, it means that the servod is running.
		return true
	}
	return false
}

func (sc *sshConnector) servoPort() int {
	return sc.connInfo.ServoPort
}

// collectLogs downloads servo logs to the dest directory.
func (sc *sshConnector) collectLogs(ctx context.Context, dst string) (retErr error) {
	// Tast will download log from labstation. Don't need to do anything here.
	return nil
}

func (sc *sshConnector) cleanup(ctx context.Context) {
	if sc.proxy != nil {
		sc.proxy.Close(ctx)
		sc.proxy = nil
	}
	inUseFile := fmt.Sprintf("/var/lib/servod/%d_in_use", sc.servoPort())
	cmd := sc.hst.CommandContext(ctx, "rm", inUseFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		testing.ContextLogf(ctx, "Failed to remove %s from %s: %s: %v",
			inUseFile, sc.connInfo.Hostname, out, err)
	}
}

type containerConnector struct {
	servoHost   string
	keyFile     string
	keyDir      string
	dutTopology *labapi.Dut
	connInfo    *servo.ConnectInfo
	proxy       *servo.Proxy
}

func newContainerConnector(ctx context.Context, servoHost, keyFile, keyDir string,
	dutTopology *labapi.Dut, connInfo *servo.ConnectInfo) (*containerConnector, error) {

	return &containerConnector{
		servoHost:   servoHost,
		keyFile:     keyFile,
		keyDir:      keyDir,
		dutTopology: dutTopology,
		connInfo:    connInfo,
	}, nil
}

func (cc *containerConnector) startServo(ctx context.Context) (err error) {
	if cc == nil {
		return nil
	}
	if proxyRunning(ctx, cc.proxy) {
		testing.ContextLog(ctx, "Servo has already been running")
		return nil
	}
	cc.proxy, err = servo.NewProxy(ctx, cc.servoHost, cc.keyFile, cc.keyDir)
	if err == nil {
		// if we can create a proxy, it means that the container is running.
		testing.ContextLog(ctx, "Servo has already been running")
		return nil
	}
	testing.ContextLog(ctx, "Start Docker servod container via Satlab RPC server")
	conn, err := grpc.Dial(testing.SatlabRPCServer, grpc.WithInsecure())
	if err != nil {
		return errors.Wrap(err, "failed to connect to servo container")
	}
	c := satlabrpcserver.NewSatlabRpcServiceClient(conn)
	if _, err = c.StartServod(ctx,
		&api.StartServodRequest{ServodDockerContainerName: cc.connInfo.DockerContainer}); err != nil {
		return err
	}
	cc.proxy, err = servo.NewProxy(ctx, cc.servoHost, cc.keyFile, cc.keyDir)
	if err != nil {
		testing.ContextLogf(ctx, "Failed to create proxy with container %s: %v", cc.servoHost, err)
	}
	return nil
}

func (cc *containerConnector) getFile(ctx context.Context, src, dst string, startLine int64) error {
	if !proxyRunning(ctx, cc.proxy) {
		return errors.New("cannot get file because no proxy is running")
	}
	return cc.proxy.GetFile(ctx, false, src, dst)
}

func (cc *containerConnector) servoRunning(ctx context.Context) bool {
	if cc == nil {
		return false
	}
	return proxyRunning(ctx, cc.proxy)
}

func (cc *containerConnector) servoPort() int {
	return cc.connInfo.ServoPort
}

func (cc *containerConnector) cleanup(ctx context.Context) {
	if cc.proxy != nil {
		cc.proxy.Close(ctx)
		cc.proxy = nil
	}
}

// collectLogs downloads servo logs to the dest directory.
func (cc *containerConnector) collectLogs(ctx context.Context, dst string) (retErr error) {
	if cc == nil {
		return nil
	}
	servodLogDir := fmt.Sprintf("/var/log/servod_%d", cc.servoPort())

	testing.ContextLog(ctx, "Collecting servo logs from ", servodLogDir)
	defer testing.ContextLog(ctx, "Done collecting servo logs from ", servodLogDir)

	servoHostDestDir := filepath.Join(dst, fmt.Sprintf("servo_host_%s", cc.servoHost))

	destDir := filepath.Join(servoHostDestDir, fmt.Sprintf("servod_%d", cc.servoPort()))
	if err := os.MkdirAll(destDir, 0755); err != nil {
		testing.ContextLogf(ctx, "Failed to create dir %s for downloading servo logs: %v", destDir, err)
	}

	// There are no /var/log/message and servo startup log in
	// a container so we will not download.

	// Getting servod logs.
	cc.downloadServodLogs(ctx, servodLogDir, destDir)

	// Getting dmesg.
	cc.downloadServodDMesgLogs(ctx, servoHostDestDir)

	// Extract MCU logs from latest.DEBUG.
	extractServodMCULogs(ctx, destDir)

	return nil
}

func (cc *containerConnector) downloadServodLogs(ctx context.Context, servodLogDir, destDir string) {
	fn := "latest.DEBUG"
	src := filepath.Join(servodLogDir, fn)
	dst := filepath.Join(destDir, fn)

	out, err := cc.proxy.OutputCommand(ctx, false, "realpath", src)
	if err != nil {
		testing.ContextLogf(ctx, "Failed to get real servo log path %s: %v", fn, err)
		return
	}
	realpath := strings.TrimSpace(string(out))
	testing.ContextLog(ctx, "Saving servo log ", realpath)
	if err := cc.proxy.GetFile(ctx, false, realpath, dst); err != nil {
		testing.ContextLogf(ctx, "Failed to servod log %s: %v", src, err)
	}
}

func (cc *containerConnector) downloadServodDMesgLogs(ctx context.Context, servoHostDestDir string) {
	testing.ContextLog(ctx, "Saving dmesg log")
	out, err := cc.proxy.OutputCommand(ctx, false, "dmesg", "-H")
	if err != nil {
		testing.ContextLogf(ctx, "Failed to start dmesg for host %s: %v", cc.connInfo.Hostname, err)
		return
	}
	dmesgFile := filepath.Join(servoHostDestDir, "dmesg")
	dmesgOut, err := os.Create(dmesgFile)
	if err != nil {
		testing.ContextLog(ctx, "Failed to create servo log dmesg: ", err)
		return
	}
	defer dmesgOut.Close()
	if _, err := dmesgOut.Write(out); err != nil {
		testing.ContextLog(ctx, "Failed to write dmesg to file: ", err)
	}
}

func isLabStation(c *servo.ConnectInfo) bool {
	return c.ServoSSHPort > 0 &&
		((c.Hostname != "localhost" && c.Hostname != "127.0.0.1" && c.Hostname != "::1") || c.ServoSSHPort != 22)
}

// extractServodMCULogs extract MCU logs from latest.DEBUG.
func extractServodMCULogs(ctx context.Context, destDir string) {
	testing.ContextLog(ctx, "Extracing servod MCU logs")
	mcuFiles := make(map[string]*os.File)
	src := filepath.Join(destDir, "latest.DEBUG")
	f, err := os.Open(src)
	if err != nil {
		testing.ContextLogf(ctx, "Failed to open %s: %v", src, err)
		return
	}
	defer f.Close()

	regExpr := `(?P<time>[\d\-]+ [\d:,]+ )` +
		`- (?P<mcu>[\w/]+) - ` +
		`EC3PO\.Console[\s\-\w\d:.]+LogConsoleOutput - /dev/pts/\d+ - ` +
		`(?P<line>.+$)`

	re, err := regexp.Compile(regExpr)
	if err != nil {
		fmt.Printf("Fail in compiling expression %v\n", err)
		return
	}

	sc := bufio.NewScanner(f)
	sc.Split(bufio.ScanLines)
	for sc.Scan() {
		text := sc.Text()
		matches := re.FindStringSubmatch(text)
		timeIndex := re.SubexpIndex("time")
		if timeIndex < 0 || timeIndex >= len(matches) {
			continue
		}
		mcuIndex := re.SubexpIndex("mcu")
		if mcuIndex < 0 || mcuIndex >= len(matches) {
			continue
		}
		lineIndex := re.SubexpIndex("line")
		if lineIndex < 0 || lineIndex >= len(matches) {
			continue
		}
		timestamp := matches[timeIndex]
		mcu := strings.ToLower(matches[mcuIndex])
		line := matches[lineIndex]
		mcuFile, ok := mcuFiles[mcu]
		if !ok {
			mcuFile, err = os.Create(filepath.Join(destDir, fmt.Sprintf("%s.txt", mcu)))
			if err != nil {
				testing.ContextLogf(ctx, "Failed to create servo log %s.txt: %v", mcu, err)
				mcuFiles[mcu] = nil
				continue
			}
			mcuFiles[mcu] = mcuFile
			defer mcuFile.Close()
		}
		if mcuFile == nil {
			continue
		}
		fmt.Fprintln(mcuFile, timestamp, "- ", line)
	}
}

func proxyRunning(ctx context.Context, proxy *servo.Proxy) bool {
	if proxy == nil || proxy.Servo() == nil {
		return false
	}
	if _, err := proxy.OutputCommand(ctx, false, "echo"); err != nil {
		return false
	}
	return true
}
