// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/camera/hal3"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

const hwTimestampsPath = "/sys/module/uvcvideo/parameters/hwtimestamps"

type v4l2TestType int

const (
	vttDefault v4l2TestType = iota
	vttSupportedFormats
	vttCertification
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         V4L2,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies required V4L2 operations on USB camera devices",
		Contacts:     []string{"chromeos-camera-eng@google.com", "kamesan@chromium.org", "shik@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Timeout:      10 * time.Minute,
		Params: []testing.Param{
			{
				ExtraAttr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
				ExtraSoftwareDeps: []string{caps.BuiltinUSBCamera},
				Val:               vttDefault,
			},
			{
				Name:              "supported_formats",
				ExtraAttr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
				ExtraSoftwareDeps: []string{caps.BuiltinUSBCamera},
				Val:               vttSupportedFormats,
			},
			{
				Name:      "certification",
				ExtraAttr: []string{"group:camera-usb-qual"},
				Val:       vttCertification,
			},
		},
	})
}

func V4L2(ctx context.Context, s *testing.State) {
	testType := s.Param().(v4l2TestType)
	isCertification := testType == vttCertification

	hasHWTimestamps, err := pathExist(hwTimestampsPath)
	if err != nil {
		s.Fatal("Failed to check hardware timestamps: ", err)
	}
	if hasHWTimestamps {
		origVal, err := setHWTimestamps("1")
		if err != nil {
			s.Fatal("Failed to set hardware timestamps: ", err)
		}
		defer setHWTimestamps(origVal)
	}

	testList := "certification"
	if !isCertification {
		if testList, err = getTestList(ctx); err != nil {
			s.Fatal("Failed to get test list: ", err)
		}
	}

	usbCams, err := testutil.USBCamerasFromV4L2Test(ctx)
	if err != nil {
		s.Fatal("Failed to get USB cameras: ", err)
	}
	if len(usbCams) == 0 {
		s.Fatal("Failed to find any valid device")
	}
	s.Log("USB cameras: ", usbCams)

	filter := ""
	if testType == vttDefault {
		filter = "-V4L2Test.SupportedFormats"
	} else if testType == vttSupportedFormats {
		filter = "V4L2Test.SupportedFormats"
	}

	for _, devicePath := range usbCams {
		extraArgs := []string{
			"--device_path=" + devicePath,
			"--test_list=" + testList,
		}

		logFile := fmt.Sprintf("media_v4l2_test_%s.log", filepath.Base(devicePath))

		t := gtest.New("media_v4l2_test",
			gtest.Logfile(filepath.Join(s.OutDir(), logFile)),
			gtest.Filter(filter),
			gtest.ExtraArgs(extraArgs...))

		if args, err := t.Args(); err == nil {
			s.Log("Running " + shutil.EscapeSlice(args))
		}

		report, err := t.Run(ctx)
		if err != nil {
			if report != nil {
				for _, name := range report.FailedTestNames() {
					s.Error(name, " failed")
				}
			}
			s.Fatal("Failed to run media_v4l2_test: ", err)
		}
	}

}

func pathExist(path string) (bool, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func setHWTimestamps(newValue string) (oldValue string, err error) {
	b, err := ioutil.ReadFile(hwTimestampsPath)
	if err != nil {
		return "", err
	}
	oldValue = string(b)

	if err := ioutil.WriteFile(hwTimestampsPath, []byte(newValue), 0644); err != nil {
		return "", err
	}
	return oldValue, err
}

func getTestList(ctx context.Context) (string, error) {
	hasV3, err := pathExist("/usr/bin/cros_camera_service")
	if err != nil {
		return "", err
	}

	isV1, err := hal3.IsV1Legacy(ctx)
	if err != nil {
		return "", err
	}

	if hasV3 && !isV1 {
		return "halv3", nil
	}

	return "default", nil
}
