// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pvs

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"time"

	"go.chromium.org/tast/core/testing"
)

const pvsOutputDir = "/home/chronos/user/.pvs"

var pvsResultsDir = path.Join(pvsOutputDir, "results")

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
	})

}

type pvsFixture struct {
	containerID string
}

func (f *pvsFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Run shop unpack
	dut := s.DUT().Conn()
	shopUnpack := `echo test0000 | sudo -Sv && FORCE_DLM_SKU_ID=1111 shop unpack --dut localhost:2223 --milestone 115 --chromeos-version 15460.0.0`
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
	if _, err := removeDirAsRoot(ctx, dut, pvsOutputDir); err != nil {
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
	if _, err := removeDirAsRoot(ctx, dut, pvsResultsDir); err != nil {
		s.Fatal("Error occured when trying to cleanup existing pvs results dir: ", err)
	}
}

func (f *pvsFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *pvsFixture) Reset(ctx context.Context) error {
	return nil
}
