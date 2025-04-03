// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package typectest contains constants & helper functions used by the tests in the typec directory.
// See "go.chromium.org/tast-tests/cros/common/typecutils" for generic typec helper functions
package typectest

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/services/cros/typec"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

const (
	// Filepath on the DUT for the servo Type C partner device.
	thunderboltDevicePath = "/sys/bus/thunderbolt/devices"
)

// Identifiers used to specify which Thunderbolt generation is being checked.
const (
	TbtGenAny = iota
	TbtGen4   = 4
)

// List of built-in Thunderbolt devices enumerated by the OS.
var builtInTBTDevices = []string{"domain0", "domain1", "0-0", "1-0"}

// LoginChrome is a helper which performs a Chrome Login with the Peripheral Data Access setting disabled.
func LoginChrome(ctx context.Context, d *dut.DUT, s *testing.State, keyFile string) error {
	// Connect to gRPC server.
	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}

	// Send key file to DUT.
	keyPath := filepath.Join("/tmp", keyFile)
	defer d.Conn().CommandContext(ctx, "rm", "-r", keyPath).Output()
	if _, err := linuxssh.PutFiles(
		ctx, d.Conn(), map[string]string{
			s.DataPath(keyFile): keyPath,
		},
		linuxssh.DereferenceSymlinks); err != nil {
		return errors.Wrapf(err, "failed to send data to remote data path %v", keyPath)
	}

	// Log in to Chrome.
	client := typec.NewServiceClient(cl.Conn)
	_, err = client.NewChromeLoginWithPeripheralDataAccess(ctx, &typec.KeyPath{Path: keyPath})
	if err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}

	return nil
}

// builtInTBTDevice returns whether the specified name is a built-in Thunderbolt device or not.
func builtInTBTDevice(name string) bool {
	for _, device := range builtInTBTDevices {
		if name == device {
			return true
		}
	}

	return false
}

// CheckTBTDevice is a helper function which checks for TBT device connection to a DUT.
// |expected| specifies whether we want to check for the presence of a TBT device (true) or the
// absence of one (false).
//
// Caller can specify |gen| to check for a specific Thunderbolt
// generation, or `TbtGenAny` if the check is not necessary.
func CheckTBTDevice(ctx context.Context, d *dut.DUT, expected bool, gen int) error {
	out, err := d.Conn().CommandContext(ctx, "ls", thunderboltDevicePath).Output()
	if err != nil {
		return errors.Wrap(err, "could not run ls command on DUT")
	}

	found := ""
	// Check for retimers.
	// They are of the form "0-0:1.1" or "0-0:3.1".
	re := regexp.MustCompile(`[\d\-\:]+\.\d`)
	for _, device := range strings.Split(string(out), "\n") {
		if device == "" {
			continue
		}

		if builtInTBTDevice(device) {
			continue
		}

		if re.MatchString(device) {
			continue
		}

		found = device
		break
	}

	if expected && found == "" {
		return errors.New("no TBT device found")
	} else if !expected && found != "" {
		return errors.Errorf("TBT device found: %s", found)
	}

	if expected && gen != TbtGenAny {
		out, err := d.Conn().CommandContext(ctx, "cat", filepath.Join(thunderboltDevicePath, found, "generation")).Output()
		if err != nil {
			return errors.Wrap(err, "couldn't read Thunderbolt generation")
		}

		if genFound, err := strconv.Atoi(strings.TrimSpace(string(out))); err != nil {
			return errors.Wrap(err, "couldn't parse Thunderbolt generation")
		} else if genFound != gen {
			return errors.Errorf("incorrect Thunderbolt generation; found: %d expected: %d", genFound, gen)
		}
	}

	return nil
}
