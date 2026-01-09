// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hooks

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"

	"go.chromium.org/chromiumos/config/go/test/api"
	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"
	"go.chromium.org/chromiumos/infra/proto/go/satlabrpcserver"

	"go.chromium.org/tast-tests/cros/common/servers"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
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
	if h.servoHost == "" {
		return nil
	}
	dt, err := s.ChromeOSDUTLabConfig("")
	if err != nil {
		testing.ContextLog(ctx, "DUT Topology is not available")
	}
	h.dutTopology = dt

	h.keyFile = s.KeyFile()
	h.keyDir = s.KeyDir()
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
	// Tell labStation that servo is in-use.
	h.connector.markServoInUse(ctx)
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
	markServoInUse(ctx context.Context)
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

func (sc *sshConnector) markServoInUse(ctx context.Context) {
	if sc == nil {
		return
	}
	hst := sc.hst
	port := sc.connInfo.ServoPort

	inUseFile := fmt.Sprintf("/var/lib/servod/%d_in_use", port)
	if out, err := hst.CommandContext(ctx, "touch", inUseFile).CombinedOutput(); err != nil {
		testing.ContextLogf(ctx, "Failed to touch %s: %s: %v", inUseFile, string(out), err)
	}
}

func (sc *sshConnector) startServo(ctx context.Context) error {
	if sc == nil {
		return nil
	}
	hst := sc.hst
	port := sc.connInfo.ServoPort
	sshPort := sc.connInfo.ServoSSHPort
	host := sc.connInfo.Hostname

	sc.markServoInUse(ctx)

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
	return err == nil
}

func (sc *sshConnector) servoPort() int {
	return sc.connInfo.ServoPort
}

