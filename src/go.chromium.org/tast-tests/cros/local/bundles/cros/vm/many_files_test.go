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
		Name            string
		Kind            string
		Kernel          string
		Cache           string
		CaseFold        bool
		Deps            []string
		Fixture         string
		NegativeTimeout int
	}

	type fsCacheParam struct {
		policy          string
		negativeTimeout int
	}

	var params []paramData
	for _, p := range []struct {
		kernel  string
		dep     string
		fixture string
	}{{"arcvm", "android_vm", "chromeLoggedIn"}, {"termina", "dlc", "vmDLC"}} {
		// Block
		for _, kind := range []string{"block", "block_lvm"} {
			deps := []string{p.dep}
			if kind == "block_lvm" {
				deps = append(deps, "lvm_stateful_partition")
			}
			params = append(params, paramData{
				Name:    fmt.Sprintf("%s_%s", kind, p.kernel),
				Kernel:  p.kernel,
				Kind:    kind,
				Deps:    deps,
				Fixture: p.fixture,
			})
		}

		// Virtiofs
		for _, cache := range []fsCacheParam{
			{policy: "auto", negativeTimeout: 0},
			{policy: "always", negativeTimeout: 0},
			{policy: "always", negativeTimeout: 3600},
		} {
			deps := []string{p.dep}
			for _, caseFold := range []bool{false, true} {
				if cache.negativeTimeout > 0 && caseFold {
					// Negative cache is not supported with case-folding.
					continue
				}

				name := "virtiofs"
				if cache.policy == "always" {
					name += "_cached"
				}
				if cache.negativeTimeout > 0 {
					name += "_negativecache"
				}
				if caseFold {
					name += "_casefold"
				}
				name = fmt.Sprintf("%s_%s", name, p.kernel)

				params = append(params, paramData{
					Name:            name,
					Kind:            "virtiofs",
					Kernel:          p.kernel,
					Cache:           cache.policy,
					CaseFold:        caseFold,
					NegativeTimeout: cache.negativeTimeout,
					Deps:            deps,
					Fixture:         p.fixture,
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
				negativeTimeout: {{ .NegativeTimeout }},
			},
			Fixture: {{ .Fixture | fmt }},
			ExtraSoftwareDeps: {{ .Deps | fmt }},
		},
		{{ end }}`,
		params)

	genparams.Ensure(t, "many_files.go", code)
}
