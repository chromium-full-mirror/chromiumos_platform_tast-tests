// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pre

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/android/adb"
	"go.chromium.org/tast-tests/cros/common/testexec"
	remoteadb "go.chromium.org/tast-tests/cros/remote/android/adb"
	pb "go.chromium.org/tast-tests/cros/services/cros/camerabox"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

// To uprev |ctsVerifierX86Zip| and |ctsVerifierArmZip|, download the new zip
// from https://source.android.com/compatibility/cts/downloads, replace old zip
// under data folder and check the test can still pass.
const (
	ctsVerifierRoot = "android-cts-verifier"

	// CtsVerifierX86Zip is data path to ITS bundle testing x86 compatible platform.
	CtsVerifierX86Zip = "its/android-cts-verifier-9.0_r15-linux_x86-x86.zip"

	// CtsVerifierArmZip is data path to ITS bundle testing arm compatible platform.
	CtsVerifierArmZip = "its/android-cts-verifier-9.0_r15-linux_x86-arm.zip"

	// CtsRVerifierX86Zip is data path to ITS bundle testing x86 compatible platform.
	CtsRVerifierX86Zip = "its/android-cts-verifier-11_r12-linux_x86-x86.zip"

	// CtsRVerifierArmZip is data path to ITS bundle testing arm compatible platform.
	CtsRVerifierArmZip = "its/android-cts-verifier-11_r12-linux_x86-arm.zip"

	// CtsVerifierX86Zip is data path to ITS bundle testing x86 compatible platform.
	CtsTVerifierX86Zip = "its/android-cts-verifier-13_r4-linux_x86-x86.zip"

	// CtsVerifierArmZip is data path to ITS bundle testing arm compatible platform.
	CtsTVerifierArmZip = "its/android-cts-verifier-13_r4-linux_x86-arm.zip"

	//NumpySrcTarGz is numpy source package
	NumpySrcTarGz = "its/numpy-1.24.4.tar.gz"

	//ConfigYml is the config.ym for ITS
	ConfigYml = "its/config.yml"

	// ITSPy3Patch is the data path of py2 to py3 patch for ITS test
	// scripts. Update the script content with the steps:
	// $ python3 setup_its_repo.py android-cts-verifier-XXX.zip
	// $ cd android-cts-verifier/CameraITS
	// # Do modification to *.py
	// $ git diff base > <Path to this patch>
	ITSPy3Patch = "its/its.patch"

	// SetupITSRepoScript is the data path of the script unpacking ITS
	// bundle and apply python3 patches.
	SetupITSRepoScript = "its/setup_its_repo.py"

	// ChartPath is the path of the displayed chart
	ChartPath = "its/google.png"
)

type bundleAbi string
type androidCodeName string

const (
	x86      bundleAbi = "x86"
	arm                = "arm"
	androidP           = "androidP"
	androidR           = "androidR"
	androidT           = "androidT"
)

func (abi bundleAbi) bundlePath() (string, error) {
	switch abi {
	case x86:
		return CtsVerifierX86Zip, nil
	case arm:
		return CtsVerifierArmZip, nil
	}
	return "", errors.Errorf("cannot get bundle path of unknown abi %v", abi)
}

// itsPreImpl implements testing.Precondition.
type itsPreImpl struct {
	cl              *rpc.Client
	itsCl           pb.ITSServiceClient
	abi             bundleAbi
	dir             string
	oldEnvPath      string
	hostname        string
	adbDevice       *adb.Device
	prepared        bool
	androidCodeName androidCodeName
	defaultPy3Path  string
}

// ITSHelper provides helper functions accessing ITS package and mandating ARC.
type ITSHelper struct {
	p *itsPreImpl
}

// ITSX86Pre is the test precondition to run Android x86 ITS test.
var ITSX86Pre = &itsPreImpl{abi: x86, androidCodeName: androidP}

// ITSArmPre is the test precondition to run Android x86-arm ITS test.
var ITSArmPre = &itsPreImpl{abi: arm, androidCodeName: androidP}

// RITSX86Pre is the test precondition to run Android R x86 ITS test.
var RITSX86Pre = &itsPreImpl{abi: x86, androidCodeName: androidR}