// collectLogs downloads servo logs to the dest directory.
func (sc *sshConnector) collectLogs(ctx context.Context, dst string) (retErr error) {
	// Tast will download log from labstation.
	writeServoInfoLog(ctx, sc.proxy.Servo(), dst)
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
	servoHost          string
	keyFile            string
	keyDir             string
	dutTopology        *labapi.Dut
	connInfo           *servo.ConnectInfo
	proxy              *servo.Proxy
	currentLogFileName string
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

func (*containerConnector) markServoInUse(ctx context.Context) {
	// Servo in container does not in-use file.
}

func (cc *containerConnector) startServo(ctx context.Context) (err error) {
	if cc == nil {
		return nil
	}

	defer func() {
		if err != nil {
			// Don't try to get log file name if there was an error starting servo.
			return
		}
		if cc.currentLogFileName != "" {
			return
		}
		if cc.proxy == nil {
			testing.ContextLog(ctx, "Proxy not available to get current log file name")
			return
		}
		port := cc.connInfo.ServoPort
		logLink := fmt.Sprintf("/var/log/servod_%d/latest.DEBUG", port)
		out, err := cc.proxy.OutputCommand(ctx, false, "realpath", logLink)
		if err != nil {
			testing.ContextLogf(ctx, "Failed to get realpath for %s: %v", logLink, err)
		} else {
			cc.currentLogFileName = filepath.Base(strings.TrimSpace(string(out)))
		}
	}()

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

	writeServoInfoLog(ctx, cc.proxy.Servo(), dst)

	return nil
}

func (cc *containerConnector) downloadServodLogs(ctx context.Context, servodLogDir, destDir string) {
	if cc == nil || cc.proxy == nil {
		testing.ContextLog(ctx, "Proxy not available, cannot download servod logs")
		return
	}

	if cc.currentLogFileName == "" {
		testing.ContextLog(ctx, "currentLogFileName is empty, falling back to downloading latest.DEBUG with GetFileTail")
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
			testing.ContextLogf(ctx, "Failed to get servod log %s: %v", src, err)
		}
		return
	}

	// Find all files in the servod log directory.
	findCmd := fmt.Sprintf("find %s -maxdepth 1 -type f -printf '%%f\\n'", shutil.Escape(servodLogDir))
	out, stderr, err := cc.proxy.SeparatedOutputCommand(ctx, false, "sh", "-c", findCmd)
	if err != nil {
		testing.ContextLogf(ctx, "Failed to list files in %s (stderr: %q): %v", servodLogDir, string(stderr), err)
		return
	}
	allFiles := strings.Split(strings.TrimSpace(string(out)), "\n")

	// For containerConnector, we don't track InitialRotatedCount yet (can be added if needed later, similar to sshConnector/HostInfo).
	// Passing 0 for InitialRotatedCount.
	filesToDownload := getRelevantLogs(allFiles, cc.currentLogFileName, 0)

	if len(filesToDownload) == 0 {
		testing.ContextLog(ctx, "No specific log files found to process, falling back to downloading latest.DEBUG")
		src := filepath.Join(servodLogDir, "latest.DEBUG")
		dest := filepath.Join(destDir, "latest.DEBUG")
		if err := cc.proxy.GetFile(ctx, false, src, dest); err != nil {
			testing.ContextLogf(ctx, "Failed to get servod log %s: %v", src, err)
		}
		return
	}

	testing.ContextLogf(ctx, "Concatenating the following servo log files: %s", strings.Join(filesToDownload, ", "))
	var remotePaths []string
	for _, f := range filesToDownload {
		remotePaths = append(remotePaths, shutil.Escape(filepath.Join(servodLogDir, f)))
	}

	out, stderr, err = cc.proxy.SeparatedOutputCommand(ctx, false, "mktemp")
	if err != nil {
		testing.ContextLogf(ctx, "Failed to create remote temp file (stderr: %q): %v", string(stderr), err)
		return
	}
	remoteTempPath := strings.TrimSpace(string(out))
	defer cc.proxy.RunCommand(ctx, false, "rm", remoteTempPath)

	catCmdStr := fmt.Sprintf("cat %s > %s", strings.Join(remotePaths, " "), shutil.Escape(remoteTempPath))
	if out, stderr, err := cc.proxy.SeparatedOutputCommand(ctx, false, "sh", "-c", catCmdStr); err != nil {
		// Log error but continue, as some logs might have been concatenated successfully.
		testing.ContextLogf(ctx, "Concatenating remote log files with command `%s` failed with output %q (stderr: %q): %v", catCmdStr, string(out), string(stderr), err)
	}

	destPath := filepath.Join(destDir, "latest.DEBUG")
	testing.ContextLog(ctx, "Saving concatenated servo log to ", destPath)
	if err := cc.proxy.GetFile(ctx, false, remoteTempPath, destPath); err != nil {
		testing.ContextLogf(ctx, "Failed to get concatenated servod log from %s: %v", remoteTempPath, err)
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

	regExpr := `(?P<time>[\d\-]+(( [\d:,]+ )|(T[\d:.+]+ )))` +
		`- (?P<mcu>[\w/]+) - ` +
		`EC3PO\.Console[\s\-\w\d:.]+(LogConsoleOutput|log_console_output) - /dev/pts/\d+ - ` +
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

type servoInfo struct {
	ServodVersion string `json:"servod_version"`
	ServoType     string `json:"servo_type"`
}

func writeServoInfoLog(ctx context.Context, servo *servo.Servo, outDir string) {
	testing.ContextLog(ctx, "Writing servo_info.json")

	if err := os.MkdirAll(outDir, 0755); err != nil {
		testing.ContextLog(ctx, "Failed to create dir: ", err)
	}

	servodVersion, err := servo.GetServodVersion(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get servod version: ", err)
	}
	// The servod version output is multiple lines:
	//   v1.0.2382+643d2b40
	//   Date: 2024-10-02 11:54:12
	//   Builder: 613dcd9ba833
	//   Hash: +643d2b40
	//   Branch: hdctools-release-1024.1
	// Reduce this to just version and date: "v1.0.2382+643d2b40 2024-10-02 11:54:12"
	verRE := regexp.MustCompile(`^(v\S+)\s+(.*)`)
	matches := verRE.FindStringSubmatch(servodVersion)
	if len(matches) == 3 {
		ver := matches[1]
		date := strings.ReplaceAll(matches[2], "Date: ", "")
		servodVersion = ver + " " + date
	}

	// Servo type is a string like "servo_v4_with_c2d2_and_ccd_gsc".
	servoType, err := servo.GetServoType(ctx)
	if err != nil {
		testing.ContextLog(ctx, "Failed to get servo type: ", err)
	}

	var si servoInfo
	si.ServodVersion = servodVersion
	si.ServoType = servoType
	jsonData, err := json.Marshal(si)
	if err != nil {
		testing.ContextLog(ctx, "Failed to marshal json: ", err)
	}
	err = os.WriteFile(filepath.Join(outDir, "servo_info.json"), jsonData, 0666)
	if err != nil {
		testing.ContextLog(ctx, "Failed to write file: ", err)
	}
}

func getRelevantLogs(allFiles []string, startFile string, initialRotatedCount int) []string {
	// Regex captures: group 1 is the base filename (log...DEBUG), group 2 is the optional numeric suffix.
	logPattern := regexp.MustCompile(`^(log\.\d{4}-\d{2}-\d{2}--\d{2}-\d{2}-\d{2}\.\d{3}\.DEBUG)(?:\.(\d+))?$`)

	type logEntry struct {
		name   string
		base   string
		suffix int
	}

	var relevantLogs []logEntry
	for _, f := range allFiles {
		// Filter by base string comparison to startFile.
		// This includes the startFile itself, and any file that sorts after it (newer timestamps or suffixes).
		// Note: "log...DEBUG.1" > "log...DEBUG".
		if f >= startFile {
			matches := logPattern.FindStringSubmatch(f)
			if matches != nil {
				entry := logEntry{
					name: f,
					base: matches[1],
					// Default suffix to -1 for the active file (no numeric suffix),
					// so it sorts after numbered backups.
					suffix: -1,
				}
				if matches[2] != "" {
					if val, err := strconv.Atoi(matches[2]); err == nil {
						entry.suffix = val
					}
				}
				relevantLogs = append(relevantLogs, entry)
			}
		}
	}

	// Sort logs chronologically:
	// 1. Different base (timestamp): Ascending order (oldest invocation first).
	// 2. Same base: Descending suffix order (Oldest backup first).
	//    Example: log.DEBUG.2 (oldest) -> log.DEBUG.1 -> log.DEBUG (newest/current, suffix -1).
	sort.Slice(relevantLogs, func(i, j int) bool {
		if relevantLogs[i].base != relevantLogs[j].base {
			return relevantLogs[i].base < relevantLogs[j].base
		}
		return relevantLogs[i].suffix > relevantLogs[j].suffix
	})

	// Skip the oldest rotated logs that existed before the current session started.
	// relevantLogs is sorted [Oldest ... Newest]. The first N items are the oldest history.
	if len(relevantLogs) > initialRotatedCount {
		relevantLogs = relevantLogs[initialRotatedCount:]
	} else {
		// If we have fewer logs than the initial count (files deleted?), return empty or whatever is left?
		// Safest to return empty to avoid dupes if state is inconsistent, but typically this implies no new data.
		return nil
	}

	var files []string
	for _, l := range relevantLogs {
		files = append(files, l.name)
	}
	return files
}
