// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package modulepower

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/testexec"
	rp "go.chromium.org/tast-tests/cros/remote/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

const deviceID = "18d1:5020"

var portRE = regexp.MustCompile(`port:\s+(\d+)`)

// Config represents a set of power measurement configuration options.
type Config struct {
	// MeasurementInterval is the sampling rate to use between measurements.
	MeasurementInterval time.Duration
	ServodPort          int
}

// SweetberryRecorder wraps ServodRecorder to provide intrusive power measurements.
type SweetberryRecorder struct {
	proxy      *ssh.Conn
	cleanup    func(ctx context.Context) error
	fwd        *ssh.Forwarder
	svo        *servo.Servo
	recorder   *rp.ServodRecorder
	servodPort int
}

// NewSweetberryRecorder creates and initializes a Sweetberry power recorder object.
//
// Note: We need to launch our own servo instance since we need to pass the correct configuration
// file for the sweetberry rather than connecting to the servod instance for the DUT.
func NewSweetberryRecorder(ctx context.Context, proxyHost *ssh.Conn, xmlFile string, config *Config) (*SweetberryRecorder, error) {
	s := &SweetberryRecorder{
		proxy:      proxyHost,
		servodPort: config.ServodPort,
	}

	if s.servodPort == 0 {
		testing.ContextLog(ctx, "No servod port specified, defaulting to 9999")
		s.servodPort = 9999
	}

	// Setup and configure a servod instance on the proxyHost.
	if err := s.initialize(ctx, xmlFile); err != nil {
		return nil, errors.Wrap(err, "failed to initialize servod")
	}

	onFwdError := func(err error) {
		testing.ContextLog(ctx, "Ssh forwarding error: ", err)
	}

	// Forward servod instance that we just started to our local machine.
	fwd, err := proxyHost.ForwardLocalToRemote("tcp", "localhost:", fmt.Sprintf("localhost:%d", s.servodPort), onFwdError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to forward local port to servod port")
	}
	s.fwd = fwd

	host, portstr, err := net.SplitHostPort(s.fwd.ListenAddr().String())
	if err != nil {
		return nil, errors.Wrap(err, "failed to split fowarding address")
	}
	port, err := strconv.Atoi(portstr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse servo port")
	}

	testing.ContextLogf(ctx, "Connecting to servod at %s:%d", host, port)
	s.svo, err = servo.New(ctx, host, port)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to servod instance")
	}

	servodRecorder, err := rp.NewServodRecorder(ctx, config.MeasurementInterval, s.svo, false /*isCPD*/, false /*useAccumulators*/)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create servod recorder")
	}
	s.recorder = servodRecorder

	return s, nil
}

// initialize starts the background servod instance with the provided xml config file.
func (s *SweetberryRecorder) initialize(ctx context.Context, xmlFile string) error {
	filePath, err := s.copyPowerConfig(ctx, xmlFile)
	if err != nil {
		return errors.Wrap(err, "failed to copy configuration xml file to proxy host")
	}
	defer s.proxy.CommandContext(ctx, "rm", filePath).Run(testexec.DumpLogOnError)

	deviceSerial, err := s.findPowerDevice(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to fetch power measurement device ID")
	}
	testing.ContextLogf(ctx, "Using power measurement device: %q", deviceSerial)

	// Start servod on host.
	// Make up to 3 attempts to start servod since the first detect tends to fail,
	// something about running 'lsusb -v' before launching servod can prevent some
	// of the device descriptions from being detected.
	for i := 0; i < 3; i++ {
		if err := s.stopServod(ctx, deviceSerial); err != nil {
			return errors.Wrap(err, "failed to kill existing servod instances")
		}

		if err := s.startServod(ctx, deviceSerial, filePath); err != nil {
			continue
		}
		s.cleanup = func(ctx context.Context) error {
			return s.stopServod(ctx, deviceSerial)
		}
		testing.ContextLog(ctx, "finished starting servod")
		return nil
	}
	return errors.New("failed to start servod on proxy")
}

// stopServod stops the servod session on the proxy host.
func (s *SweetberryRecorder) stopServod(ctx context.Context, deviceSerial string) error {
	err := s.proxy.CommandContext(ctx, "pkill", "-9", "-f", fmt.Sprintf(" -s %s", deviceSerial)).Run()
	if err != nil && err.Error() != "Process exited with status 1" {
		return errors.Wrap(err, "failed to kill servod with pkill")
	}
	return nil
}

