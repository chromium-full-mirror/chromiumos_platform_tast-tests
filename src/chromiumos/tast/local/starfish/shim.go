// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package starfish

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"time"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
	"go.chromium.org/tast/core/timing"
)

const charCR = "\r\n"
const rCharCR = "\n\r"
const starfishName = "Starfish"
const starfishPrompt = starfishName + ":~$ "
const cmdNotFound = ": command not found"
const logPrefix = "inf"
const warnPrefix = "wrn"
const errPrefix = "err"
const drvPrefix = "usb_cdc_acm: " //unused as of now
const consPrefix = "console: "
const simPrefix = "sim: "

// Time to wait between command write and response read
const commandWaitTime = 250 * time.Millisecond
const readTimeout = 1 * time.Second
const baud = 115200

var garbageChars = string([]byte{27, 91, 49, 50, 68, 27, 91, 74})

const maxReadBufferSize = 16384 // bytes

// shim holds data pertaining to the serial interface exposed on Starfish module
type shim struct {
	siface *SerialInterface
}

// NewShim creates a shim object and ensures that it is initialized properly.
func NewShim(ctx context.Context) (*shim, []string, error) {
	ctx, st := timing.Start(ctx, "shim.NewShim")
	defer st.End()

	devName, err := findStarfish(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "find failed")
	}
	si, err := NewSerialInterface(ctx, devName, baud, readTimeout)
	if err != nil {
		return nil, nil, errors.Wrap(err, "open failed")
	}
	s := shim{siface: si}
	testing.Sleep(ctx, 6*commandWaitTime)
	err = s.siface.Flush(ctx)
	if err != nil {
		s.Close(ctx)
		return nil, nil, errors.Wrap(err, "flush failed")
	}
	testing.Sleep(ctx, commandWaitTime)
	_, logs, _ := s.SendCommand(ctx, "")
	return &s, logs, nil
}

// Close handles the close of the Starfish SHIM layer.
func (s *shim) Close(ctx context.Context) error {
	if err := s.siface.Close(ctx); err != nil {
		return errors.Wrap(err, "close failed")
	}
	return nil
}

// SendCommand handles sending a command and parsing the response
func (s *shim) SendCommand(ctx context.Context, command string) ([]string, []string, error) {
	command += charCR
	inBuf := []byte(command)
	x := len(inBuf)
	n, err := s.siface.Write(ctx, inBuf)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "writing command %s failed", command)
	}
	if n != x {
		return nil, nil, errors.Errorf("write command %s, length mismatch. expected: %d, actual: %d", command, n, x)
	}
	testing.Sleep(ctx, 2*commandWaitTime)
	outBuf := make([]byte, maxReadBufferSize)
	if _, err = s.siface.Read(ctx, outBuf); err != nil {
		return nil, nil, errors.Wrapf(err, "reading response for command %s failed", command)
	}
	outBuf = bytes.Trim(outBuf, "\x00")
	response := string(outBuf)
	response = strings.ReplaceAll(response, garbageChars, "")
	responses, logs, errStr := parseOutput(ctx, response, command)
	if errStr != "" {
		return responses, logs, errors.New(errStr)
	}
	return responses, logs, nil
}

// findStarfish checks if a Starfish module is connected
func findStarfish(ctx context.Context) (string, error) {
	cmd := testexec.CommandContext(ctx, "find", "/dev/")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	var listOfDevs string = string(output)
	re := regexp.MustCompile(`/dev/ttyACM\S*`)
	var ldevs []string = re.FindAllString(listOfDevs, -1)
	for _, dev := range ldevs {
		cmd := testexec.CommandContext(ctx, "udevadm", "info", "--query=symlink", "--name="+dev)
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		if strings.Contains(string(output), starfishName) {
			return dev, nil
		}
	}
	return "", errors.New("starfish not found")
}

// parseOutput sanitizes the raw reads from the serial port and puts out a string array
func parseOutput(ctx context.Context, response, cmd string) ([]string, []string, string) {
	errs := ""
	var logs []string
	response = strings.TrimPrefix(response, cmd)
	response = strings.ReplaceAll(response, rCharCR, charCR)
	response = strings.ReplaceAll(response, starfishPrompt, "")
	response = strings.TrimSuffix(response, charCR)
	i := 0
	responses := strings.Split(response, charCR)
	for _, line := range responses {
		_, l, r, p := parseOutputLine(line)
		if !r {
			if line != "" {
				logs = append(logs, line)
			}
		} else {
			responses[i] = p
			i++
			if (l == errPrefix || l == warnPrefix) && r {
				errs = p
			}
		}
	}
	responses = responses[:i]
	return responses, logs, errs
}

// parseOutputLine splits response line into timestamp, log type, relevant, payload
// typical output line is one of the following:
//
// a. USB driver log (which gets piped out to tast context log as-is):
//
//	[00:00:00.343,000] <inf> usb_cdc_acm: from suspend
//
// b. Starfish console output:
//
//	[01:07:02.235,000] <inf> console: SIM 0 = None
//
// c. SIM command response:
//
//	[01:09:43.040,000] <inf> sim: Enabled SIM mux:1
func parseOutputLine(line string) (string, string, bool, string) {
	t := ""
	l := logPrefix
	r := false
	p := ""
	re := regexp.MustCompile(`\[[0-9]{16}\] `)
	matches := re.FindAllString(line, -1)
	for _, m := range matches {
		line = strings.ReplaceAll(line, m, "")
		m = strings.TrimSuffix(m, " ")
		t = m
	}
	re = regexp.MustCompile(`<[a-z][a-z][a-z]> `)
	matches = re.FindAllString(line, -1)
	for _, m := range matches {
		line = strings.ReplaceAll(line, m, "")
		replacer := strings.NewReplacer(" ", "", "<", "", ">", "")
		m = replacer.Replace(m)
		l = m
	}
	if strings.HasPrefix(line, consPrefix) {
		r = true
		line = strings.ReplaceAll(line, consPrefix, "")
	} else if strings.HasPrefix(line, simPrefix) {
		r = true
		line = strings.ReplaceAll(line, simPrefix, "")
	}
	p = line
	if strings.Contains(p, cmdNotFound) {
		r = true
		l = errPrefix
	}
	return t, l, r, p
}
