// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pvsutils

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"time"

	"go.chromium.org/tast/core/testing"
)

const chronosHome = "/home/chronos/user"
const uploadConfigJSON = `{"bucket":"chromeos-moblab-pvs-dev","service_account":"/home/chronos/user/.pvs/upload_config/.service_account.json","boto_key":""}`

var pvsOutputDir = path.Join(chronosHome, ".pvs")
var pvsResultsDir = path.Join(pvsOutputDir, "results")
var gitCookiesPath = path.Join(chronosHome, ".gitcookies")
var uploadConfigDir = path.Join(pvsOutputDir, "upload_config")
var serviceAccountPath = path.Join(uploadConfigDir, ".service_account.json")
var uploadConfigJSONPath = path.Join(uploadConfigDir, "upload_config.json")

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
		Vars:            []string{"pvs.git_cookies", "pvs.service_account"},
	})

}

type pvsFixture struct {
	containerID string
}

func (f *pvsFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Populate git cookies
	dut := s.DUT().Conn()
	gitCookies := s.RequiredVar("pvs.git_cookies")
	if _, err := writeToFileAsChronos(ctx, dut, gitCookies, gitCookiesPath); err != nil {
		s.Fatal("Error occured when populating git cookies: ", err)
	}

	// Populate service account and upload config
	if _, err := removeAsRoot(ctx, dut, pvsOutputDir); err != nil {
		s.Fatal("Error occured when trying to cleanup pvs output dir: ", err)
	}
	serviceAccount := s.RequiredVar("pvs.service_account")
	createUploadConfig := fmt.Sprintf(`mkdir -p %v`, uploadConfigDir)
	if _, err := RunAsChronos(ctx, dut, createUploadConfig); err != nil {
		s.Fatal("Error occured when creating upload config dir: ", err)
	}
	if _, err := writeToFileAsChronos(ctx, dut, serviceAccount, serviceAccountPath); err != nil {
		s.Fatal("Error occured when populating service account: ", err)
	}
	if _, err := writeToFileAsChronos(ctx, dut, uploadConfigJSON, uploadConfigJSONPath); err != nil {
		s.Fatal("Error occured when populating upload config : ", err)
	}

	// Run shop unpack
	shopUnpack := `FORCE_DLM_SKU_ID=1111 shop unpack --dut localhost:2223 --milestone 115 --chromeos-version 15393.59.0`
	shopOutput, err := RunAsChronos(ctx, dut, shopUnpack)
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
