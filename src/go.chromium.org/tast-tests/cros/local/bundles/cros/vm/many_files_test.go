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

func TestManyFiles(t *testing.T) {
	type paramData struct {
		Name     string
		Kind     string
		Kernel   string
		Cache    string
		CaseFold bool
		Dep      string
		Fixture  string
	}

	var params []paramData
	for _, p := range []struct {
		kernel  string
		dep     string
		fixture string
	}{{"arcvm", "android_vm", "chromeLoggedIn"}, {"termina", "dlc", "vmDLC"}} {
		// Block
		for _, kind := range []string{"block", "block_lvm"} {
			params = append(params, paramData{
				Name:    fmt.Sprintf("%s_%s", kind, p.kernel),
				Kernel:  p.kernel,
				Kind:    kind,
				Dep:     p.dep,
				Fixture: p.fixture,
			})
		}

		// Virtiofs
		for _, cache := range []string{"auto", "always"} {
			for _, caseFold := range []bool{false, true} {
				name := "virtiofs"
				if cache == "always" {
					name += "_cached"
				}
				if caseFold {
					name += "_casefold"
				}
				name = fmt.Sprintf("%s_%s", name, p.kernel)

				params = append(params, paramData{
					Name:     name,
					Kind:     "virtiofs",
					Kernel:   p.kernel,
					Cache:    cache,
					CaseFold: caseFold,
					Dep:      p.dep,
					Fixture:  p.fixture,
				})
			}
		}
	}

	code := genparams.Template(t,
		`{{ range . }}{
			Name: {{ .Name | fmt }},
			Val: manyFilesParams{
				kernel: {{ .Kernel | fmt }},
				kind: {{ .Kind | fmt }},
				cache: {{ .Cache | fmt }},
				caseFold: {{ .CaseFold }},
			},
			Fixture: {{ .Fixture | fmt }},
			ExtraSoftwareDeps: []string { {{ .Dep | fmt }} },
		},
		{{ end }}`,
		params)

	genparams.Ensure(t, "many_files.go", code)
}
