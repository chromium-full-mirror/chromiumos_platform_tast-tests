// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/xmlrpc"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

const (
	xIdx        = 0
	yIdx        = 1
	pressureIdx = 2
	timeIdx     = 3

	maxNumParts = 5
)

// TouchHostConnectionManager manages ssh connection information between the drone and touchhost.
type TouchHostConnectionManager struct {
	SSHConn                *ssh.Conn
	SSHOptions             *ssh.Options
	TouchhostPortForwarder *ssh.Forwarder
}

// NewHMRInterface creates an XMLRPC interface through which RPC calls can be made to the HMR Touchhost.
func NewHMRInterface(ctx context.Context, host string, port int) (*xmlrpc.CommonRPCInterface, error) {
	hmrInterface := xmlrpc.NewCommonRPCInterface(xmlrpc.New(host, port), "")

	err := action.Retry(5, func(ctx context.Context) error {
		testing.ContextLogf(ctx, "Attempting to connect to HMR Touchhost: %s:%d", host, port)
		deadline, _ := ctx.Deadline()
		timeUntil := time.Until(deadline)
		testing.ContextLogf(ctx, "time until deadline: %s", timeUntil)
		err := hmrInterface.RPC("TestConnection").Timeout(30 * time.Second).Call(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to communicate with HMR device with a call to TestConnection on HMR Touchhost")
		}
		return nil
	}, 30*time.Second)(ctx)

	return hmrInterface, err
}

// RemoveCommonDataErrorsFromStylusLogFile checks for common touch log data errors and generates a cleaned touch log file with these errors removed.
func RemoveCommonDataErrorsFromStylusLogFile(rawFilePath, cleanedFilePath string) error {
	readFile, err := os.Open(rawFilePath)
	if err != nil {
		return err
	}
	defer readFile.Close()

	writeFile, err := os.Create(cleanedFilePath)
	if err != nil {
		return err
	}
	defer writeFile.Close()

	scanner := bufio.NewScanner(readFile)
	writer := bufio.NewWriter(writeFile)

	// All touch log csv files must contain "x,y,pressure,time" as their header.
	scanner.Scan()
	line := scanner.Text()
	if line != "x,y,pressure,time" {
		return errors.New(rawFilePath + " does not contain csv header")
	}
	if _, err = writer.WriteString(line + "\n"); err != nil {
		return err
	}

	// There are two common data errors that are being checked for:

	// The prefix error occurs when an number of rows at the start of a touch log file are missing elements.
	// Example:
	// 	10216,5164,,1691552057.683170,
	//	10216,,,1691552067.683172,
	// These errors must occur before the first row with all elements.
	// prefixDataChecked refers to whether a row with all elements has been read yet.
	// If a row missing elements is read after a row with all elements has been read, a fatal error is triggered.

	// The suffix error occurs when the final row at the end of a touch log file excludes some delimiters & elements. (The last element may also be only partially written)
	// Example:
	// 8200,680
	// suffixDataChecked refers to whether this row has been read yet.
	// If any row is read after this row, a fatal error is triggered.

	prefixDataChecked := false
	suffixDataChecked := false
	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return err
		}
		line := scanner.Text()
		splitLine := strings.Split(line, ",")
		if len(splitLine) == maxNumParts {
			if splitLine[xIdx] == "" || splitLine[yIdx] == "" || splitLine[pressureIdx] == "" || splitLine[timeIdx] == "" {
				if !prefixDataChecked {
					continue
				} else {
					return errors.New(rawFilePath + " contains rows with empty elements")
				}
			} else {
				prefixDataChecked = true
			}
		} else if len(splitLine) < maxNumParts {
			if !suffixDataChecked {
				suffixDataChecked = true
				continue
			} else {
				return errors.New(rawFilePath + " contains rows with less than 4 elements")
			}
		} else {
			return errors.New(rawFilePath + " contains a row with excess columns")
		}
		if _, err = writer.WriteString(line + "\n"); err != nil {
			return err
		}
	}
	writer.Flush()
	return nil
}

// CreateSSHTunnelToTouchhost creates a ssh tunnel between localhost port 9992 to TouchhostD's port on the Touchhost.
func CreateSSHTunnelToTouchhost(ctx context.Context, touchhostHostname string, touchhostPort int, d *dut.DUT) (*TouchHostConnectionManager, error) {
	sshOptions := &ssh.Options{
		KeyDir:  d.KeyDir(),
		KeyFile: d.KeyFile(),
	}

	err := ssh.ParseTarget(touchhostHostname, sshOptions)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse ssh target touchhost host (%s)", touchhostHostname)
	}
	sshConn, err := ssh.New(ctx, sshOptions)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to touchhost host (%s) over ssh", touchhostHostname)
	}

	onFwdError := func(err error) {
		testing.ContextLogf(ctx, "ssh forwarding error for touchhostd host %s: %s", touchhostHostname, err)
	}

	// TODO: b/343532845 - Handle finding a new local port if a port collision occurs.
	// Forwards TouchHostD's port on TouchHost to 9992 on localhost.
	touchhostPortForwarder, err := sshConn.ForwardLocalToRemote("tcp", "127.0.0.1:9992", "127.0.0.1:"+strconv.Itoa(touchhostPort), onFwdError)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to port forward touchhost (%s) port (%d)", touchhostHostname, touchhostPort)
	}
	return &TouchHostConnectionManager{SSHConn: sshConn, SSHOptions: sshOptions, TouchhostPortForwarder: touchhostPortForwarder}, nil
}
