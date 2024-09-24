// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/prototext"

	pb "go.chromium.org/tast-tests/cros/common/camera/manualcontrol"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast-tests/cros/services/cros/camera"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

type controlFunctionalityMap = map[vidPidType]map[pb.Control]bool
type controlKey string
type vidPidType = string

const (
	saveImageKey         = "camera.ManualControlUSBCamera.SaveImage"
	outputResultFileName = "control_functionality_result.json"
	imageConfigsFileName = "image_configs.pbtxt"
	reportFileName       = "image_validation_report.pbtxt"

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

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			camera.RegisterManualControlUSBCameraServiceServer(srv, &ManualControlUSBCameraService{s: s})
		},
	})
}

func controlToControlKey(pbControl pb.Control) (controlKey, error) {
	switch pbControl {
	case pb.Control_BRIGHTNESS:
		return brightnessControlKey, nil
	case pb.Control_CONTRAST:
		return contrastControlKey, nil
	case pb.Control_SATURATION:
		return saturationControlKey, nil
	case pb.Control_SHARPNESS:
		return sharpnessKey, nil
	case pb.Control_WHITE_BALANCE:
		return whiteBalanceTemperatureControlKey, nil
	case pb.Control_HUE:
		return hueControlKey, nil
	default:
		return "", errors.Errorf("unknown control: %s", pbControl)
	}
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

func validateControlByImages(ctx context.Context, outDir, cameraUserControlValidateScriptPath string, imageConfigs *pb.ManualControlImageConfigs) error {
	opts := prototext.MarshalOptions{Multiline: true, Indent: "  "}
	imageConfigsByte, err := opts.Marshal(imageConfigs)
	if err != nil {
		return errors.Wrap(err, "failed to marshal image configs")
	}
	imageConfigsFilePath := fmt.Sprintf("%s/%s", outDir, imageConfigsFileName)
	if err := os.WriteFile(imageConfigsFilePath, imageConfigsByte, 0644); err != nil {
		return errors.Wrapf(err, "failed to write to path %s:", imageConfigsFilePath)
	}
	validateControlCmd := testexec.CommandContext(
		ctx, "python3", cameraUserControlValidateScriptPath, "--config_path", imageConfigsFilePath)
	testing.ContextLogf(ctx, "Run command: %s", validateControlCmd)
	validateControlOutput, err := validateControlCmd.Output(testexec.DumpLogOnError)
	testing.ContextLogf(ctx, "Command Output: %s", validateControlOutput)

	if err != nil {
		return errors.Wrapf(err, "failed to run command %s", validateControlCmd)
	}
	return nil
}

func controlCameraByVideoNode(ctx context.Context, videoNode, vidPid string, saveImage bool, app *cca.App, outDir string) (map[pb.Control]bool, []*pb.ManualControlImageConfig, error) {

	controlMap, err := getV4L2ListControlInfo(ctx, videoNode)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse v4l2 list control info")
	}

	if err := resetCameraUserControl(ctx, videoNode); err != nil {
		return nil, nil, errors.Wrap(err, "failed to reset all camera user controls")
	}

	controlFunctionality := make(map[pb.Control]bool)
	type controlConfig struct {
		controlName    pb.Control
		numLevel       int32
		levelDistance  float64
		minMaxDistance float64
	}
	var imageConfigs []*pb.ManualControlImageConfig
	for _, controlConfig := range []controlConfig{
		{pb.Control_BRIGHTNESS, 10, 5.0, 150.0},
		{pb.Control_CONTRAST, 10, 1.0, 30.0},
		{pb.Control_SATURATION, 10, 2.0, 60.0},
		{pb.Control_SHARPNESS, 10, 0.0, 30.0},
		{pb.Control_WHITE_BALANCE, 10, 50.0, 2000.0},
		{pb.Control_HUE, 10, 3.0, 120.0},
	} {
		controlName := controlConfig.controlName
		numControlLevel := int(controlConfig.numLevel)
		controlFunctionality[controlName] = true
		if controlName == pb.Control_WHITE_BALANCE {
			if err := setCameraUserControl(ctx, videoNode, whiteBalanceAutomaticControlKey, 0); err != nil {
				controlFunctionality[controlName] = false
				testing.ContextLogf(ctx, "Failed to set camera control: %s", err)
				continue
			}
		}

		controlKey, err := controlToControlKey(controlName)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to convert control %s to a control key", controlName)
		}
		if _, ok := controlMap[string(controlKey)]; !ok {
			controlFunctionality[controlName] = false
			testing.ContextLogf(ctx, "Control %s does not exist", controlName)
			continue
		}

		control := controlMap[string(controlKey)]
		imageOutputPathPrefix := fmt.Sprintf("%s/%s_%s", outDir, strings.ReplaceAll(vidPid, ":", "-"), controlName)
		imageConfigs = append(imageConfigs, &pb.ManualControlImageConfig{
			ControlName:     controlName,
			VidPid:          vidPid,
			ImagePathPrefix: imageOutputPathPrefix,
			NumLevel:        controlConfig.numLevel,
			LevelDistance:   controlConfig.levelDistance,
			MinMaxDistance:  controlConfig.minMaxDistance,
		})
		for i := 0; i < numControlLevel; i++ {
			if numControlLevel < 2 {
				return nil, nil, errors.New("number of control level should be at least 2")
			}
			targetValue := int(float64(control.min*(numControlLevel-1-i)+control.max*i) / float64(numControlLevel-1))

			if err := setCameraUserControl(ctx, videoNode, controlKey, targetValue); err != nil {
				controlFunctionality[controlName] = false
				testing.ContextLogf(ctx, "Failed to set camera user control of videoNode %s with control name %s and target value %d: %s", videoNode, controlName, targetValue, err)
				continue
			}
			if err := takePhoto(ctx, app, fmt.Sprintf("%s_%d.jpg", imageOutputPathPrefix, i)); err != nil {
				return nil, nil, errors.Wrap(err, "failed to take a photo")
			}

		}

		// Restore Control.
		if err := setCameraUserControl(ctx, videoNode, controlKey, control.defaultValue); err != nil {
			return nil, nil, errors.Wrap(err, "failed to reset camera user control")
		}
		if controlName == pb.Control_WHITE_BALANCE {
			if err := setCameraUserControl(ctx, videoNode, whiteBalanceAutomaticControlKey, 1); err != nil {
				return nil, nil, errors.Wrapf(err, "failed to reset %s", whiteBalanceAutomaticControlKey)
			}
		}
	}
	return controlFunctionality, imageConfigs, nil
}

