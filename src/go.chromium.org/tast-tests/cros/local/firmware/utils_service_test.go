// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"github.com/google/go-cmp/cmp"
	fwpb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"testing"
)

func TestGetAPCandidateURLs(t *testing.T) {
	for index, td := range []struct {
		url, board, model string
		config            *fwpb.FirmwareBuildTargetsResponse
		expected          []imageCandidate
	}{
		{
			"gs://chromeos-image-archive/firmware-ap-postsubmit/R150-16689.0.0-84695-8681180813530564017/tanjiro/firmware_from_source.tar.bz2",
			"tanjiro",
			"sapphire",
			&fwpb.FirmwareBuildTargetsResponse{
				CorebootName:        "sapphire",
				StandaloneEcName:    "sapphire",
				LegacyEcName:        "sapphire",
				FirmwareManifestKey: "sapphire",
			},
			[]imageCandidate{
				{
					GSURL:     "gs://firmware-image-archive/tanjiro/firmware-ap-postsubmit/16689.0.0/sapphire.16689.0.0.tar.bz2",
					Filenames: []string{"image-sapphire.bin", "image.bin"},
				},
				{
					GSURL:     "gs://firmware-image-archive/firmware-ap-postsubmit/16689.0.0/sapphire.16689.0.0.tar.bz2",
					Filenames: []string{"image-sapphire.bin", "image.bin"},
				},
				{
					GSURL:     "gs://firmware-image-archive/firmware-ap-postsubmit/16689.0.0/Sapphire.16689.0.0.tbz2",
					Filenames: []string{"image-sapphire.bin", "image.bin"},
				},
				{
					GSURL: "gs://chromeos-image-archive/firmware-ap-postsubmit/R150-16689.0.0-84695-8681180813530564017/tanjiro/firmware_from_source.tar.bz2",
					Filenames: []string{
						"image-sapphire.bin", "image-sapphire.bin", "image-tanjiro.bin", "image.bin",
						"bios.bin",
					},
				},
			},
		},
		{
			"gs://firmware-image-archive/firmware-skywalker-16378.B/16378.100.0/",
			"skywalker",
			"obiwan",
			&fwpb.FirmwareBuildTargetsResponse{
				CorebootName:        "obiwan",
				StandaloneEcName:    "obiwan",
				LegacyEcName:        "obiwan",
				FirmwareManifestKey: "obiwan",
			},
			[]imageCandidate{
				{
					GSURL:     "gs://firmware-image-archive/firmware-skywalker-16378.B/16378.100.0/obiwan.16378.100.0.tar.bz2",
					Filenames: []string{"image-obiwan.bin", "image.bin"},
				},
				{
					GSURL:     "gs://firmware-image-archive/firmware-skywalker-16378.B/16378.100.0/Obiwan.16378.100.0.tbz2",
					Filenames: []string{"image-obiwan.bin", "image.bin"},
				},
			},
		},
	} {
		candidates, err := getAPCandidateURLs(context.Background(), td.url, td.board, td.model, td.config)
		if err != nil {
			t.Fatal("Unexpected error: ", err)
		}
		if !cmp.Equal(td.expected, candidates) {
			t.Fatalf("[%d]For url %q, got wrong candidates: %s", index, td.url, cmp.Diff(td.expected, candidates))
		}
	}
}

func TestGetECCandidateURLs(t *testing.T) {
	for index, td := range []struct {
		url, board, model string
		config            *fwpb.FirmwareBuildTargetsResponse
		expected          []imageCandidate
	}{
		{
			"gs://chromeos-image-archive/firmware-ap-postsubmit/R150-16689.0.0-84695-8681180813530564017/tanjiro/firmware_from_source.tar.bz2",
			"tanjiro",
			"sapphire",
			&fwpb.FirmwareBuildTargetsResponse{
				CorebootName:        "sapphire",
				StandaloneEcName:    "sapphire",
				LegacyEcName:        "sapphire",
				FirmwareManifestKey: "sapphire",
			},
			[]imageCandidate{
				{
					GSURL:     "gs://firmware-image-archive/tanjiro/firmware-ap-postsubmit/16689.0.0/sapphire.EC.16689.0.0.tar.bz2",
					Filenames: []string{"ec.bin"},
				},
				{
					GSURL:     "gs://firmware-image-archive/firmware-ap-postsubmit/16689.0.0/sapphire.EC.16689.0.0.tar.bz2",
					Filenames: []string{"ec.bin"},
				},
				{
					GSURL:     "gs://firmware-image-archive/firmware-ap-postsubmit/16689.0.0/Sapphire_EC.16689.0.0.tbz2",
					Filenames: []string{"ec.bin"},
				},
				{
					GSURL: "gs://chromeos-image-archive/firmware-ap-postsubmit/R150-16689.0.0-84695-8681180813530564017/tanjiro/firmware_from_source.tar.bz2",
					Filenames: []string{
						"sapphire/ec.bin", "sapphire/ec.bin", "tanjiro/ec.bin", "ec.bin",
					},
				},
			},
		},
		{
			"gs://firmware-image-archive/firmware-skywalker-16378.B/16378.100.0/",
			"skywalker",
			"obiwan",
			&fwpb.FirmwareBuildTargetsResponse{
				CorebootName:        "obiwan",
				StandaloneEcName:    "obiwan",
				LegacyEcName:        "obiwan",
				FirmwareManifestKey: "obiwan",
			},
			[]imageCandidate{
				{
					GSURL:     "gs://firmware-image-archive/firmware-skywalker-16378.B/16378.100.0/obiwan.EC.16378.100.0.tar.bz2",
					Filenames: []string{"ec.bin"},
				},
				{
					GSURL:     "gs://firmware-image-archive/firmware-skywalker-16378.B/16378.100.0/Obiwan_EC.16378.100.0.tbz2",
					Filenames: []string{"ec.bin"},
				},
			},
		},
		{
			"gs://firmware-image-archive/firmware-ec-R145-16552.2.B/16552.2.22/",
			"skywalker",
			"obiwan",
			&fwpb.FirmwareBuildTargetsResponse{
				CorebootName:        "obiwan",
				StandaloneEcName:    "obiwan",
				LegacyEcName:        "obiwan",
				FirmwareManifestKey: "obiwan",
			},
			[]imageCandidate{
				{
					GSURL:     "gs://firmware-image-archive/firmware-ec-R145-16552.2.B/16552.2.22/obiwan.EC.16552.2.22.tar.bz2",
					Filenames: []string{"ec.bin"},
				},
				{
					GSURL:     "gs://firmware-image-archive/firmware-ec-R145-16552.2.B/16552.2.22/Obiwan_EC.16552.2.22.tbz2",
					Filenames: []string{"ec.bin"},
				},
			},
		},
	} {
		candidates, err := getECCandidateURLs(context.Background(), td.url, td.board, td.model, td.config)
		if err != nil {
			t.Fatal("Unexpected error: ", err)
		}
		if !cmp.Equal(td.expected, candidates) {
			t.Fatalf("[%d]For url %q, got wrong candidates: %s", index, td.url, cmp.Diff(td.expected, candidates))
		}
	}
}
