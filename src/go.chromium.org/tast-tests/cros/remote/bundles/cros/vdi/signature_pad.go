// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vdi

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/robotics/arm/amber"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/vdi/citrix"
	ps "go.chromium.org/tast-tests/cros/services/cros/policy"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/vdi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

type signaturePadTestParams struct {
	appName          citrix.AppName
	appTitle         string
	appIcon          string
	motionData       string
	deviceName       string
}

var signaturePadData = append(citrix.CitrixData, citrix.SignaturePadData...)

func init() {
	testing.AddTest(&testing.Test{
		Func: SignaturePad,
		Desc: "Setup of eSignature device, sign/save/clear/load signature from Citrix remote desktop",
		Contacts: []string{
			"cros-ent-peripherals-team@google.com",
			"rzakarian@google.com",
			"sudhirperka@google.com",
			"cienet-development@googlegroups.com",
			"chicheny@google.com",
		},
		BugComponent: "b:885494", // ChromeOS > Platform > Enablement > Services > Peripherals
		Timeout:      15 * time.Minute,
		ServiceDeps: []string{
			"tast.cros.policy.PolicyService",
			"tast.cros.vdi.CitrixService",
			"tast.cros.ui.ScreenRecorderService",
			citrix.FaillogServiceName,
		},
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"vdi.ota_citrix_username",
			"vdi.ota_citrix_password",
			"vdi.record_screen",
			"vdi.manual_test",
		},
		Data: signaturePadData,
		Params: []testing.Param{
			{
				Name:      "scriptel",
				ExtraData: []string{citrix.ScriptelMotionData},
				Val: signaturePadTestParams{
					appName:    citrix.ScriptelAppName,
					appTitle:   citrix.ScriptelAppTitle,
					appIcon:    citrix.ScriptelAppIcon,
					motionData: citrix.ScriptelMotionData,
				},
			},
			{
				Name:      "topaz",
				ExtraData: []string{citrix.TopazMotionData},
				Val: signaturePadTestParams{
					appName:    citrix.TopazAppName,
					appTitle:   citrix.TopazAppTitle,
					motionData: citrix.TopazMotionData,
					deviceName: citrix.TopazDeviceName,
				},
			},
		},
		Fixture: fixture.CleanOwnership,
	})
}