// RITSArmPre is the test precondition to run Android R x86-arm ITS test.
var RITSArmPre = &itsPreImpl{abi: arm, androidCodeName: androidR}

// TITSX86Pre is the test precondition to run Android T x86 ITS test.
var TITSX86Pre = &itsPreImpl{abi: x86, androidCodeName: androidT}

// TITSArmPre is the test precondition to run Android T x86-arm ITS test.
var TITSArmPre = &itsPreImpl{abi: arm, androidCodeName: androidT}

func (p *itsPreImpl) String() string         { return fmt.Sprintf("its_%s_precondition", p.abi) }
func (p *itsPreImpl) Timeout() time.Duration { return 5 * time.Minute }

func copyFile(src, dst string, perm os.FileMode) error {
	content, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(dst, content, perm)
}

func itsUnzip(ctx context.Context, zipPath, outDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return errors.Wrap(err, "failed to open ITS zip file")
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		src, err := f.Open()
		if err != nil {
			return errors.Wrapf(err, "failed to open file %v in ITS zip", f.Name)
		}
		defer src.Close()
		dstPath := path.Join(outDir, f.Name)
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return errors.Wrapf(err, "failed to create directory for unzipped ITS file %v", f.Name)
		}
		dst, err := os.Create(dstPath)
		if err != nil {
			return errors.Wrapf(err, "failed to create file for copying ITS file %v", f.Name)
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			return errors.Wrapf(err, "failed to copy ITS file %v", f.Name)
		}
	}
	return nil
}

func (p *itsPreImpl) Prepare(ctx context.Context, s *testing.PreState) interface{} {
	if p.prepared {
		return &ITSHelper{p}
	}

	d := s.DUT()
	// Connect to the gRPC server on the DUT.
	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the HAL3 service on the DUT: ", err)
	}
	p.cl = cl

	// Set up ARC on DUT.
	itsClient := pb.NewITSServiceClient(cl.Conn)
	_, err = itsClient.SetUp(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Remote call Setup() failed: ", err)
	}
	p.itsCl = itsClient

	// Prepare temp bin dir.
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		s.Fatal("Failed to create a temp dir for extra binaries: ", err)
	}
	p.dir = tempDir
	p.oldEnvPath = os.Getenv("PATH")
	os.Setenv("PATH", p.dir+":"+p.oldEnvPath)

	// Prepare ADB downloaded from fixed url without versioning (Same
	// strategy as CTS), may consider associate proper version in
	// tast-build-deps.
	if err := copyFile(s.DataPath("adb"), path.Join(p.dir, "adb"), 0755); err != nil {
		s.Fatal("Failed to copy adb binary: ", err)
	}

	p.hostname = d.HostName()
	if err := remoteadb.LaunchServer(ctx); err != nil {
		s.Fatal("Failed to launch adb server: ", err)
	}

	testing.ContextLog(ctx, "ADB connect to DUT")
	adbDevice, err := adb.Connect(ctx, p.hostname, 30*time.Second)
	if err != nil {
		s.Fatal("Failed to set up adb connection to DUT: ", err)
	}
	p.adbDevice = adbDevice

	// Unpack ITS bundle.
	if err != nil {
		s.Fatal("Failed to get bundle path: ", err)
	}

	if err := testexec.CommandContext(ctx, "unzip", "-d", p.dir, s.DataPath(p.bundlePath())).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to unzip its package: ", err)
	}

	if p.androidCodeName == androidT {
		if err := testexec.CommandContext(
			ctx, "cp", s.DataPath(ConfigYml), p.itsRoot()).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to move config.yml: ", err)
		}
		configYml := fmt.Sprintf("%s/config.yml", p.itsRoot())
		if err := testexec.CommandContext(
			ctx, "sudo", "sed", "-i", fmt.Sprintf("s/%s/%s/g", "<device-id>", p.hostname), configYml).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Failed to write device_id into config.yml: ", err)
		}
	}

	// Install CTSVerifier apk.
	ctsVerifierRootPath := path.Join(p.dir, ctsVerifierRoot)
	verifierAPK := path.Join(ctsVerifierRootPath, "CtsVerifier.apk")
	if err := p.adbDevice.Command(ctx, "install", "-r", "-g", verifierAPK).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to install CTSVerifier: ", err)
	}

	p.prepared = true
	return &ITSHelper{p}
}

