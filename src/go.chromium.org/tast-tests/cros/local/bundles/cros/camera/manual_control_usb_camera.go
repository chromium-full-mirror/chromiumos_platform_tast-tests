// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

type controlKey string

const (
	saveImageKey         = "camera.ManualControlUSBCamera.SaveImage"
	numControlLevel      = 10
	outputResultFileName = "control_functionality_result.json"

	brightnessControlKey              controlKey = "brightness"
	contrastControlKey                controlKey = "contrast"
	hueControlKey                     controlKey = "hue"
	saturationControlKey              controlKey = "saturation"
	whiteBalanceTemperatureControlKey controlKey = "white_balance_temperature"
	whiteBalanceAutomaticControlKey   controlKey = "white_balance_automatic"
	sharpnessKey                      controlKey = "sharpness"
)

type control struct {
	id           int64
	dataType     string
	min          int
	max          int
	step         int
	defaultValue int
	value        int
}

type result struct {
	VidPid        string              `json:"vidPid"`
	Functionality map[controlKey]bool `json:"functionality"`
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManualControlUSBCamera,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check the functionality of manual controlling usb cameras",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{caps.BuiltinUSBCamera},
		Fixture:      "ccaLaunched",
		Vars:         []string{saveImageKey},
		Timeout:      10 * time.Minute,
	})
}

func checkV4L2ListControlInfoKeys(dataType string, detailMatches [][]string) error {
	var requiredKeys []string
	switch dataType {
	case "int":
		requiredKeys = []string{"min", "max", "step", "default", "value"}
	case "bool":
		requiredKeys = []string{"default", "value"}
	case "menu":
		requiredKeys = []string{"min", "max", "default", "value"}
	default:
		return errors.Errorf("unknown dataType: %s", dataType)
	}

	foundKeys := make(map[string]bool)
	for _, match := range detailMatches {
		foundKeys[match[1]] = true
	}
	for _, key := range requiredKeys {
		if !foundKeys[key] {
			return errors.Errorf("missing required key %q for data type %q", key, dataType)
		}
	}
	return nil
}

func getV4L2ListControlInfo(ctx context.Context, videoNode string) (map[string]*control, error) {
	v4l2GetControlCmd := testexec.CommandContext(
		ctx, "v4l2-ctl", "-d", videoNode, "--list-ctrls")
	v4l2GetControlOutput, err := v4l2GetControlCmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to run command %s", v4l2GetControlCmd)
	}

	// Control line regex  is of the following pattern:
	// <control name> <control number> (<type>) : <key-value pairs>
	var controlLineRegex = regexp.MustCompile(`^\s*(\w+)\s+0x([a-fA-F0-9]+)\s+\((\w+)\)\s+:(.*)$`)

	var keyValuePairRegex = regexp.MustCompile(`(\w+)=(-?\d+)`)

	lines := strings.Split(string(v4l2GetControlOutput), "\n")
	userControlStart := false
	controlMap := make(map[string]*control)
	for _, line := range lines {
		if line == "" {
			continue
		}

		line = strings.TrimSpace(line)
		if line == "User Controls" {
			userControlStart = true
			continue
		} else if line == "Camera Controls" {
			userControlStart = false
			continue
		} else if userControlStart {
			matches := controlLineRegex.FindStringSubmatch(line)
			if matches == nil {
				return nil, errors.Errorf("failed to parse the line: %s", line)
			}
			if _, ok := controlMap[matches[1]]; ok {
				return nil, errors.Errorf("control %s already exists", matches[1])
			}

			controlMap[matches[1]] = &control{}
			control := controlMap[matches[1]]
			control.id, err = strconv.ParseInt(matches[2], 16, 64)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert control id from a string to a number")
			}
			control.dataType = matches[3]
			detailMatches := keyValuePairRegex.FindAllStringSubmatch(matches[4], -1)
			checkV4L2ListControlInfoKeys(control.dataType, detailMatches)
			for _, detailMatch := range detailMatches {
				switch detailMatch[1] {
				case "min":
					control.min, err = strconv.Atoi(detailMatch[2])
					break
				case "max":
					control.max, err = strconv.Atoi(detailMatch[2])
					break
				case "step":
					control.step, err = strconv.Atoi(detailMatch[2])
					break
				case "default":
					control.defaultValue, err = strconv.Atoi(detailMatch[2])
					break
				case "value":
					control.value, err = strconv.Atoi(detailMatch[2])
					break
				default:
					return nil, errors.Wrapf(err, "unknown key of a control: %s", detailMatch[1])
				}
				if err != nil {
					return nil, errors.Wrap(err, "failed to convert control value from a string to a number")
				}
			}
		}
	}
	return controlMap, nil
}

