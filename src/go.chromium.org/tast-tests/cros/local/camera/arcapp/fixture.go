// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package arcapp provides utilities to interact with ARC Camera Test App.
package arcapp

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/testing"
)

const (
	setUpTimeout = 60 * time.Second
)

type fixture struct{}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:         "arcWithWorkingCamera",
		Desc:         "ARC is booted with working camera(s)",
		Contacts:     []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Impl:         &fixture{},
		Parent:       "arcBootedRestricted",
		SetUpTimeout: setUpTimeout,
	})

	testing.AddFixture(&testing.Fixture{
		Name:         "arcWithWorkingCameraForPerf",
		Desc:         "ARC is booted with working camera(s) with noise disabled for performance measurement",
		Contacts:     []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Impl:         &fixture{},
		Parent:       "arcBootedWithDisableExternalStorage",
		SetUpTimeout: setUpTimeout,
	})
}

func (f *fixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	preData := s.ParentValue().(*arc.PreData)

	if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
		s.Fatal("Failed to start cros-camera: ", err)
	}

	a := preData.ARC
	cmd := a.Command(ctx, "dumpsys", "media.camera")
	o, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to execute dumpsys commmand: ", err)
	}

	cameraNumRE := regexp.MustCompile(`Number of camera devices: (\d+)`)
	outputString := string(o)
	matches := cameraNumRE.FindStringSubmatch(outputString)
	if matches == nil {
		s.Fatal("Failed to find the amount of cameras in the dumpsys output. Got: ", outputString)
	}
	numOfCameras, err := strconv.Atoi(matches[1])
	if err != nil {
		s.Fatal("Failed to parse number of cameras: ", err)
	}
	if numOfCameras < 1 {
		s.Fatal("No camera is found on the device")
	}
	return preData
}

func (f *fixture) TearDown(ctx context.Context, s *testing.FixtState) {}

func (f *fixture) Reset(ctx context.Context) error {
	return nil
}

func (f *fixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *fixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