func (p *itsPreImpl) itsRoot() string {
	return path.Join(p.dir, ctsVerifierRoot, "CameraITS")
}

func (p *itsPreImpl) Close(ctx context.Context, s *testing.PreState) {
	if len(p.oldEnvPath) > 0 {
		if err := os.Setenv("PATH", p.oldEnvPath); err != nil {
			s.Errorf("Failed to restore environment variable PATH %v: %v", p.oldEnvPath, err)
		}
	}
	if len(p.dir) > 0 {
		if err := os.RemoveAll(p.dir); err != nil {
			s.Errorf("Failed to remove temp directory %v: %v", p.dir, err)
		}
	}
	if p.itsCl != nil {
		if _, err := p.itsCl.TearDown(ctx, &empty.Empty{}); err != nil {
			s.Error("Failed to call remote its TearDown(): ", err)
		}
	}
	if p.cl != nil {
		p.cl.Close(ctx)
	}
	p.prepared = false
}

// PrepareEnvironment prepare the environment for running ITS
func (h *ITSHelper) PrepareEnvironment(ctx context.Context, numpyPath string) (string, error) {
	retStr := ""
	createPy3VenvCmd := fmt.Sprintf("python3 -m venv %s/py3venv --copies", h.p.itsRoot())

	out, err := testexec.CommandContext(ctx, "bash", "-c", createPy3VenvCmd).Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "Fail to create python venv")
	}
	retStr += string(out)

	pipCmd := fmt.Sprintf("%s/py3venv/bin/pip", h.p.itsRoot())
	out, err = testexec.CommandContext(ctx, pipCmd, "install", numpyPath).Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "Fail to install numpy")
	}
	retStr += string(out)

	out, err = testexec.CommandContext(ctx, pipCmd, "install", "opencv-python==3.4.8.29").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "Fail to install opencv-python")
	}
	retStr += string(out)

	out, err = testexec.CommandContext(ctx, pipCmd, "install", "matplotlib>=3.3.2,<4.0").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "Fail to install matplotlib")
	}
	retStr += string(out)

	out, err = testexec.CommandContext(ctx, pipCmd, "install", "scipy==1.5.2").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "Fail to install scipy")
	}
	retStr += string(out)

	out, err = testexec.CommandContext(ctx, pipCmd, "install", "pyserial>=3.5,<4.0").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "Fail to install pyserial")
	}
	retStr += string(out)

	out, err = testexec.CommandContext(ctx, pipCmd, "install", "Pillow>=8.1.0,<9.0").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "Fail to install Pillow")
	}
	retStr += string(out)

	out, err = testexec.CommandContext(ctx, pipCmd, "install", "PyYAML>=5.3.1,<6.0").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "Fail to install PyYAML")
	}
	retStr += string(out)

	out, err = testexec.CommandContext(ctx, pipCmd, "install", "mobly").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "Fail to install mobly")
	}
	retStr += string(out)

	envsetup := fmt.Sprintf("%s/build/envsetup.sh", h.p.itsRoot())
	py3Cmd := fmt.Sprintf("%s/py3venv/bin/python3", h.p.itsRoot())

	if err := testexec.CommandContext(
		ctx, "sudo", "sed", "-i", fmt.Sprintf("s#%s#%s#g", "Require python opencv", "Require Python opencv"), envsetup).Run(testexec.DumpLogOnError); err != nil {
		return "", errors.Wrap(err, "failed to replace python path in envsetup.sh")
	}
	if err := testexec.CommandContext(
		ctx, "sudo", "sed", "-i", fmt.Sprintf("s#%s#%s#g", "python", py3Cmd), envsetup).Run(testexec.DumpLogOnError); err != nil {
		return "", errors.Wrap(err, "failed to replace python path in envsetup.sh")
	}

	runAllTestsPY := fmt.Sprintf("%s/tools/run_all_tests.py", h.p.itsRoot())
	if err := testexec.CommandContext(
		ctx, "sudo", "sed", "-i", fmt.Sprintf("s#%s#%s#g", "python3", py3Cmd), runAllTestsPY).Run(testexec.DumpLogOnError); err != nil {
		return "", errors.Wrap(err, "failed to replace python path in run_all_tests.py")
	}

	runSensorFusionPY := fmt.Sprintf("%s/tools/run_sensor_fusion.py", h.p.itsRoot())
	if err := testexec.CommandContext(
		ctx, "sudo", "sed", "-i", fmt.Sprintf("s#%s#%s#g", "python", py3Cmd), runSensorFusionPY).Run(testexec.DumpLogOnError); err != nil {
		return "", errors.Wrap(err, "failed to replace python path in run_sensor_fusion.py")
	}

	os.Setenv("PATH", os.Getenv("PATH")+fmt.Sprintf(":%s/py3venv/bin/", h.p.itsRoot()))
	os.Setenv("MPLCONFIGDIR", fmt.Sprintf("%s/tmp", h.p.itsRoot()))

	out, err = testexec.CommandContext(ctx, "adb", "shell", "am", "compat", "enable", "ALLOW_TEST_API_ACCESS", "com.android.cts.verifier").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "Fail to enable ALLOW_TEST_API_ACCESS")
	}
	retStr += string(out)

	return retStr, nil
}

