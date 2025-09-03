// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package runtimeprobe

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	rppb "go.chromium.org/chromiumos/system_api/runtime_probe_proto"
	"golang.org/x/crypto/ssh"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/runtimeprobe/utils"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

var (
	// allowedUnidentifiedComponentsFields is a map from model name to a set of
	// component fields that are allowed to have unidentified components (i.e. `?`)
	// as their value.
	allowedUnidentifiedComponentsFields = map[string]map[string]struct{}{
		"ciri": {"touchpad": {}},
	}
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RuntimeHWIDVerify,
		Desc: "Verify the Runtime HWID file content",
		Contacts: []string{
			"chromeos-runtime-probe@google.com",
			"allenshihmc@google.com",
		},
		BugComponent: "b:606088",
		SoftwareDeps: []string{"reboot", "racc"},
		HardwareDeps: hwdep.D(hwdep.RuntimeProbeConfig(), hwdep.RuntimeProbeConfigPrivate(false)),
		Fixture:      fixture.CleanupRuntimeHWID,
		Attr:         []string{"group:racc", "racc_config_installed"},
	})
}

func RuntimeHWIDVerify(ctx context.Context, s *testing.State) {
	const (
		runtimeHWIDFileDir  = "/var/cache/hardware_verifier"
		runtimeHWIDFileName = "runtime_hwid"
	)

	d := s.DUT()

	cmd := []string{
		"sudo", "-u", "hardware_verifier", "hardware_verifier", "--runtime_hwid_refresh_policy=force_generate", "--verbosity=1",
	}
	if _, err := d.Conn().CommandContext(ctx, cmd[0], cmd[1:]...).Output(); err != nil {
		exitError, isExitError := err.(*ssh.ExitError)
		// For unqualified hardware components, hardware_verifier would exit
		// with status 1 and it is expected.
		if !isExitError || exitError.ExitStatus() != 1 {
			s.Fatal("Failed to invoke hardware_verifier: ", err)
		}
	}

	runtimeHWIDFilePath := filepath.Join(runtimeHWIDFileDir, runtimeHWIDFileName)
	outPath := filepath.Join(s.OutDir(), runtimeHWIDFileName)
	if err := linuxssh.GetFile(ctx, d.Conn(), runtimeHWIDFilePath, outPath, linuxssh.DereferenceSymlinks); err != nil {
		s.Fatal("Failed to get the Runtime HWID file from DUT: ", err)
	}

	bytes, err := os.ReadFile(outPath)
	if err != nil {
		s.Fatalf("Failed to read the Runtime HWID file at %q: %v", outPath, err)
	}
	fileContent := string(bytes)

	if err := verifyRuntimeHWIDFileContent(ctx, d, fileContent); err != nil {
		s.Fatal("The verification of Runtime HWID file content failed: ", err)
	}
}

