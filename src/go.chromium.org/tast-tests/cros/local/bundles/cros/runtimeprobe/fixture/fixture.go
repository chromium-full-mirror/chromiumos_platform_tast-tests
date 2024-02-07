// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture contains the fixtures used in the runtimeprobe tests.
package fixture

import (
	"context"
	"io/fs"
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
	DecryptProbeConfig     = "decryptProbeConfig"
	probeConfigStatefulDir = "/usr/local/etc/runtime_probe/"
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

func decryptProbeConfig(ctx context.Context, s *testing.FixtState, model string) (string, error) {
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
		return "", errors.New("cannot find encrypted probe configs")
	}
	s.Log("Found encrypted probe config: ", probeConfigEncryptedPath)

	keys, ok := s.Var("runtimeprobe.ProbeFunction.keys")
	if !ok {
		return "", errors.New("failed to read variable: runtimeprobe.ProbeFunction.keys")
	}

	tempProbeConfig, err := os.CreateTemp("", "probe_config-*.json")
	if err != nil {
		return "", errors.Wrap(err, "failed to create temp probe config")
	}
	tempProbeConfig.Close()
	tempProbeConfigPath := tempProbeConfig.Name()
	decrypted := false
	for keyIndex, key := range strings.Split(keys, "\n") {
		cmd := testexec.CommandContext(ctx, "openssl", "aes-256-cbc", "-d", "-pbkdf2", "-base64", "-in", probeConfigEncryptedPath, "-out", tempProbeConfigPath, "-pass", "env:RUNTIME_PROBE_CONFIG_KEY")
		cmd.Env = append(os.Environ(), "RUNTIME_PROBE_CONFIG_KEY="+key)
		if _, err := cmd.Output(); err == nil {
			decrypted = true
			s.Logf("Decrypt success with runtimeprobe.ProbeFunction.keys[%d]", keyIndex)
			break
		}
	}
	if !decrypted {
		os.Remove(tempProbeConfigPath)
		return "", errors.New("failed to decrypt with openssl. Incorrect key?")
	}
	if err := os.Chmod(tempProbeConfigPath, 0644); err != nil {
		os.Remove(tempProbeConfigPath)
		return "", errors.Wrap(err, "failed to chmod for probe config")
	}
	s.Log("Decrypt probe config to ", tempProbeConfigPath)
	return tempProbeConfigPath, nil
}

type decryptProbeConfigFixture struct {
	probeConfigBackupDir string
}

func (f *decryptProbeConfigFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	model, err := modelName(ctx)
	if err != nil {
		s.Fatal("Failed to get model name: ", err)
	}

	tempDecryptProbeConfig, err := decryptProbeConfig(ctx, s, model)
	if err != nil {
		s.Fatal("Failed to decrypt probe config: ", err)
	}
	defer os.Remove(tempDecryptProbeConfig)

	tempDir, err := os.MkdirTemp("", "runtimeprobe-")
	if err != nil {
		s.Fatal("Failed to create temp directory: ", err)
	}
	f.probeConfigBackupDir = filepath.Join(tempDir, "bak/")
	if err := os.Rename(probeConfigStatefulDir, f.probeConfigBackupDir); err == nil {
		s.Log("Backup probe config to ", f.probeConfigBackupDir)
	} else if !errors.Is(err, fs.ErrNotExist) {
		s.Fatal("Failed to backup probe config directory: ", err)
	}
	probeConfigDir := filepath.Join(probeConfigStatefulDir, model)
	if err := os.MkdirAll(probeConfigDir, 0755); err != nil {
		s.Fatal("Failed to create probe config directory: ", err)
	}
	os.Rename(tempDecryptProbeConfig, filepath.Join(probeConfigDir, "probe_config.json"))
	return nil
}

func (f *decryptProbeConfigFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	os.RemoveAll(probeConfigStatefulDir)
	if f.probeConfigBackupDir != "" {
		if err := os.Rename(f.probeConfigBackupDir, probeConfigStatefulDir); err == nil {
			s.Log("Restore probe config from ", f.probeConfigBackupDir)
		}
		os.RemoveAll(filepath.Join(f.probeConfigBackupDir, ".."))
	}
}

func (f *decryptProbeConfigFixture) Reset(ctx context.Context) error                        { return nil }
func (f *decryptProbeConfigFixture) PreTest(ctx context.Context, s *testing.FixtTestState)  {}
func (f *decryptProbeConfigFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}
