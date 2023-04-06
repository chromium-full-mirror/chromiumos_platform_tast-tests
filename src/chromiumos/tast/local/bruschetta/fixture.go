// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bruschetta

import (
	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/cryptohome"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/local/policyutil/fixtures"
	"chromiumos/tast/local/terminalapp"
	"chromiumos/tast/local/vm"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	installationTimeout   = 15 * time.Minute
	resetTimeout          = time.Minute
	postTestTimeout       = 30 * time.Second
	uninstallationTimeout = 2 * time.Minute

	chronosUID = 1000
	crosvmUID  = 299

	// testOemString is an OEM string set in the VM for these tests.
	testOemString = "OEM string set by tast test"

	// referenceVMInstaller is the installer image for the reference VM.
	referenceVMInstaller = "refvm.img.zst"
	// referenceVMPflash is the pflash image for the reference VM.
	referenceVMPflash = "refvm_VARS.fd"

	imageInstallPath  = "crosvm/YnJ1.img"
	pflashInstallPath = "crosvm/YnJ1.pflash"

	// BruschettaFixture is the name of the fixture defined in this file.
	BruschettaFixture = "bruschettaReferenceVM"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            BruschettaFixture,
		Desc:            "Set up reference VM",
		Contacts:        []string{"sidereal@google.com", "clumptini+oncall@google.com"},
		Impl:            &bruschettaFixture{},
		SetUpTimeout:    installationTimeout + uninstallationTimeout,
		ResetTimeout:    resetTimeout,
		PostTestTimeout: postTestTimeout,
		TearDownTimeout: uninstallationTimeout,
		Data:            []string{referenceVMInstaller, referenceVMPflash},
		Parent:          fixture.ChromePolicyLoggedInBruschetta,
	})
}

// bruschettaFixture holds the runtime state of the fixture.
type bruschettaFixture struct {
	// fakeDMS is an already running DMS server.
	fakeDMS *fakedms.FakeDMS
	// chrome is a logged in chrome instance that loads policies from fakeDMS.
	chrome *chrome.Chrome
	// policy is the base-line policy all tests start with.
	policy *policy.BruschettaVMConfiguration
	// vm is the running VM instance.
	vm *vm.VM
	// tconn is the test connection to chrome.
	tconn *chrome.TestConn
	// How far into the VM log we've read.
	logOffset int
}

// FixtureData is the data returned by SetUp and passed to tests.
type FixtureData struct {
	// FakeDMS is an already running DMS server. At the start of each test it will
	// serve a baseline bruschetta policy which allows the VM to run. Modifying this
	// policy during a test is okay.
	FakeDMS *fakedms.FakeDMS
	// Chrome is a logged in chrome instance that loads policies from fakeDMS.
	Chrome *chrome.Chrome
	// VM is the running VM instance.
	VM *vm.VM
	// Tconn is the test connection to Chrome.
	Tconn *chrome.TestConn
}

