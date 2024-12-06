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

type drawOnCanvasTestParamData struct {
	Name       string
	FilePrefix string
}

func genDrawOnCanvasTestParamData(resolution int, yuvEnc yuvEncoding, yuvRng yuvRange) (drawOnCanvasTestParamData, error) {
	var yuvEncStr string
	switch yuvEnc {
	case bt601:
		yuvEncStr = "bt601"
	case bt709:
		yuvEncStr = "bt709"
	case bt2020:
		yuvEncStr = "bt2020"
	default:
		return drawOnCanvasTestParamData{}, fmt.Errorf("unknown YUV encoding: %v", yuvEnc)
	}

	var yuvRngStr string
	switch yuvRng {
	case limited:
		yuvRngStr = "limited"
	case full:
		yuvRngStr = "full"
	default:
		return drawOnCanvasTestParamData{}, fmt.Errorf("unknown YUV range: %v", yuvRng)
	}

	testName := fmt.Sprintf("h264_%dp_%s_%s_srgb", resolution, yuvEncStr, yuvRngStr)
	filePrefix := fmt.Sprintf("still-colors-%dp-%s-%s-srgb.h264", resolution, yuvEncStr, yuvRngStr)

	return drawOnCanvasTestParamData{
		Name:       testName,
		FilePrefix: filePrefix,
	}, nil
}

func TestDrawOnCanvasConfig(t *testing.T) {
	var params []drawOnCanvasTestParamData

	// Regular variants.
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
				param, err := genDrawOnCanvasTestParamData(resolution, yuvEnc, yuvRng)
				if err != nil {
					t.Fatal(err)
				}
				params = append(params, param)
			}
		}
	}

	// Exotic cropping variant.
	params = append(params, drawOnCanvasTestParamData{
		Name:       fmt.Sprintf("h264_360p_exotic_crop_bt601_limited_srgb"),
		FilePrefix: "still-colors-720x480-cropped-to-640x360-bt601-limited-srgb.h264",
	})

	code := genparams.Template(t, `{{ range . }}{
		Name: "{{ .Name }}",
		Val: drawOnCanvasParams{
			fileName: "{{ .FilePrefix }}.mp4",
			refFileName: "{{ .FilePrefix }}.ref.png",
		},
		ExtraData: []string{
			"{{ .FilePrefix }}.mp4",
			"{{ .FilePrefix }}.ref.png",
		},
  },
  {{ end }}`, params)

	genparams.Ensure(t, "draw_on_canvas.go", code)
}
