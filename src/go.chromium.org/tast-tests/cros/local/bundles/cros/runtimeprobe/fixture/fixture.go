// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture contains the fixtures used in the runtimeprobe tests.
package fixture

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Fixture names.
const (
	DecryptProbeConfig = "decryptProbeConfig"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: DecryptProbeConfig,
		Desc: "Decrypt private probe configs",
		Contacts: []string{
			"chromeos-runtime-probe@google.com",
			"clarkchung@google.com",
		},
		Impl:            &decryptProbeConfigFixture{},
		Vars:            []string{"runtimeprobe.ProbeFunction.keys"},
		SetUpTimeout:    10 * time.Second,
		TearDownTimeout: 10 * time.Second,
	})
}

func modelName(ctx context.Context) (string, error) {
	out, err := testexec.CommandContext(ctx, "cros_config", "/", "name").Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to run \"cros_config / name\"")
	}
	return string(out), nil
}

func backupProbeConfig(s *testing.FixtState, model string) error {
	probeConfigDir := filepath.Join("/usr/local/etc/runtime_probe/", model)
	probeConfigPath := filepath.Join(probeConfigDir, "probe_config.json")
	probeConfigBackupPath := filepath.Join(probeConfigDir, "probe_config.json.bak")
	if _, err := os.Stat(probeConfigPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.Rename(probeConfigPath, probeConfigBackupPath); err != nil {
		return err
	}
	s.Log("Backup probe config to ", probeConfigBackupPath)
	return nil
}

func decryptProbeConfig(ctx context.Context, s *testing.FixtState, model string) error {
	probeConfigDir := filepath.Join("/usr/local/etc/runtime_probe/", model)
	probeConfigPath := filepath.Join(probeConfigDir, "probe_config.json")
	probeConfigEncryptedPath := ""
	configRoots := []string{
		"/usr/local/",
		"/",
	}
	for _, configRoot := range configRoots {
		probeConfigEncryptedPath = filepath.Join(configRoot, "/etc/runtime_probe/", model, "probe_config.json.enc")
		if _, err := os.Stat(probeConfigEncryptedPath); err == nil {
			break
		}
		probeConfigEncryptedPath = ""
	}
	if probeConfigEncryptedPath == "" {
		return errors.New("cannot find encrypted probe configs")
	}
	s.Log("Found encrypted probe config: ", probeConfigEncryptedPath)

	if err := os.MkdirAll(probeConfigDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create probe config directory: %s", probeConfigDir)
	}

	keys, ok := s.Var("runtimeprobe.ProbeFunction.keys")
	if !ok {
		return errors.New("failed to read variable: runtimeprobe.ProbeFunction.keys")
	}

	decrypted := false
	for keyIndex, key := range strings.Split(keys, "\n") {
		cmd := testexec.CommandContext(ctx, "openssl", "aes-256-cbc", "-d", "-pbkdf2", "-base64", "-in", probeConfigEncryptedPath, "-out", probeConfigPath, "-pass", "env:RUNTIME_PROBE_CONFIG_KEY")
		cmd.Env = append(os.Environ(), "RUNTIME_PROBE_CONFIG_KEY="+key)
		if _, err := cmd.Output(); err == nil {
			decrypted = true
			s.Logf("decrypt success with runtimeprobe.ProbeFunction.keys[%d]", keyIndex)
			break
		}
	}
	if !decrypted {
		os.Remove(probeConfigPath)
		return errors.New("failed to decrypt with openssl. Incorrect key?")
	}
	if err := os.Chmod(probeConfigPath, 0644); err != nil {
		os.Remove(probeConfigPath)
		return errors.Wrap(err, "failed to chmod for probe config")
	}
	s.Log("Decrypt probe config to ", probeConfigPath)
	return nil
}

func restoreProbeConfig(s *testing.FixtState, model string) {
	probeConfigDir := filepath.Join("/usr/local/etc/runtime_probe/", model)
	probeConfigPath := filepath.Join(probeConfigDir, "probe_config.json")
	os.Remove(probeConfigPath)
	if err := os.Rename(filepath.Join(probeConfigDir, "probe_config.json.bak"), probeConfigPath); err == nil {
		s.Log("Restore probe config to ", probeConfigPath)
	}
}

type decryptProbeConfigFixture struct{}

func (f *decryptProbeConfigFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	model, err := modelName(ctx)
	if err != nil {
		s.Fatal("Failed to get model name: ", err)
	}
	if err := backupProbeConfig(s, model); err != nil {
		s.Fatal("Failed to backup probe config: ", err)
	}
	if err := decryptProbeConfig(ctx, s, model); err != nil {
		s.Fatal("Failed to decrypt probe config: ", err)
	}
	return nil
}

func (f *decryptProbeConfigFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	model, err := modelName(ctx)
	if err != nil {
		s.Fatal("Failed to get model name: ", err)
	}
	restoreProbeConfig(s, model)
}

func (f *decryptProbeConfigFixture) Reset(ctx context.Context) error                        { return nil }
func (f *decryptProbeConfigFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (f *decryptProbeConfigFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
