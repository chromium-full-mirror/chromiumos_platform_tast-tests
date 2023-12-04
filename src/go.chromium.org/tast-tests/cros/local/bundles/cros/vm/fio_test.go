// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

// To update test parameters after modifying this file, run:
// TAST_GENERATE_UPDATE=1 ~/chromiumos/src/platform/tast/tools/go.sh test -count=1 go.chromium.org/tast-tests/cros/local/bundles/cros/vm

import (
	"fmt"
	"testing"

	"go.chromium.org/tast-tests/cros/common/genparams"
)

func TestFio(t *testing.T) {
	type paramData struct {
		Name   string
		Kind   string
		Job    string
		DepDax string
		DepLvm string
	}

	jobs := []string{"randread", "randwrite", "randrw", "seqread", "seqwrite", "stress_rw"}
	kinds := []string{"block", "block_tpq", "block_lvm", "virtiofs", "virtiofs_dax", "scsi", "pmem"}

	var params []paramData
	for _, job := range jobs {
		for _, kind := range kinds {
			depDax := ""
			DepLvm := ""

			if kind == "virtiofs_dax" {
				depDax = "amd64"
			} else if kind == "block_lvm" {
				DepLvm = "lvm_stateful_partition"
			}

			params = append(params, paramData{
				Name:   fmt.Sprintf("%s_%s", kind, job),
				Kind:   kind,
				Job:    fmt.Sprintf("fio_%s.job", job),
				DepDax: depDax,
				DepLvm: DepLvm,
			})
		}
	}

	code := genparams.Template(t,
		`{{ range . }}{
			Name: {{ .Name | fmt }},
			ExtraData: []string{ {{ .Job | fmt }} },
			Val: param{
				kind: {{ .Kind | fmt }},
				job: {{ .Job | fmt }},
			},
			{{ if .DepDax }}
			// TODO(b/176129399): Remove this line once virtiofs DAX is enabled
			// on ARM.
			ExtraSoftwareDeps: []string { {{ .DepDax | fmt }} },
			{{ end }}
			{{ if .DepLvm }}
			ExtraSoftwareDeps: []string { {{ .DepLvm | fmt }} },
			{{ end }}
		},
		{{ end }}`,
		params)

	genparams.Ensure(t, "fio.go", code)
}