func switchCameraByVidPid(ctx context.Context, vidPid string, app *cca.App) error {
	numCameras, err := app.GetNumOfCameras(ctx)
	if err != nil {
		return errors.Wrap(err, "can't get number of cameras")
	}

	for i := 0; i < numCameras; i++ {
		if activeVidPid, err := app.GetVidPid(ctx); err != nil {
			return errors.Wrap(err, "cannot get vid:pid from CCA")
		} else if activeVidPid == vidPid {
			return nil
		}

		if err := app.SwitchCamera(ctx); err != nil {
			return errors.Wrap(err, "failed to switch camera")
		}
	}
	return errors.Errorf("cannot find the camera with vid:pid=%s", vidPid)
}

func resetCameraUserControl(ctx context.Context, videoNode string) error {
	cmd := testexec.CommandContext(
		ctx, "yavta", "--reset-controls", videoNode)
	testing.ContextLogf(ctx, "Run command: %s", cmd)
	if _, err := cmd.Output(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to run command %s: ", cmd)
	}
	// GoBigSleepLint: Wait for the control to be written.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	return nil
}

func setCameraUserControl(ctx context.Context, videoNode string, controlName controlKey, targetValue int) error {
	cmd := testexec.CommandContext(
		ctx, "v4l2-ctl", "-d", videoNode, "--set-ctrl", fmt.Sprintf("%s=%d", controlName, targetValue))
	if _, err := cmd.Output(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to run command %s: ", cmd)
	}
	testing.ContextLogf(ctx, "Run command: %s", cmd)

	// GoBigSleepLint: Wait for the control to be written.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	return nil
}

func takePhoto(ctx context.Context, app *cca.App, imageOutputPath string) error {
	info, err := app.TakeSinglePhoto(ctx, cca.TimerOff)
	if err != nil {
		return errors.Wrap(err, "failed to take photo")
	}
	imageInputPath, err := app.FilePathInSavedDir(ctx, info[0].Name())
	if err != nil {
		return errors.Wrap(err, "failed to get file path")
	}

	if err := fsutil.CopyFile(imageInputPath, imageOutputPath); err != nil {
		return errors.Wrapf(err, "failed to copy file from path %s to path %s", imageInputPath, imageOutputPath)
	}
	return nil
}

