// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cca provides utilities to interact with Chrome Camera App.
package cca

import (
	"context"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/camera/chart"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

type fixture struct{}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "remoteCameraBox",
		Desc:            "Set up remotely for camera box tests",
		Contacts:        []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		BugComponent:    "b:167281", // ChromeOS > Platform > Technologies > Camera
		Impl:            &fixture{},
		SetUpTimeout:    5 * time.Second,
		TearDownTimeout: 5 * time.Second,
	})
}

// rsaFilesMap returns a map of RSA files to copy from the host device to DUT.
func rsaFilesMap() (map[string]string, error) {
	fileExists := func(file string) bool {
		_, err := os.Stat(file)
		return !os.IsNotExist(err)
	}

	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	if u.HomeDir == "" {
		return nil, errors.New("cannot determine home directory")
	}

	fileMap := make(map[string]string, 0)
	partnerTestingRSAFile := filepath.Join(u.HomeDir, ".ssh", "partner_testing_rsa")
	if fileExists(partnerTestingRSAFile) {
		fileMap[partnerTestingRSAFile] = filepath.Join(chart.SSHKeysDir, "partner_testing_rsa")
	}

	// TODO(b/260622707): Remove testing_rsa case once they are all deprecated.
	testingRSAFile := filepath.Join(u.HomeDir, ".ssh", "testing_rsa")
	if fileExists(testingRSAFile) {
		fileMap[testingRSAFile] = filepath.Join(chart.SSHKeysDir, "testing_rsa")
	}

	if len(fileMap) == 0 {
		return nil, errors.New("no required RSA files are found under ~/.ssh. Please put either `testing_rsa` or `partner_testing_rsa` under `~/.ssh`")
	}
	return fileMap, nil
}

func (f *fixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	filesMap, err := rsaFilesMap()
	if err != nil {
		s.Fatal("Failed to get map of the RSA files: ", err)
	}

	if _, err := linuxssh.PutFiles(ctx, s.DUT().Conn(), filesMap, linuxssh.DereferenceSymlinks); err != nil {
		s.Fatal("Failed to copy RSA files to the DUT: ", err)
	}
	return tabletIP(ctx, s.DUT().HostName())
}

func (f *fixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if _, err := s.DUT().Conn().CommandContext(ctx, "rm", "-rf", chart.SSHKeysDir).Output(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to remove ssh keys for camera box: ", err)
	}
}

func (f *fixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *fixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *fixture) Reset(ctx context.Context) error { return nil }

func tabletIP(ctx context.Context, dutHost string) string {
	tabletHost, err := utils.CompanionDeviceHostname(dutHost, utils.CompanionSuffixTablet)
	if err != nil {
		// It is acceptable if it fails since the test could be run outside test without the expected host name.
		// As a result, only collect the tablet host name if it succeed.
		testing.ContextLog(ctx, "Did not find the host name of the tablet device. Ignore")
		return ""
	}
	testing.ContextLog(ctx, "Try to look up IP from tablet host name: ", tabletHost)
	ipAddrs, err := net.LookupIP(tabletHost)
	if err != nil {
		testing.ContextLog(ctx, "Did not find the IP of the tablet device. Ignore")
		return ""
	}
	testing.ContextLog(ctx, "Found IP of the tablet: ", ipAddrs)
	// Use IP so that it can find the tablet device on DUT.
	return ipAddrs[0].String()
}
