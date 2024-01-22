// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/crosconfig"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// If you need to add an entry:
// 1. Lease a model from crosfleet and run `cros-camera-tool modules list` to get module_id.
// 2. Add an entry in models.json.
// 3. Add a new configuration file as data/kernel_smoke_mipi/<model>-<module>.yaml.
//    If there was no module_id given in Step 1, name it <model>.yaml.

const (
	modelConfigFile = "kernel_smoke_mipi/models.json"
)

func configFiles() []string {
	return []string{
		modelConfigFile,
		"kernel_smoke_mipi/atlas.yaml",
		"kernel_smoke_mipi/aviko-KC4eb5.yaml",
		"kernel_smoke_mipi/brya.yaml",
		"kernel_smoke_mipi/bugzzy-KCfa36.yaml",
		"kernel_smoke_mipi/craask-TC0002.yaml",
		"kernel_smoke_mipi/craaskbowl-TC0003.yaml",
		"kernel_smoke_mipi/drawcia-LV0001.yaml",
		"kernel_smoke_mipi/kano-CH0001.yaml",
		"kernel_smoke_mipi/kano-CH3c6d.yaml",
		"kernel_smoke_mipi/karis-CH3c6d.yaml",
		"kernel_smoke_mipi/maglia-TC0003.yaml",
		"kernel_smoke_mipi/magolor-TC0003.yaml",
		"kernel_smoke_mipi/nautilus.yaml",
		"kernel_smoke_mipi/nautiluslte.yaml",
		"kernel_smoke_mipi/nocturne.yaml",
		"kernel_smoke_mipi/redrix-PLcf06.yaml",
		"kernel_smoke_mipi/rex-KC6977.yaml",
		"kernel_smoke_mipi/screebo-ST0d2c.yaml",
		"kernel_smoke_mipi/skolas-KC4eb5.yaml",
		"kernel_smoke_mipi/soraka.yaml",
		"kernel_smoke_mipi/storo360-KC0001.yaml",
		"kernel_smoke_mipi/vell-TCf939.yaml",
		"kernel_smoke_mipi/voema-CH0001.yaml",
		"kernel_smoke_mipi/xivu360-KC0001.yaml",
		"kernel_smoke_mipi/yavilly-LV0001.yaml",
		"kernel_smoke_mipi/yavilly-TC0001.yaml",
	}
}

// TODO(b/317158480): Skip on those models until we can add configuration file for them
var unsupportedModel = []string{
	"krane", "kakadu", "kodama", "katsu", // kukui
	"coachz", "homestar", "mrbland", "wormdingler", "quackingstick", // strongbad
	"rex4es", "screebo4es", // rex es
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         KernelSmokeMIPI,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Smoke test for MIPI Camera Kernel Drivers",
		Contacts:     []string{"chromeos-camera-eng@google.com", "hidenorik@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:mainline", "informational", "group:camera-stability", "group:camera-kernelnext", "group:camera_dependent"},
		SoftwareDeps: []string{caps.BuiltinMIPICamera},
		HardwareDeps: hwdep.D(hwdep.CameraEnumerated(), hwdep.SkipOnModel(unsupportedModel...)),
		Data:         configFiles(),
	})
}

func configureMedia(ctx context.Context, bus, yaml string) error {
	cmd := testexec.CommandContext(ctx, "mctk", "--load-by-businfo", bus, "--reset-links", "--merge-yaml", yaml)
	return cmd.Run(testexec.DumpLogOnError)
}

func captureFrames(ctx context.Context, bus, entity string) error {
	cmd := testexec.CommandContext(ctx, "media-ctl", "-d", bus, "-e", entity)
	out, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return err
	}
	outDevice := strings.TrimRight(string(out[:]), "\n")

	cmd = testexec.CommandContext(ctx, "yavta", "--capture=10", outDevice)
	return cmd.Run(testexec.DumpLogOnError)
}

type testInfo struct {
	BusInfo   string `json:"bus_info"`
	OutEntity string `json:"out_entity"`
}

func KernelSmokeMIPI(ctx context.Context, s *testing.State) {
	model, err := crosconfig.Get(ctx, "/", "name")
	if err != nil {
		s.Fatal("Failed to get model name: ", err)
	}
	mipiCams, err := testutil.MIPICamerasFromCrOSCameraTool(ctx)
	if err != nil {
		s.Fatal("Failed to get MIPI camera info: ", err)
	}

	bytes, err := os.ReadFile(s.DataPath(modelConfigFile))
	if err != nil {
		s.Fatal("Failed to read model config file: ", err)
	}
	var modelMap map[string]testInfo
	if err := json.Unmarshal(bytes, &modelMap); err != nil {
		s.Fatal("Failed to parse model config: ", err)
	}

	suffixes := make([]string, 0, len(mipiCams))
	for _, cam := range mipiCams {
		module := cam["module_id"]
		if module != "" {
			suffixes = append(suffixes, "-"+module)
		}
	}

	// For models that do not provide module_id, there is only one type.
	if len(suffixes) == 0 {
		suffixes = append(suffixes, "")
	}

	for _, suffix := range suffixes {
		name := model + suffix
		info, ok := modelMap[name]
		if !ok {
			s.Fatal("Failed to get config for ", name)
		}

		yamlFile := "kernel_smoke_mipi/" + name + ".yaml"
		if err := configureMedia(ctx, info.BusInfo, s.DataPath(yamlFile)); err != nil {
			s.Fatal("Failed to configure: ", err)
		}

		if err := captureFrames(ctx, info.BusInfo, info.OutEntity); err != nil {
			s.Fatal("Failed to capture: ", err)
		}
	}
}