func controlCameraByVideoNode(ctx context.Context, videoNode string, saveImage bool, app *cca.App, outDir string) (*result, error) {
	usbCameraVersion, err := testutil.GetUsbCameraVersion(ctx, videoNode)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get usb camera version from videoNode %s", videoNode)
	}
	vidPid := usbCameraVersion.IDVendor + ":" + usbCameraVersion.IDProduct
	if err := switchCameraByVidPid(ctx, vidPid, app); err != nil {
		return nil, errors.Wrapf(err, "failed to find camera with vid:pid=%s", vidPid)
	}

	controlMap, err := getV4L2ListControlInfo(ctx, videoNode)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse v4l2 list control info")
	}

	if err := resetCameraUserControl(ctx, videoNode); err != nil {
		return nil, errors.Wrap(err, "failed to reset all camera user controls")
	}

	controlResult := &result{
		VidPid:        vidPid,
		Functionality: make(map[controlKey]bool),
	}
	for _, controlName := range []controlKey{
		brightnessControlKey,
		contrastControlKey,
		hueControlKey,
		saturationControlKey,
		whiteBalanceTemperatureControlKey,
		sharpnessKey,
	} {
		controlResult.Functionality[controlName] = true
		if controlName == whiteBalanceTemperatureControlKey {
			if err := setCameraUserControl(ctx, videoNode, whiteBalanceAutomaticControlKey, 0); err != nil {
				controlResult.Functionality[controlName] = false
				testing.ContextLogf(ctx, "Failed to set camera control: %s", err)
				continue
			}
		}

		if _, ok := controlMap[string(controlName)]; !ok {
			controlResult.Functionality[controlName] = false
			testing.ContextLogf(ctx, "Control %s does not exist", controlName)
			continue
		}

		control := controlMap[string(controlName)]
		for i := 0; i < numControlLevel; i++ {
			targetValue := int(float64(control.min*(numControlLevel-1-i)+control.max*i) / float64(numControlLevel-1))
			targetValue -= (targetValue - control.min) % control.step
			if err := setCameraUserControl(ctx, videoNode, controlName, targetValue); err != nil {
				controlResult.Functionality[controlName] = false
				testing.ContextLogf(ctx, "Failed to set camera user control of videoNode %s with control name %s and target value %d: %s", videoNode, controlName, targetValue, err)
			}
			if saveImage {
				imageOutputPath := fmt.Sprintf("%s/%s_%s_%d.jpg", outDir, strings.ReplaceAll(vidPid, ":", "-"), controlName, i)
				if err := takePhoto(ctx, app, imageOutputPath); err != nil {
					return nil, errors.Wrap(err, "failed to take a photo")
				}
			}
		}

		// Restore Control.
		if err := setCameraUserControl(ctx, videoNode, controlName, control.defaultValue); err != nil {
			return nil, errors.Wrap(err, "failed to reset camamera user control")
		}
		if controlName == whiteBalanceAutomaticControlKey {
			if err := setCameraUserControl(ctx, videoNode, whiteBalanceAutomaticControlKey, 1); err != nil {
				return nil, errors.Wrapf(err, "failed to reset %s", whiteBalanceTemperatureControlKey)
			}
		}
	}
	return controlResult, nil
}

func ManualControlUSBCamera(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := upstart.RestartJob(cleanupCtx, "cros-camera"); err != nil {
		s.Fatal("Failed to restart cros-camera service: ", err)
	}

	app := s.FixtValue().(cca.FixtureData).App()
	usbCameraList, err := testutil.USBCamerasFromV4L2Test(ctx)
	if err != nil {
		s.Fatal("Failed to get usb camera list: ", err)
	}

	saveImage := false
	var controlResults []result
	if saveImageString, hasSaveImage := s.Var(saveImageKey); hasSaveImage {
		saveImage, err = strconv.ParseBool(saveImageString)
		if err != nil {
			s.Fatalf("Failed to parse %s from a string to a bool", saveImageString)
		}
	}
	for _, videoNode := range usbCameraList {
		defer func() {
			if err := resetCameraUserControl(ctx, videoNode); err != nil {
				s.Fatalf("Failed to reset all camera user controls of videoNode %s", videoNode)
			}

		}()
		controlResult, err := controlCameraByVideoNode(ctx, videoNode, saveImage, app, s.OutDir())
		controlResults = append(controlResults, *controlResult)
		if err != nil {
			s.Fatalf("Failed to control camera with video node %s: %s", videoNode, err)
		}
	}

	jsonData, err := json.MarshalIndent(controlResults, "", " ")
	if err != nil {
		s.Fatal("Failed to marshal the control result: ", err)
	}
	outputResultPath := path.Join(s.OutDir(), outputResultFileName)
	if err := os.WriteFile(outputResultPath, jsonData, 0644); err != nil {
		s.Fatalf("Failed to write the JSON data to path %s", outputResultPath)
	}
}
