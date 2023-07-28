// Copyright 2023 The ChromiumOS Authors
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

func TestIperf(t *testing.T) {
	type paramData struct {
		Name   string
		VqType virtqueueType
	}

	vqTypes := []virtqueueType{split, packed}

	var params []paramData
	for _, vt := range vqTypes {
		params = append(params, paramData{
			Name:   fmt.Sprintf("tcp_%s", vt),
			VqType: vt,
		})
	}

	code := genparams.Template(t,
		`{{ range . }}{
			Name: {{ .Name | fmt }},
			Val: iperfParam{
				vqType: {{ .VqType }},
			},
		},
		{{ end }}`,
		params)

	genparams.Ensure(t, "iperf.go", code)
}
