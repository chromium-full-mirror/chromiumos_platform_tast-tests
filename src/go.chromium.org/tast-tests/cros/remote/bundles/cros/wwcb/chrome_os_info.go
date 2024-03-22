// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/exec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeOsInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Get the system information information",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		Data:         []string{"Capabilities.json"},
	})
}

func ChromeOsInfo(ctx context.Context, s *testing.State) {
	// Set up the servo attached to the DUT.
	dut := s.DUT()
	capabilitiesMap, err := utils.ReadConfigFile(s)
	if err != nil {
		s.Fatal("Failed to read config file: ", err)
	}
	pollTimeout, pollInterval := 10*time.Second, 200*time.Millisecond
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := dut.Conn().CommandContext(ctx, "sudo", "cat", "/etc/lsb-release").Output(exec.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "get the OS information")
		}
		info := string(out)
		pattern := `(CHROMEOS_RELEASE_VERSION=(\S+)\n)|(CHROMEOS_RELEASE_BOARD=(\S+))\n`
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(info, -1)

		if len(matches) > 0 {
			version := strings.TrimSpace(matches[1][0])
			board := strings.TrimSpace(matches[0][0])
			testing.ContextLogf(ctx, "%s", version)
			testing.ContextLogf(ctx, "%s", board)
			capabilitiesMap["version"] = strings.Replace(version, "CHROMEOS_RELEASE_VERSION=", "", -1)
			capabilitiesMap["board"] = strings.Replace(board, "CHROMEOS_RELEASE_BOARD=", "", -1)
		} else {
			return errors.New("no matches found OS information")
		}

		return nil
	}, &testing.PollOptions{Timeout: pollTimeout, Interval: pollInterval}); err != nil {
		s.Fatal("Failed to get the os information by lsb-release: ", err)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := dut.Conn().CommandContext(ctx, "ectool", "version").Output(exec.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "get the ectool version")
		}
		info := string(out)
		pattern := `(RO version:\s*(\S+))|(RW version:\s*(\S+))`
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(info, -1)
		if len(matches) > 0 {
			rov := strings.TrimSpace(matches[1][0])
			rwv := strings.TrimSpace(matches[0][0])
			testing.ContextLogf(ctx, "%s", rov)
			testing.ContextLogf(ctx, "%s", rwv)
			capabilitiesMap["ro version"] = strings.Replace(rov, "RW version:    ", "", -1)
			capabilitiesMap["rw version"] = strings.Replace(rwv, "RO version:    ", "", -1)
		} else {
			return errors.New("no matches found ectool version")
		}
		return nil
	}, &testing.PollOptions{Timeout: pollTimeout, Interval: pollInterval}); err != nil {
		s.Fatal("Failed to get the ectool version: ", err)
	}

	if err := utils.WriteConfigFile(s, capabilitiesMap); err != nil {
		s.Fatal("Failed to write config file: ", err)
	}
}