func augmentReportWithFunctionality(ctx context.Context, reportPath string, controlFunctionality map[vidPidType]map[pb.Control]bool) error {
	pbtxtData, err := os.ReadFile(reportPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read the report file: %s", reportPath)
	}
	reports := &pb.ManualControlReports{}
	if err := prototext.Unmarshal(pbtxtData, reports); err != nil {
		return errors.Wrapf(err, "failed to unmarshal the report: %s", reportPath)
	}

	for _, report := range reports.Reports {
		if functionailityInnerMap, ok := controlFunctionality[report.VidPid]; ok {
			if functionaility, ok := functionailityInnerMap[report.ControlName]; ok {
				report.CommandFunctionalityTestResult = functionaility
			} else {
				return errors.Wrapf(err, "failed to find control name key %s in the map", report.ControlName)
			}
		} else {
			return errors.Wrapf(err, "failed to find vid:pid key %s in the map", report.VidPid)
		}
	}

	opts := prototext.MarshalOptions{Multiline: true, Indent: "  "}
	reportByte, err := opts.Marshal(reports)
	if err != nil {
		return errors.Wrap(err, "failed to marshal report")
	}
	if err := os.WriteFile(reportPath, reportByte, 0644); err != nil {
		return errors.Wrapf(err, "failed to write to path %s:", reportPath)
	}
	return nil
}

