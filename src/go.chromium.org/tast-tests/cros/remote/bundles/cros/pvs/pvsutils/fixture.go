// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pvsutils

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast/core/testing"
)

const chronosHome = "/home/chronos/user"
const installDir = "/usr/local/share/shop_install"
const gcloudSymlinkPath = "/usr/local/bin/gcloud"
const uploadConfigJSON = `{"bucket":"chromeos-moblab-pvs-dev","service_account":"/home/chronos/user/.pvs/upload_config/.service_account.json","boto_key":""}`
const defaultTagAndRef = "prod"
const containerPVSOutputDir = "/home/pvs/.pvs"

var pvsOutputDir = path.Join(chronosHome, ".pvs")
var pvsResultsDir = path.Join(pvsOutputDir, "results")
var gitCookiesPath = path.Join(chronosHome, ".gitcookies")
var pvsConfigDir = path.Join(pvsOutputDir, "config")
var featureFlagsPath = path.Join(pvsConfigDir, "featureflags.textproto")
var uploadConfigDir = path.Join(pvsOutputDir, "upload_config")
var serviceAccountPath = path.Join(uploadConfigDir, ".service_account.json")
var uploadConfigJSONPath = path.Join(uploadConfigDir, "upload_config.json")
var gcloudPath = path.Join(installDir, "cipd", "gcloud")

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: "pvsShopUnpack",
		Desc: "Set up pvs container using `shop unpack`",
		Contacts: []string{
			"chromeos-pvs-eng@google.com",
			"jackgelinas@google.com",
		},
		Impl:            &pvsFixture{},
		SetUpTimeout:    10 * time.Minute,
		ResetTimeout:    1 * time.Minute,
		TearDownTimeout: 2 * time.Minute,
		PreTestTimeout:  2 * time.Minute,
		PostTestTimeout: 1 * time.Minute,
		Vars: []string{
			"pvs.git_cookies",
			"pvs.service_account",
			"pvs.shop_ref",
			"pvs.image_tag",
			"pvs.chromeos_version",
			"pvs.skip_teardown",
			"pvs.simulated_mode",
			"pvs.feature_flags",
			"pvs.force_dlm_sku_id",
		},
	})
}

type pvsFixture struct {
	containerID string
}

func (f *pvsFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	pvsHost := s.DUT().Conn()

	// Change owner of shop install dir to chronos
	// TODO(b/276726105): remove chronos permissions issue is resolved
	chownInstallDir := pvsHost.CommandContext(ctx, "chown", "chronos", installDir)
	if _, err := runAsRoot(ctx, chownInstallDir); err != nil {
		s.Fatal("Error occured when chowning the install directory: ", err)
	}

	// Setup gcloud symlink
	// TODO(b/276776309): remove once gcloud is no longer a dependency
	symlinkGcloud := pvsHost.CommandContext(ctx, "ln", "-sf", gcloudPath, gcloudSymlinkPath)
	if _, err := runAsRoot(ctx, symlinkGcloud); err != nil {
		s.Fatal("Error occured when setting up gcloud symlink: ", err)
	}

	// Populate git cookies
	gitCookies := s.RequiredVar("pvs.git_cookies")
	if _, err := writeToFileAsChronos(ctx, pvsHost, gitCookies, gitCookiesPath); err != nil {
		s.Fatal("Error occured when populating git cookies: ", err)
	}

	// Populate service account and upload config
	if _, err := removeAsRoot(ctx, pvsHost, pvsOutputDir); err != nil {
		s.Fatal("Error occured when trying to cleanup pvs output dir: ", err)
	}
	serviceAccount := s.RequiredVar("pvs.service_account")
	createUploadConfig := fmt.Sprintf(`mkdir -p %v`, uploadConfigDir)
	if _, err := RunAsChronos(ctx, pvsHost, createUploadConfig); err != nil {
		s.Fatal("Error occured when creating upload config dir: ", err)
	}
	if _, err := writeToFileAsChronos(ctx, pvsHost, serviceAccount, serviceAccountPath); err != nil {
		s.Fatal("Error occured when populating service account: ", err)
	}
	if _, err := writeToFileAsChronos(ctx, pvsHost, uploadConfigJSON, uploadConfigJSONPath); err != nil {
		s.Fatal("Error occured when populating upload config : ", err)
	}

	// set feature flags file
	flagsString, ok := s.Var("pvs.feature_flags")
	if ok {
		featureFlagFileText := ""
		for _, flagKeyValue := range strings.Split(flagsString, ",") {
			parsed := strings.Split(flagKeyValue, "=")
			if len(parsed) != 2 {
				s.Fatalf("Error parsing: %q feature flag key value, correct format is 'flag1=value1,flag2=value2,...'", flagKeyValue)
			}
			featureFlagFileText += fmt.Sprintf("flags { key: %q value: %q }\n", parsed[0], parsed[1])
		}
		// write feature flags to config file
		createConfigDir := fmt.Sprintf(`mkdir -p %v`, pvsConfigDir)
		if _, err := RunAsChronos(ctx, pvsHost, createConfigDir); err != nil {
			s.Fatal("Error occured when creating config dir: ", err)
		}
		if _, err := writeToFileAsChronos(ctx, pvsHost, featureFlagFileText, featureFlagsPath); err != nil {
			s.Fatal("Error occured when populating git cookies: ", err)
		}
	}

	// Run shop unpack
	shopUnpack := shopUnpackCmd(s)
	shopOutput, err := RunAsChronos(ctx, pvsHost, shopUnpack)
	if err != nil {
		s.Fatal("Error occured when running shop unpack: ", err)
	}

	// Parse container id from shop output
	containerIDRegex := regexp.MustCompile(`"docker attach (.*)"`)
	regexpMatch := containerIDRegex.FindStringSubmatch(shopOutput)
	if len(regexpMatch) != 2 {
		s.Fatal("container id not found in shop command output: ", shopOutput)
	}
	f.containerID = regexpMatch[1]
	s.Log("Started pvs container with id: ", f.containerID)
	return f.containerID
}