// TestCmd returns command to run test scene with camera id.
func (h *ITSHelper) TestCmd(ctx context.Context, scene, camera int) *testexec.Cmd {
	setupPath := path.Join("build", "envsetup.sh")
	scriptPath := path.Join("tools", "run_all_tests.py")
	py3Cmd := fmt.Sprintf("%s/py3venv/bin/python3", h.p.itsRoot())
	cmdStr := fmt.Sprintf(`cd %s
	chmod -R 755 .
	source %s
	%s %s device=%s scenes=%d camera=%d skip_scene_validation`,
		h.p.itsRoot(), setupPath, py3Cmd, scriptPath, h.p.hostname, scene, camera)
	cmd := testexec.CommandContext(ctx, "bash", "-c", cmdStr)
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=y")
	return cmd
}

// Chart returns scene chart to run test scene.
func (h *ITSHelper) Chart(scene int) string {
	s := fmt.Sprintf("scene%d", scene)
	return path.Join(h.p.itsRoot(), "tests", s, s+".pdf")
}

// CameraID returns corresponding camera id of camera facing on DUT.
func (h *ITSHelper) CameraID(ctx context.Context, facing pb.Facing) (int, error) {
	out, err := h.p.adbDevice.Command(ctx, "shell", "pm", "list", "features").Output(testexec.DumpLogOnError)
	if err != nil {
		return -1, errors.Wrap(err, "failed to list features on ARC")
	}
	var front, back bool
	for _, feature := range strings.Split(string(out), "\n") {
		if feature == "feature:android.hardware.camera.front" {
			front = true
		} else if feature == "feature:android.hardware.camera" {
			back = true
		}
	}
	if (facing == pb.Facing_FACING_BACK && !back) || (facing == pb.Facing_FACING_FRONT && !front) {
		return -1, errors.Errorf("cannot run test on DUT without %s facing camera", facing)
	}
	if back && front && facing != pb.Facing_FACING_BACK {
		return 1, nil
	}
	return 0, nil
}

func (p *itsPreImpl) bundlePath() string {
	if p.androidCodeName == androidP {
		if p.abi == x86 {
			return CtsVerifierX86Zip
		} else if p.abi == arm {
			return CtsVerifierArmZip
		} else {
			return ""
		}
	} else if p.androidCodeName == androidR {
		if p.abi == x86 {
			return CtsRVerifierX86Zip
		} else if p.abi == arm {
			return CtsRVerifierArmZip
		} else {
			return ""
		}
	} else if p.androidCodeName == androidT {
		if p.abi == x86 {
			return CtsTVerifierX86Zip
		} else if p.abi == arm {
			return CtsTVerifierArmZip
		} else {
			return ""
		}
	} else {
		return ""
	}
}