func checkFacing(ctx context.Context, app *cca.App, requiredFacing camera.Facing) (bool, error) {
	facing, err := app.GetFacing(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get facing from cca")
	}
	switch facing {
	case cca.FacingBack:
		return requiredFacing == camera.Facing_FACING_BACK, nil
	case cca.FacingFront:
		return requiredFacing == camera.Facing_FACING_FRONT, nil
	default:
		return false, nil
	}
}

func removeImagesInDir(outDir string) error {
	return filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.Wrapf(err, "failed to walk through the directory: %s", outDir)
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".jpg") {
			if err := os.Remove(path); err != nil {
				return errors.Wrapf(err, "failed to remove file: %s", path)
			}
		}
		return nil
	})
}

type ManualControlUSBCameraService struct {
	s *testing.ServiceState
}

func (f *ManualControlUSBCameraService) ValidateControl(ctx context.Context, req *camera.ValidateControlRequest) (*empty.Empty, error) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := upstart.RestartJob(cleanupCtx, "cros-camera"); err != nil {
		return nil, errors.Wrap(err, "failed to restart cros-camera service")
	}

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return nil, errors.New("failed to get remote output directory")
	}

	// Start CCA
	cr, err := chrome.New(ctx)
	if err != nil {
		return nil, err
	}
	tb, err := testutil.NewTestBridge(ctx, cr, testutil.UseRealCamera)
	if err != nil {
		return nil, errors.Wrap(err, "failed to construct test bridge")
	}
	defer tb.TearDown(cleanupCtx)
	if err := cca.ClearSavedDir(ctx, cr); err != nil {
		return nil, errors.Wrap(err, "failed to clear saved directory")
	}
	app, err := cca.New(ctx, cr, outDir, tb)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open CCA")
	}
	defer app.Close(cleanupCtx)

	// Traverse USB cameras and toggle controls to take photos.
	usbCameraList, err := testutil.USBCamerasFromV4L2Test(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get usb camera list")
	}

	controlFunctionality := make(controlFunctionalityMap)
	var imageConfigs pb.ManualControlImageConfigs
	imageConfigs.ReportOutputPath = fmt.Sprintf("%s/%s", outDir, reportFileName)

	for _, videoNode := range usbCameraList {
		defer func() {
			if err := resetCameraUserControl(ctx, videoNode); err != nil {
				testing.ContextLogf(ctx, "Failed to reset all camera user controls of videoNode %s", videoNode)
			}
		}()
		usbCameraVersion, err := testutil.GetUsbCameraVersion(ctx, videoNode)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get usb camera version from videoNode %s", videoNode)
		}
		vidPid := usbCameraVersion.IDVendor + ":" + usbCameraVersion.IDProduct

		if err := switchCameraByVidPid(ctx, vidPid, app); err != nil {
			return nil, errors.Wrapf(err, "failed to find camera with vid:pid=%s", vidPid)
		}

		if correctFacing, err := checkFacing(ctx, app, req.Facing); err != nil {
			return nil, errors.Wrap(err, "failed to check facing")
		} else if !correctFacing {
			continue
		}

		subControlFunctionality, imageConfigList, err := controlCameraByVideoNode(ctx, videoNode, vidPid, req.SaveImage, app, outDir)
		controlFunctionality[vidPid] = subControlFunctionality
		imageConfigs.Configs = append(imageConfigs.Configs, imageConfigList...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to control camera with video node %s", videoNode)
		}
	}

	if err := validateControlByImages(ctx, outDir, req.CameraUserControlValidateScriptPath, &imageConfigs); err != nil {
		return nil, errors.Wrapf(err, "failed to run script %s", req.CameraUserControlValidateScriptPath)
	}

	// Remove images if not needed.
	if !req.SaveImage {
		if err := removeImagesInDir(outDir); err != nil {
			return nil, errors.Wrapf(err, "failed to remove images in directory: %s", outDir)
		}
	}

	reportPath := fmt.Sprintf("%s/%s", outDir, reportFileName)
	if err := augmentReportWithFunctionality(ctx, reportPath, controlFunctionality); err != nil {
		return nil, errors.Wrapf(err, "failed to augment report %s with functionaility", reportPath)
	}

	return &empty.Empty{}, nil
}