func (f *pvsFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	// skip teardown if specified so logs can be inspected
	if _, ok := s.Var("pvs.skip_teardown"); ok {
		return
	}

	// Remove pvs output dir created during tests
	dut := s.DUT().Conn()
	if _, err := removeAsRoot(ctx, dut, pvsOutputDir); err != nil {
		s.Fatal("Error occured when trying to cleanup pvs output dir: ", err)
	}

	// Stop running pvs container
	stopContainer := fmt.Sprintf(`docker stop %v`, f.containerID)
	_, err := RunAsChronos(ctx, dut, stopContainer)
	if err != nil {
		s.Fatal("Error occured when stopping container: ", err)
	}
}

func (f *pvsFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// Remove any pre-existing results dir so incremental run tests can work correctly
	dut := s.DUT().Conn()
	if _, err := removeAsRoot(ctx, dut, pvsResultsDir); err != nil {
		s.Fatal("Error occured when trying to cleanup existing pvs results dir: ", err)
	}
}

func (f *pvsFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *pvsFixture) Reset(ctx context.Context) error {
	return nil
}

func shopUnpackCmd(s *testing.FixtState) string {
	env := shopEnv(s).generateEnvMap()
	var envArgs []string
	for key, val := range env {
		envArgs = append(envArgs, fmt.Sprintf("%v=%v", key, val))
	}
	args := shopArgs(s)
	cmd := fmt.Sprintf(
		"%v shop unpack %v",
		strings.Join(envArgs, " "),
		strings.Join(args, " "),
	)
	return cmd
}

func shopEnv(s *testing.FixtState) RuntimeEnv {
	env := RuntimeEnv{}
	if shopRef, ok := s.Var("pvs.shop_ref"); ok {
		env.ShopRef = shopRef
	} else {
		env.ShopRef = defaultTagAndRef
	}
	if pvsImageTag, ok := s.Var("pvs.image_tag"); ok {
		env.PvsImageTag = pvsImageTag
	}
	if forceDlmSkuID, ok := s.Var("pvs.force_dlm_sku_id"); ok {
		env.ForceDlmSkuID = forceDlmSkuID
	}
	if _, ok := s.Var("pvs.simulated_mode"); ok {
		env.SimulatedDut = true
		env.SimulatedTestRunner = true
	}
	return env
}

func shopArgs(s *testing.FixtState) []string {
	args := make([]string, 0)
	if _, ok := s.Var("pvs.simulated_mode"); !ok {
		dutHostname := s.CompanionDUT("dut").HostName()
		args = append(args, "--dut", dutHostname)
	}
	if chromeosVersion, ok := s.Var("pvs.chromeos_version"); ok {
		args = append(args, "--chromeos-version", chromeosVersion)
	}
	return args
}
