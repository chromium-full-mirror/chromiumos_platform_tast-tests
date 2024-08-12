// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package video

import (
	"fmt"
	"testing"

	"go.chromium.org/tast-tests/cros/common/genparams"
)

// Re-generate the test parameters by running the following in a chroot:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/video

type contentsTestParamData struct {
	Name         string
	FilePrefix   string
	HardwareDeps string
	Fixture      string
}

type yuvEncoding int

const (
	bt601 yuvEncoding = iota
	bt709
	bt2020
)

type yuvRange int

const (
	limited yuvRange = iota
	full
)

func genContentsTestParamData(resolution int, yuvEnc yuvEncoding, yuvRng yuvRange, forceCompositing bool) contentsTestParamData {
	nameSuffix := ""
	if forceCompositing {
		nameSuffix = "_composited"
	}

	fixture := "chromeVideo"
	if forceCompositing {
		fixture = "chromeCompositedVideo"
	}

	var yuvEncStr string
	switch yuvEnc {
	case bt601:
		yuvEncStr = "bt601"
	case bt709:
		yuvEncStr = "bt709"
	case bt2020:
		yuvEncStr = "bt2020"
	default:
		panic("Unknown YUV encoding")
	}

	var yuvRngStr string
	switch yuvRng {
	case limited:
		yuvRngStr = "limited"
	case full:
		yuvRngStr = "full"
	default:
		panic("Unknown YUV range")
	}

	testName := fmt.Sprintf("h264_%dp_%s_%s_srgb%s", resolution, yuvEncStr, yuvRngStr, nameSuffix)
	filePrefix := fmt.Sprintf("still-colors-%dp-%s-%s-srgb.h264", resolution, yuvEncStr, yuvRngStr)

	hardwareDeps := ""
	if !forceCompositing {
		hardwareDeps = "hwdep.D(hwdep.SupportsNV12Overlays())"
	}

	return contentsTestParamData{
		Name:         testName,
		FilePrefix:   filePrefix,
		HardwareDeps: hardwareDeps,
		Fixture:      fixture,
	}
}

func TestContentsConfig(t *testing.T) {
	var params []contentsTestParamData

	// Regular and forced-compositing variants.
	for _, resolution := range []int{360, 480, 720} {
		for _, yuvEnc := range []yuvEncoding{bt601, bt709, bt2020} {
			if resolution != 720 && yuvEnc != bt601 {
				// This is simply to limit the number of combinations.
				continue
			}
			for _, yuvRng := range []yuvRange{limited, full} {
				if resolution != 720 && yuvRng != limited {
					// This is simply to limit the number of combinations.
					continue
				}
				for _, forceCompositing := range []bool{false, true} {
					params = append(params, genContentsTestParamData(resolution, yuvEnc, yuvRng, forceCompositing))
				}
			}
		}
	}

	// Exotic cropping variants.
	for _, forceCompositing := range []bool{false, true} {
		nameSuffix := ""
		hwDeps := "hwdep.D(hwdep.SupportsNV12Overlays())"
		fixture := "chromeVideo"
		if forceCompositing {
			nameSuffix = "_composited"
			hwDeps = ""
			fixture = "chromeCompositedVideo"
		}
		params = append(params, contentsTestParamData{
			Name:         fmt.Sprintf("h264_360p_exotic_crop_bt601_limited_srgb%s", nameSuffix),
			FilePrefix:   "still-colors-720x480-cropped-to-640x360-bt601-limited-srgb.h264",
			HardwareDeps: hwDeps,
			Fixture:      fixture,
		})
	}

	code := genparams.Template(t, `{{ range . }}{
		Name: "{{ .Name }}",
		Val: contentsParams{
			fileName: "{{ .FilePrefix }}.mp4",
			refFileName: "{{ .FilePrefix }}.ref.png",
			browserType: browser.TypeAsh,
		},
		ExtraData: []string{
			"{{ .FilePrefix }}.mp4",
			"{{ .FilePrefix }}.ref.png",
		},
		{{ if .HardwareDeps }}
		ExtraHardwareDeps: {{ .HardwareDeps }},
		{{ end }}
		Fixture: "{{ .Fixture }}",
  },
  {{ end }}`, params)

	genparams.Ensure(t, "contents.go", code)
}