func (f *bruschettaFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	f.fakeDMS = s.ParentValue().(*fixtures.FixtData).FakeDMS()
	f.chrome = s.ParentValue().(*fixtures.FixtData).Chrome()
	tconn, err := f.chrome.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	f.tconn = tconn

	s.Log("Computing chrome policy")

	imagePolicy, err := makeFilePolicy(s.DataPath(referenceVMInstaller))
	if err != nil {
		s.Fatal("Failed to generate policy for installer: ", err)
	}

	pflashPolicy, err := makeFilePolicy(s.DataPath(referenceVMPflash))
	if err != nil {
		s.Fatal("Failed to generate policy for pflash: ", err)
	}

	f.policy = &policy.BruschettaVMConfiguration{
		Stat: policy.StatusSet,
		Val: map[string]interface{}{
			"glinux-latest": map[string]interface{}{
				"name":                   "Test VM Configuration",
				"enabled_state":          "INSTALL_ALLOWED",
				"installer_image_x86_64": imagePolicy,
				"uefi_pflash_x86_64":     pflashPolicy,
				"vtpm": map[string]interface{}{
					"enabled":              true,
					"policy_update_action": "NONE",
				},
				"oem_strings": []interface{}{
					testOemString,
				},
			},
		},
	}

	if err := policyutil.ServeAndVerify(ctx, f.fakeDMS, f.chrome, []policy.Policy{f.policy}); err != nil {
		s.Fatal("Failed to serve bruschetta policy to chrome: ", err)
	}

	s.Log("Installing VM")

	// Because the graphical install flow is currently pretty dodgy, we don't actually
	// install the VM properly through chrome. Instead we set the BruschettaAlphaMigrate
	// feature in the parent fixture which makes chrome just assume there's a VM called "bru"
	// associated with the "glinux-latest" VM config. We will now create that VM by copying
	// the files into concierge's data directory.
	// TODO(281772103) Change that

	systemPath, err := cryptohome.SystemPath(ctx, f.chrome.User())
	if err != nil {
		s.Fatal("Couldn't find user's system directory: ", err)
	}

	if err := decompressVMImage(ctx, s.DataPath(referenceVMInstaller), path.Join(systemPath, imageInstallPath)); err != nil {
		s.Fatal("Failed to install VM image: ", err)
	}
	defer func() {
		if !s.HasError() {
			return
		}

		if err := os.Remove(path.Join(systemPath, imageInstallPath)); err != nil {
			s.Fatal("Failed to delete VM image after setup failure: ", err)
		}
	}()

	if err := copyPflashFile(ctx, s.DataPath(referenceVMPflash), path.Join(systemPath, pflashInstallPath)); err != nil {
		s.Fatal("Failed to install pflash file: ", err)
	}
	defer func() {
		if !s.HasError() {
			return
		}

		if err := os.Remove(path.Join(systemPath, pflashInstallPath)); err != nil {
			s.Fatal("Failed to delete pflash image after setup failure: ", err)
		}
	}()

	s.Log("Starting VM")

	concierge, err := vm.NewConcierge(ctx, f.chrome.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to restart concierge: ", err)
	}

	vm, err := vm.NewSystemRecognizedVM(concierge, false, 0, vm.Bruschetta)
	if err != nil {
		s.Fatal("Failed to get VM object: ", err)
	}
	f.vm = vm

	defer func(ctx context.Context) {
		if !s.HasError() {
			// No error, so no cleanup is required.
			return
		}

		if err := f.vm.Stop(ctx); err != nil {
			s.Error("Failed to stop VM after setup failure: ", err)
		}
		if err := f.vm.Delete(ctx); err != nil {
			s.Error("Failed to delete VM after setup failure: ", err)
		}
	}(ctx)

	// Skip past logs that might be left over from previous tests.
	existingLogs, err := f.vm.RetrieveLogs(ctx)
	if err != nil {
		s.Fatal("Failed to get existing VM logs: ", err)
	}
	f.logOffset = len(existingLogs)

	defer func(ctx context.Context) {
		if err := f.saveLogs(ctx, s.OutDir(), "setup"); err != nil {
			s.Fatal("Failed to save VM logs from fixture setup: ", err)
		}
	}(ctx)

	// Now use the terminal app to boot the VM.
	term, err := terminalapp.LaunchBruschetta(ctx, f.tconn)
	if err != nil {
		s.Fatal("Failed to start bruschetta VM using terminal app: ", err)
	}

	if err := term.Close()(ctx); err != nil {
		s.Fatal("Failed to close terminal app: ", err)
	}

	if err := concierge.GetVMInfo(ctx, f.vm); err != nil {
		s.Fatal("Failed to get running VM info: ", err)
	}

	return FixtureData{
		FakeDMS: f.fakeDMS,
		Chrome:  f.chrome,
		Tconn:   f.tconn,
		VM:      f.vm,
	}
}

func (f *bruschettaFixture) Reset(ctx context.Context) error {
	// Reset the policy in case a test changed it.
	if err := policyutil.ServeAndVerify(ctx, f.fakeDMS, f.chrome, []policy.Policy{f.policy}); err != nil {
		return errors.Wrap(err, "failed to serve bruschetta policy to chrome")
	}

	// Start up the VM, if it's not already running.
	term, err := terminalapp.LaunchBruschetta(ctx, f.tconn)
	if err != nil {
		return errors.Wrap(err, "failed to start bruschetta VM using terminal app")
	}
	if err := term.Close()(ctx); err != nil {
		return errors.Wrap(err, "failed to close terminal app")
	}

	return nil
}

