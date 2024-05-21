// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

// To update test parameters after modifying this file, run:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/graphics

import (
	"bytes"
	"strings"
	"testing"
	"text/template"
	"time"

	"go.chromium.org/tast-tests/cros/common/genparams"
)

type dEQPGenParamData struct {
	Name           string        // Name of the subtest
	Timeout        time.Duration // Timeout of the subtest or each fraction of the subtest.
	Attr           []string      // Extra Attr for the subtest's test attribution
	ShardCount     int           // Number of fractions for the subtest. If none is set, it default to 1.
	API            string        // string representation of graphics.APIType to run.
	IsParallel     bool          // If set, run with the parallel_runner.
	IsSmoke        bool          // If set, run with the handpicked subset of tests.
	SoftwareDeps   []string      // List of softwareDeps for the test.
	HardwareModels []string      // Models to run the test against.
	SkipGPUFamily  []string      // GPU Families to skip it on.

	// The following should be autogen. Do not fill values.
	ShardNum     int    // Index of the fractions for the subtest. Should only be generated.
	HardwareDeps string // The formatted HardwareDeps string.
}

// This is selective models from each board. We tries to limit our deqp test run to these selective models to test running tast version of deqp in our lab whiling minimizing the impacts to others.
var selectiveModels = []string{
	"hayato",    // asurada
	"atlas",     // atlas
	"gladios",   // brask
	"redrix",    // brya
	"tomato",    // cherry
	"babytiger", // coral
	"ponyta",    // corsola
	"boton",     // dedede
	"drallion",  // drallion
	"sion",      // fizz
	"ciri",      // geralt
	"kasumi",    // grunt
	"kohaku",    // hatch
	"cozmo",     // jacuzzi
	"karma",     // kalista
	"kodama",    // kukui
	"yaviks",    // nissa
	"nocturne",  // nocturne
	"garfour",   // octopus
	"wyvern",    // puff
	"pyro",      // pyro
	"shyvana",   // rammus
	"electro",   // reef
	"sand",      // sand
	"sarien",    // sarien
	"skyrim",    // skyrim
	"soraka",    // soraka
	"starmi",    // staryu
	"homestar",  // strongbad
	"lazor",     // trogdor
	"lindar",    // volteer
	"berknip",   // zork
}

func addTests(t *testing.T, p dEQPGenParamData) []dEQPGenParamData {
	var result []dEQPGenParamData
	tmpl, err := template.New("").Parse(p.Name)
	if err != nil {
		t.Fatalf("Failed to parse template: %v", err)
	}

	var deps []string
	for _, ts := range []string{
		`{{ if .HardwareModels }}hwdep.Model({{range $i := .HardwareModels }}{{$i|fmt}},{{end}}){{end}}`,
		`{{ if .SkipGPUFamily }}hwdep.SkipGPUFamily({{range $i := .SkipGPUFamily }}{{$i|fmt}},{{end}}){{end}}`,
	} {
		condition := genparams.Template(t, ts, p)
		if condition == "" {
			continue
		}
		deps = append(deps, condition)
	}
	if len(deps) > 0 {
		p.HardwareDeps = "hwdep.D(" + strings.Join(deps, ",") + ")"
	}

	if p.API == "graphics.VK" {
		p.SoftwareDeps = append(p.SoftwareDeps, "vulkan")
	}

	for i := 0; i < p.ShardCount; i++ {
		newParam := dEQPGenParamData{
			Name:           p.Name,
			Timeout:        p.Timeout,
			Attr:           p.Attr,
			API:            p.API,
			IsParallel:     p.IsParallel,
			IsSmoke:        p.IsSmoke,
			HardwareModels: p.HardwareModels,
			SoftwareDeps:   p.SoftwareDeps,
			ShardCount:     p.ShardCount,
			ShardNum:       i + 1,
			HardwareDeps:   p.HardwareDeps,
		}
		// Replace the name field
		var tpl bytes.Buffer
		if err := tmpl.Execute(&tpl, newParam); err != nil {
			t.Fatalf("Failed to render template: %v", err)
		}
		newParam.Name = tpl.String()
		newParam.ShardNum--
		result = append(result, newParam)
	}
	return result
}