func SignaturePad(ctx context.Context, s *testing.State) {
	testParams := s.Param().(signaturePadTestParams)
	otaUsername := s.RequiredVar("vdi.ota_citrix_username")
	otaPassword := s.RequiredVar("vdi.ota_citrix_password")

	// Prepare data path on DUT.
	d := s.DUT()
	dataPath, err := citrix.CopyFilesToRemote(ctx, s, d, signaturePadData)
	if err != nil {
		s.Fatal("Failed to put files to remote: ", err)
	}
	defer d.Conn().CommandContext(ctx, "rm", "-r", dataPath).Output()

	// Manually enroll into target organization.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect the DUT before manual enrollment: ", err)
	}
	defer cl.Close(ctx)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	policyClient := ps.NewPolicyServiceClient(cl.Conn)
	if _, err := policyClient.GAIAEnrollAndLoginUsingChrome(ctx, &ps.GAIAEnrollAndLoginUsingChromeRequest{
		Username:    otaUsername,
		Password:    otaPassword,
		DmserverURL: policy.DMServerProdURL,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	defer policyClient.StopChrome(cleanupCtx, &empty.Empty{})

	recordScreen := false
	if val, ok := s.Var("vdi.record_screen"); ok {
		if recordScreen, err = strconv.ParseBool(val); err != nil {
			s.Fatal("Failed to parse argument 'vdi.record_screen' of type bool: ", err)
		}
	}
	if recordScreen {
		s.Log("Screen recorder started")

		filePath := filepath.Join(s.OutDir(), "signature.webm")
		startRequest := pb.StartRequest{
			FileName: filePath,
		}
		screenRecorder := pb.NewScreenRecorderServiceClient(cl.Conn)
		if _, err := screenRecorder.Start(ctx, &startRequest); err != nil {
			s.Fatal("Failed to start recording: ", err)
		}
		defer func(ctx context.Context) {
			res, err := screenRecorder.Stop(ctx, &empty.Empty{})
			if err != nil {
				s.Log("Failed to stop recording: ", err)
			} else {
				s.Logf("Screen recording saved to %s", res.FileName)

				s.Log("Copying screen recording from DUT to local machine")
				destPath := filepath.Join(s.OutDir(), filepath.Base(res.FileName))
				if err := linuxssh.GetFile(ctx, s.DUT().Conn(), res.FileName, destPath, linuxssh.DereferenceSymlinks); err != nil {
					s.Log("Failed to copy screen recording to local machine: ", err)
				}
			}
		}(cleanupCtx)
	}

	citrixSvc := vdi.NewCitrixServiceClient(cl.Conn)
	if _, err := citrixSvc.NewCitrix(ctx, &vdi.NewCitrixRequest{
		DataPath: dataPath,
	}); err != nil {
		s.Fatal("Failed to create new Citrix: ", err)
	}

	defer func(ctx context.Context) {
		citrix.DumpUITreeWithScreenshotToFile(ctx, cl.Conn, s.HasError, "ui_dump")
		if _, err := citrixSvc.CloseCitrix(ctx, &empty.Empty{}); err != nil {
			s.Log("Failed to close Citrix app: ", err)
		}
	}(cleanupCtx)

	if _, err := citrixSvc.LoginCitrix(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to login Citrix: ", err)
	}

	manualTest := false
	if val, ok := s.Var("vdi.manual_test"); ok {
		manualTest, err = strconv.ParseBool(val)
		if err != nil {
			s.Fatal("Failed to parse argument 'vdi.manual_test' of type bool: ", err)
		}
	}

	if err := performSignatureOperations(ctx, cl, citrixSvc, s.DataPath, testParams, manualTest); err != nil {
		s.Fatal("Failed to perform signature operations: ", err)
	}

}

// performSignatureOperations opens signature app in Citrix, starts to sign,
// save, clear, load signature and verify the signatures are expected.
func performSignatureOperations(ctx context.Context, cl *rpc.Client, citrixSvc vdi.CitrixServiceClient, dataPath func(string) string, params signaturePadTestParams, manualTest bool) (retErr error) {
	const (
		fileName      = "mySign.SIG"
		startFileName = "start.png"
		signFileName  = "signature.png"
		clearFileName = "clear.png"
		loadFileName  = "load.png"
	)
	appName := params.appName
	appTitle := params.appTitle
	appIcon := params.appIcon
	deviceName := params.deviceName
	motionData := params.motionData

	// Only Topaz devices require reconnecting the USB device.
	// Scriptel device will be connected automatically.
	if appName == citrix.TopazAppName {
		if _, err := citrixSvc.ConnectUSBDevice(ctx, &vdi.ConnectUSBDeviceRequest{
			DeviceName: deviceName,
		}); err != nil {
			return errors.Wrap(err, "failed to connect USB device")
		}
	}

	deleteFileRequest := &vdi.DeleteFileRequest{
		FileName: fileName,
	}
	if _, err := citrixSvc.DeleteFileIfExists(ctx, deleteFileRequest); err != nil {
		return errors.Wrap(err, "failed to delete file if exists")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// Open Citrix app.
	if _, err := citrixSvc.OpenCitrixApp(ctx, &vdi.OpenCitrixAppRequest{
		AppName:  string(appName),
		AppTitle: appTitle,
		AppIcon:  appIcon,
	}); err != nil {
		return errors.Wrapf(err, "failed to open Citrix app %v", appName)
	}
	isSavedAlready := false
	defer func(ctx context.Context) {
		if isSavedAlready {
			if _, err := citrixSvc.DeleteFile(ctx, deleteFileRequest); err != nil {
				testing.ContextLog(ctx, "Failed to delete file: ", err)
			}
		}
	}(cleanupCtx)

	// Start to sign signature.
	if _, err := citrixSvc.StartSignature(ctx, &vdi.StartSignatureRequest{
		AppName: string(appName),
	}); err != nil {
		return errors.Wrap(err, "failed to start signature")
	}

	if _, err := citrixSvc.SaveCropScreenshot(ctx, &vdi.SaveCropScreenshotRequest{
		FileName: startFileName,
	}); err != nil {
		return errors.Wrapf(err, "failed to save crop sceenshot to %s", startFileName)
	}

	if !manualTest {
		if err := moveRoboticArmToSign(ctx, dataPath, motionData); err != nil {
			return errors.Wrap(err, "failed to move robotic arm to sign")
		}
		// GoBigSleepLint: Waiting for robot arm to sign.
		if err := testing.Sleep(ctx, 10*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
	} else {
		testing.ContextLog(ctx, "Please sign manually within 20 seconds")
		// GoBigSleepLint: Sign manually within 20 seconds
		if err := testing.Sleep(ctx, 20*time.Second); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
	}

	if err := waitForSignatureToLoad(ctx, citrixSvc, startFileName, signFileName); err != nil {
		return errors.Wrap(err, "failed to wait for signature to load")
	}

	signatureRequest := &vdi.SignatureRequest{
		FileName: fileName,
	}

	// Save signature.
	if _, err := citrixSvc.SaveSignature(ctx, signatureRequest); err != nil {
		return errors.Wrap(err, "failed to save signature")
	}
	isSavedAlready = true

	// Clear signature.
	if _, err := citrixSvc.ClearSignature(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to clear signature")
	}

	if _, err := citrixSvc.SaveCropScreenshot(ctx, &vdi.SaveCropScreenshotRequest{
		FileName: clearFileName,
	}); err != nil {
		return errors.Wrapf(err, "failed to save crop sceenshot to %s", clearFileName)
	}

	if _, err := citrixSvc.VerifyTwoImagesSimilarity(ctx, &vdi.VerifyTwoImagesSimilarityRequest{
		FileName1:    clearFileName,
		FileName2:    signFileName,
		ExpectedSame: false,
	}); err != nil {
		return errors.Wrapf(err, "signatures %s and %s are the same and expected to be different", clearFileName, signFileName)
	}

	// Load signature.
	if _, err := citrixSvc.LoadSignature(ctx, signatureRequest); err != nil {
		return errors.Wrap(err, "failed to load signature")
	}
	switch appName {
	case citrix.TopazAppName:
		if _, err := citrixSvc.SaveCropScreenshot(ctx, &vdi.SaveCropScreenshotRequest{
			FileName: loadFileName,
		}); err != nil {
			return errors.Wrapf(err, "failed to save crop sceenshot to %s", loadFileName)
		}
		if _, err := citrixSvc.VerifyTwoImagesSimilarity(ctx, &vdi.VerifyTwoImagesSimilarityRequest{
			FileName1:    loadFileName,
			FileName2:    signFileName,
			ExpectedSame: true,
		}); err != nil {
			return errors.Wrapf(err, "signatures %s and %s are different and expected to be the same", loadFileName, signFileName)
		}
	case citrix.ScriptelAppName:
		if _, err := citrixSvc.WaitUntilIconExists(ctx, &vdi.WaitUntilIconExistsRequest{IconName: signFileName}); err != nil {
			return errors.Wrap(err, "failed to verify that the signature has been loaded")
		}
	}

	return nil
}

func moveRoboticArmToSign(ctx context.Context, dataPath func(string) string, motionData string) error {
	const moveDuration = 50 * time.Millisecond

	cleanupArmCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	arm, err := amber.NewArm(amber.DefaultReadWriteTimeout)
	if err != nil {
		return errors.Wrap(err, "failed to create arm helper")
	}
	defer arm.Close()
	defer arm.MoveToInitialPosition(cleanupArmCtx)

	positions, err := amber.ParsePositionsFromCSV(dataPath(motionData))
	if err != nil {
		return errors.Wrap(err, "failed to read signature pad movement data from file")
	}
	testing.ContextLog(ctx, "Start to move robotic arm to sign")
	if err := arm.MultiMove(ctx, positions, moveDuration); err != nil {
		return errors.Wrap(err, "failed to move robotic arm to sign")
	}

	// GoBigSleepLint: Waiting for robotic arm to sign.
	if err := testing.Sleep(ctx, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}

	return nil
}

// waitForSignatureToLoad waits for the signature to load.
// It captures screenshots and compares them to ensure that the signature has changed
// from the initial state to a loaded state.
func waitForSignatureToLoad(ctx context.Context, citrixSvc vdi.CitrixServiceClient, startFileName, signFileName string) error {
	const lastFileName = "last_signature.png"
	loadingSignature := false
	testing.ContextLog(ctx, "Start waiting for signature to load")
	return testing.Poll(ctx, func(ctx context.Context) error {
		saveLastFileCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancel()
		defer func(ctx context.Context) {
			if loadingSignature {
				if _, err := citrixSvc.SaveCropScreenshot(ctx, &vdi.SaveCropScreenshotRequest{
					FileName: lastFileName,
				}); err != nil {
					testing.ContextLogf(ctx, "Failed to save crop sceenshot to %s", lastFileName)
				}
			}
		}(saveLastFileCtx)

		if _, err := citrixSvc.SaveCropScreenshot(ctx, &vdi.SaveCropScreenshotRequest{
			FileName: signFileName,
		}); err != nil {
			return errors.Wrapf(err, "failed to save crop sceenshot to %s", signFileName)
		}
		if !loadingSignature {
			// Check whether the initial canvas and the signed canvas are different.
			// If yes, it means that the signature has started to be loaded.
			// If not, it will wait for loading.
			if _, err := citrixSvc.VerifyTwoImagesSimilarity(ctx, &vdi.VerifyTwoImagesSimilarityRequest{
				FileName1:    startFileName,
				FileName2:    signFileName,
				ExpectedSame: false,
			}); err != nil {
				return errors.Wrapf(err, "signatures %s and %s are the same and expected to be different", startFileName, signFileName)
			}
			loadingSignature = true
			return errors.New("waiting for the complete signature")
		}
		// If the signFile and the lastFile are the same, it means the signature
		// has been loaded. If not, it means the signature is still loading.
		if _, err := citrixSvc.VerifyTwoImagesSimilarity(ctx, &vdi.VerifyTwoImagesSimilarityRequest{
			FileName1:    signFileName,
			FileName2:    lastFileName,
			ExpectedSame: true,
		}); err != nil {
			return errors.Wrapf(err, "signatures %s and %s are different and expected to be same", signFileName, lastFileName)
		}
		return nil
	}, &testing.PollOptions{Interval: 5 * time.Second, Timeout: time.Minute})
}