// verifyRuntimeHWIDFileContent verifies that the content of the Runtime HWID file meets the following criteria:
// 1. It contains two lines: the Runtime HWID and its checksum.
// 2. The Runtime HWID is in the format "<MODEL_RLZ> ... R:<RUNTIME_HWID_COMPONENTS>".
// 3. The Runtime HWID components only contain specific characters.
// 4. The Runtime HWID components do not contain unidentified components.
// 5. The checksum is the SHA-1 hash of the Runtime HWID.
func verifyRuntimeHWIDFileContent(ctx context.Context, d *dut.DUT, fileContent string) error {
	const (
		runtimeHWIDComponentRegex = `^[0-9,#X?\-]*$`
		runtimeHWIDMagicString    = "R:"
	)

	lines := strings.Split(strings.TrimSpace(fileContent), "\n")
	if len(lines) != 2 {
		return errors.Errorf("expected 2 lines in Runtime HWID file, but got %d lines", len(lines))
	}

	runtimeHwid := lines[0]
	checksum := lines[1]
	testing.ContextLogf(ctx, "Runtime HWID: %q", runtimeHwid)

	modelRLZ, err := getModelRLZ(ctx, d)
	if err != nil {
		return errors.Wrap(err, "failed to get the model and RLZ code")
	}

	parts := strings.Split(runtimeHwid, " ")
	if len(parts) < 3 {
		return errors.Errorf("invalid Runtime HWID format: %q", runtimeHwid)
	}
	if parts[0] != modelRLZ {
		return errors.Errorf("model and RLZ code mismatch: got %q, want %q", parts[0], modelRLZ)
	}

	lastPart := parts[len(parts)-1]
	if !strings.HasPrefix(lastPart, runtimeHWIDMagicString) {
		return errors.Errorf("the last part of Runtime HWID should start with 'R:', but got %q", lastPart)
	}

	runtimeHWIDComponents := strings.TrimPrefix(lastPart, runtimeHWIDMagicString)
	if matched, err := regexp.MatchString(runtimeHWIDComponentRegex, runtimeHWIDComponents); err != nil {
		return errors.Wrap(err, "failed to match Runtime HWID components")
	} else if !matched {
		return errors.Errorf("Runtime HWID components %q contains invalid characters", runtimeHWIDComponents)
	}

	if err := verifyRuntimeHWIDComponents(ctx, d, runtimeHWIDComponents); err != nil {
		return err
	}
	h := sha1.New()
	h.Write([]byte(runtimeHwid))
	expectedChecksum := strings.ToUpper(hex.EncodeToString(h.Sum(nil)))

	if checksum != expectedChecksum {
		return errors.Errorf("checksum mismatch: got %q, want %q", checksum, expectedChecksum)
	}
	return nil
}

// verifyRuntimeHWIDComponents verifies that the Runtime HWID components do not
// contain unidentified components, except for allowed ones.
func verifyRuntimeHWIDComponents(ctx context.Context, d *dut.DUT, runtimeHWIDComponents string) error {
	const (
		runtimeHWIDCompSeparator    = "-"
		runtimeHWIDUnidentifiedComp = "?"
	)

	// Get Runtime HWID component names from the protobuf definition.
	var allFields []string
	msg := &rppb.RuntimeHwidComponent{}
	fds := msg.ProtoReflect().Descriptor().Fields()
	for i := 0; i < fds.Len(); i++ {
		allFields = append(allFields, string(fds.Get(i).Name()))
	}

	fieldComps := strings.Split(runtimeHWIDComponents, runtimeHWIDCompSeparator)
	if len(fieldComps) > len(allFields) {
		return errors.Errorf("too many fields in Runtime HWID components: got %d, want at most %d", len(fieldComps), len(allFields))
	}

	modelName, err := getModelName(ctx, d)
	if err != nil {
		return errors.Wrap(err, "failed to get model name")
	}

	allowedFields := allowedUnidentifiedComponentsFields[modelName]
	probeFunctionWaivedFields := utils.ProbeFunctionWaivedFields[modelName]
	for i, comps := range fieldComps {
		if strings.Contains(comps, runtimeHWIDUnidentifiedComp) {
			fieldName := allFields[i]
			_, allowed := allowedFields[fieldName]
			_, waived := probeFunctionWaivedFields[fieldName]
			if !allowed && !waived {
				return errors.Errorf("the %q components are %q, which contain unidentified components", fieldName, comps)
			}
		}
	}
	return nil
}

func getModelRLZ(ctx context.Context, d *dut.DUT) (string, error) {
	out, err := d.Conn().CommandContext(ctx, "crossystem", "hwid").Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to run \"crossystem hwid\"")
	}
	lines := strings.Split(strings.TrimSpace(string(out)), " ")
	return lines[0], nil
}

func getModelName(ctx context.Context, d *dut.DUT) (string, error) {
	out, err := d.Conn().CommandContext(ctx, "cros_config", "/", "name").Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to run \"cros_config / name\"")
	}
	return strings.TrimSpace(string(out)), nil
}