func TestDEQPParams(t *testing.T) {
	params := []dEQPGenParamData{{}}
	// Adding the smoke run for each APIType
	for _, s := range []struct {
		api  string
		name string
	}{
		{"graphics.VK", "vk"},
		{"graphics.GLES2", "gles2"},
		{"graphics.GLES3", "gles3"},
		{"graphics.GLES31", "gles31"},
	} {
		params = append(params, dEQPGenParamData{
			Name:       "smoke_" + s.name,
			API:        s.api,
			Timeout:    2 * time.Minute,
			Attr:       []string{"graphics_perbuild", "group:mainline", "informational", "group:criticalstaging"},
			ShardCount: 1,
			IsParallel: true,
			IsSmoke:    true,
		})
	}

	// Adding the normal run for each APIType
	params = append(params, []dEQPGenParamData{{
		Name:           `vk_{{.ShardCount}}_{{ printf "%02d" .ShardNum }}`,
		Timeout:        3 * time.Hour,
		Attr:           []string{"graphics_nightly"},
		API:            "graphics.VK",
		ShardCount:     10,
		IsParallel:     true,
		HardwareModels: selectiveModels,
		SkipGPUFamily:  []string{"rogue"},
	}, {
		Name:       `gles2`,
		Timeout:    30 * time.Minute,
		Attr:       []string{"graphics_nightly"},
		API:        "graphics.GLES2",
		ShardCount: 1,
		IsParallel: true,
	}, {
		Name:       `gles3`,
		Timeout:    2 * time.Hour,
		Attr:       []string{"graphics_nightly"},
		API:        "graphics.GLES3",
		ShardCount: 1,
		IsParallel: true,
	}, {
		Name:       `gles31`,
		Timeout:    2 * time.Hour,
		Attr:       []string{"graphics_nightly"},
		API:        "graphics.GLES31",
		ShardCount: 1,
		IsParallel: true,
	}}...)

	var tests []dEQPGenParamData
	for _, param := range params {
		tests = append(tests, addTests(t, param)...)
	}
	// This is the old smoke run that not using parallel_runner.
	tests = append(tests, dEQPGenParamData{
		Name:       "smoke",
		Timeout:    2 * time.Minute,
		Attr:       []string{"group:mainline", "group:cq-medium", "graphics_perbuild"},
		ShardCount: 1,
		IsParallel: false,
		IsSmoke:    true,
	})

	code := genparams.Template(t, `{{ range . }}{
		{{ if .Name }}
		Name: {{ .Name | fmt }},
		{{ end }}
		{{ if .Timeout }}
		Timeout: {{ .Timeout | fmt }},
		{{ end }}
		{{ if .Attr }}
		ExtraAttr: {{ .Attr | fmt }},
		{{ end }}
		{{ if .HardwareDeps }}
		ExtraHardwareDeps: {{ .HardwareDeps }},
		{{ end }}
		{{ if .SoftwareDeps }}
		ExtraSoftwareDeps: {{ .SoftwareDeps | fmt }},
		{{ end }}
		Val: deqpParams {
			{{ if .API }} api: {{ .API }}, {{ end }}
			{{ if .IsSmoke }} smoke: {{ .IsSmoke }}, {{ end }}
			isParallel: {{ .IsParallel }},
			{{ if .IsParallel }}
			shardNum: {{ if .ShardNum }}{{ .ShardNum | fmt }}{{ else }}0{{ end }},
			shardCount: {{ if .ShardCount }}{{ .ShardCount | fmt }}{{ else }}1{{ end }},
			{{ end }}
		},
	},
	{{ end }}`, tests)
	genparams.Ensure(t, "deqp.go", code)
}