func (f *bruschettaFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	return
}

func (f *bruschettaFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if err := f.saveLogs(ctx, s.OutDir(), "post_test"); err != nil {
		s.Error("Failed to save VM logs from test: ", err)
	}
}

func (f *bruschettaFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if err := f.vm.Stop(ctx); err != nil {
		s.Error("Failed to stop VM: ", err)
	}

	if err := f.vm.Delete(ctx); err != nil {
		s.Error("Failed to delete VM: ", err)
	}

	if err := f.saveLogs(ctx, s.OutDir(), "tear_down"); err != nil {
		s.Error("Failed to save VM logs from fixture teardown: ", err)
	}
}

func (f *bruschettaFixture) saveLogs(ctx context.Context, outdir, suffix string) error {
	logs, err := f.vm.RetrieveLogs(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get VM logs")
	}
	trimmedLogs := logs[f.logOffset:]

	// Set the log offset forward so we skip past this part in future tests.
	f.logOffset = len(logs)

	outFile, err := os.Create(filepath.Join(outdir, fmt.Sprintf("bruschetta_vm_%s.log", suffix)))
	if err != nil {
		return errors.Wrap(err, "failed to create log file")
	}
	defer outFile.Close()

	if _, err := outFile.WriteString(trimmedLogs); err != nil {
		return errors.Wrap(err, "failed to write to log file")
	}

	return nil
}

func hashDataFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", errors.Wrapf(err, "failed to open data file %q", path)
	}
	defer file.Close()

	sha := sha256.New()

	if _, err := io.Copy(sha, file); err != nil {
		return "", errors.Wrapf(err, "failed to hash data file %q", path)
	}

	return hex.EncodeToString(sha.Sum(nil)), nil
}

func makeFilePolicy(path string) (map[string]interface{}, error) {
	hash, err := hashDataFile(path)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"url":  "file://" + path,
		"hash": hash,
	}, nil
}

func decompressVMImage(ctx context.Context, srcPath, dstPath string) (retErr error) {
	// Defer cleanup first, because if zstd fails we won't know if it created the destination file or not.
	defer func() {
		if retErr != nil {
			if err := os.Remove(dstPath); err != nil {
				testing.ContextLog(ctx, "Failed to delete VM image file after error: ", err)
			}
		}
	}()

	if err := testexec.CommandContext(ctx, "zstd", "--decompress", "--sparse", srcPath, "-o", dstPath).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to decompress VM image")
	}

	file, err := os.Open(dstPath)
	if err != nil {
		return errors.Wrap(err, "failed to open VM image")
	}
	defer file.Close()

	if err := file.Chmod(fs.ModePerm); err != nil {
		return errors.Wrap(err, "failed to change permissions on VM image")
	}

	if err := file.Chown(crosvmUID, crosvmUID); err != nil {
		return errors.Wrap(err, "failed to change owner on VM image")
	}

	return nil
}

func copyPflashFile(ctx context.Context, srcPath, dstPath string) (retErr error) {
	dst, err := os.Create(dstPath)
	if err != nil {
		return errors.Wrap(err, "failed to create pflash file")
	}
	defer func() {
		if retErr != nil {
			if err := os.Remove(dstPath); err != nil {
				testing.ContextLog(ctx, "Failed to delete pflash file after error: ", err)
			}
		}
	}()
	defer dst.Close()

	if err := dst.Chmod(fs.ModePerm); err != nil {
		return errors.Wrap(err, "failed to change permissions on pflash file")
	}

	if err := dst.Chown(crosvmUID, crosvmUID); err != nil {
		return errors.Wrap(err, "failed to change owner on pflash file")
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return errors.Wrap(err, "failed to open pflash file")
	}
	defer src.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return errors.Wrap(err, "failed to copy pflash file to destination")
	}

	return nil
}
