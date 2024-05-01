// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fixture implements fixture that generates test images for fingerprint
// tests.
package fixture

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	fp "go.chromium.org/tast-tests/cros/common/fingerprint"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint"
	"go.chromium.org/tast-tests/cros/remote/firmware/fingerprint/rpcdut"
	"go.chromium.org/tast/core/testing"
)

const (
	// Futility is the futility executable name.
	Futility = "futility"
	// BloonchipperDevKey is the path to the dev key.
	BloonchipperDevKey = "fingerprint_dev_keys/bloonchipper/dev_key.pem"
	// DartmonkeyDevKey is the path to the dev key.
	DartmonkeyDevKey = "fingerprint_dev_keys/dartmonkey/dev_key.pem"
	// NamiFPDevKey is the path to the dev key.
	NamiFPDevKey = "fingerprint_dev_keys/nami_fp/dev_key.pem"
	// NocturneFPDevKey is the path to the dev key.
	NocturneFPDevKey = "fingerprint_dev_keys/nocturne_fp/dev_key.pem"
	// HelipilotFPDevKey is the path to the dev key
	HelipilotFPDevKey = "fingerprint_dev_keys/helipilot/dev_key.pem"
)

var devKeyMap = map[fp.BoardName]string{
	fp.BoardNameBloonchipper: BloonchipperDevKey,
	fp.BoardNameDartmonkey:   DartmonkeyDevKey,
	fp.BoardNameNami:         NamiFPDevKey,
	fp.BoardNameNocturne:     NocturneFPDevKey,
	fp.BoardNameHelipilot:    HelipilotFPDevKey,
}

// FingerprintImages is the name of this fixture.
const FingerprintImages = "fingerprintTestImages"

// ImagesTestData structure contains all information related to generated test
// images to be used by tests.
type ImagesTestData struct {
	TestImages fingerprint.TestImages
}

type fingerprintImagesFixture struct {
	testData     ImagesTestData
	imagesFolder string
}

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: FingerprintImages,
		Desc: "Generate test images of FPMCU firmware",
		Contacts: []string{
			"chromeos-fingerprint@google.com",
			"patrykd@google.com",
		},
		BugComponent:    "b:782045", // ChromeOS > Platform > baseOS > Fingerprint
		Impl:            &fingerprintImagesFixture{},
		ServiceDeps:     []string{dutfs.ServiceName},
		SetUpTimeout:    10 * time.Second,
		TearDownTimeout: 10 * time.Second,
		Data: []string{
			BloonchipperDevKey,
			DartmonkeyDevKey,
			NamiFPDevKey,
			NocturneFPDevKey,
			HelipilotFPDevKey,
		},
	})
}

func devKeyForFPBoard(fpBoard fp.BoardName) string {
	return devKeyMap[fpBoard]
}

func (f *fingerprintImagesFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	dut, err := rpcdut.NewRPCDUT(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect RPCDUT: ", err)
	}
	defer dut.Close(ctx)

	fpBoard, err := fingerprint.Board(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get fingerprint board: ", err)
	}

	buildFwFile, err := fingerprint.FirmwarePath(ctx, dut, fpBoard)
	if err != nil {
		s.Fatal("Failed to get build firmware file path: ", err)
	}

	f.imagesFolder = filepath.Join("/", "mnt", "stateful_partition", fmt.Sprintf("fpimages_%d", time.Now().Unix()))
	dutfsService := dutfs.NewClient(dut.RPC().Conn)

	if err := dutfsService.MkDir(ctx, f.imagesFolder, 0755); err != nil {
		s.Fatal("Failed to create remote working directory: ", err)
	}
	testing.ContextLog(ctx, "Created non-temporary fptast directory")

	// Generate test images to flash to RW.
	f.testData.TestImages, err = fingerprint.GenerateTestFirmwareImages(ctx, dut, s.DataPath(devKeyForFPBoard(fpBoard)), fpBoard, buildFwFile, f.imagesFolder)
	if err != nil {
		if err := dutfsService.RemoveAll(ctx, f.imagesFolder); err != nil {
			testing.ContextLog(ctx, "Failed to delete dir: ", err)
		}
		s.Fatal("Failed to generate test images: ", err)
	}

	return &f.testData
}

func (f *fingerprintImagesFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	dut, err := rpcdut.NewRPCDUT(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect RPCDUT: ", err)
	}
	defer dut.Close(ctx)

	testing.ContextLog(ctx, "Delete fptast directory and contained files from DUT")
	if err := dutfs.NewClient(dut.RPC().Conn).RemoveAll(ctx, f.imagesFolder); err != nil {
		s.Fatal("Failed to delete dir: ", f.imagesFolder, err)
	}
}

func (f *fingerprintImagesFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *fingerprintImagesFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {}

func (f *fingerprintImagesFixture) Reset(ctx context.Context) error { return nil }