// startServod configures and launches servod on the proxy host.
func (s *SweetberryRecorder) startServod(ctx context.Context, deviceSerial, filePath string) error {
	cmdServod := s.proxy.CommandContext(ctx, "sudo", "/usr/bin/servod", "--config", filePath, "-s", deviceSerial, "-p", strconv.Itoa(s.servodPort))
	stdout, err := cmdServod.StdoutPipe()
	if err != nil {
		return errors.Wrapf(err, "failed to get stdout for %q", shutil.EscapeSlice(cmdServod.Args))
	}
	if err := cmdServod.Start(); err != nil {
		return errors.Wrapf(err, "failed to run: %q", shutil.EscapeSlice(cmdServod.Args))
	}

	// Wait for servod to initialize.
	sc := bufio.NewScanner(stdout)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		for sc.Scan() {
			t := sc.Text()
			testing.ContextLog(ctx, t)
			if strings.Contains(t, "Listening on localhost") {
				return nil
			}
		}

		if err := sc.Err(); err != nil {
			return testing.PollBreak(errors.Wrap(err, "error while scanning servo output"))
		}
		return errors.New("servod instance is not ready")
	}, &testing.PollOptions{
		Timeout: 10 * time.Second,
	}); err != nil {
		return errors.Wrap(err, "failed to start servod power measurement")
	}
	return nil
}

// Start starts the power measurement recording.
func (s *SweetberryRecorder) Start(ctx context.Context) error {
	testing.ContextLog(ctx, "Starting servod recording")
	if err := s.recorder.Start(ctx); err != nil {
		return err
	}
	return nil
}

// Stop stops the power measurement recording.
func (s *SweetberryRecorder) Stop(ctx context.Context) (*perf.Values, error) {
	testing.ContextLog(ctx, "Stopping servod recording")
	perf, err := s.recorder.Stop(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to stop servod recorder")
	}
	return perf, nil
}

// ProcessResults returns the performance results trimmed for the given subtest.
func (s *SweetberryRecorder) ProcessResults(ctx context.Context, outDir, subtest string) (*perf.Values, error) {
	return s.recorder.ProcessResults(ctx, outDir, subtest)
}

// Close stops the module power measurement session and closes any resources held open.
func (s *SweetberryRecorder) Close(ctx context.Context) {
	if s.cleanup != nil {
		if err := s.cleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to cleanup servod instances: ", err)
		}
		s.cleanup = nil
	}

	if s.fwd != nil {
		if err := s.fwd.Close(); err != nil {
			testing.ContextLog(ctx, "Failed to close port forwarding: ", err)
		}
		s.fwd = nil
	}

	if s.svo != nil {
		if err := s.svo.Close(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to cleanup servo: ", err)
		}
		s.svo = nil
	}
}

// copyPowerConfig moves configuration .xml file to the proxy host.
func (s *SweetberryRecorder) copyPowerConfig(ctx context.Context, xmlFile string) (string, error) {
	p, err := s.proxy.CommandContext(ctx, "mktemp", "/tmp/power_XXXXXX.xml").Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to create remote data path directory")
	}

	remotePath := string(p)
	if _, err := linuxssh.PutFiles(
		ctx, s.proxy, map[string]string{
			xmlFile: remotePath,
		},
		linuxssh.DereferenceSymlinks); err != nil {
		return "", errors.Wrapf(err, "failed to send data to remote data path %q", remotePath)
	}
	return remotePath, nil
}

// findPowerDevice locates the sweetberry device serial using its VID+PID.
// Note: This assumes there is only one Sweetberry connected to the proxy host.
func (s *SweetberryRecorder) findPowerDevice(ctx context.Context) (string, error) {
	cmd := fmt.Sprintf("sudo lsusb -d %s -v | grep iSerial", deviceID)
	out, err := s.proxy.CommandContext(ctx, "sh", "-c", cmd).Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to find power measurement usb device")
	}

	fields := strings.Fields(string(out))
	if len(fields) != 3 {
		return "", errors.Errorf("failed to parse device serial from: %s", out)
	}
	return fields[2], nil
}
