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
		Name     string
		Protocol netProtocol
		Kernel   guestKernel
		Dep      string
		Fixture  string
	}

	protocols := []netProtocol{tcp, udp, udpReverse}
	kernels := []guestKernel{arcvm, termina}

	var params []paramData
	for _, protocol := range protocols {
		for _, kernel := range kernels {
			dep := ""
			fixture := ""

			protocolName := fmt.Sprintf("%s", protocol)
			if protocol == udpReverse {
				protocolName = "udp_reverse"
			}

			if kernel == termina {
				dep = "dlc"
				fixture = "vmDLC"
			} else if kernel == arcvm {
				dep = "android_vm"
				fixture = "chromeLoggedIn"
			}

			params = append(params, paramData{
				Name:     fmt.Sprintf("%s_packed_%s", protocolName, kernel),
				Protocol: protocol,
				Kernel:   kernel,
				Dep:      dep,
				Fixture:  fixture,
			})
		}
	}

	code := genparams.Template(t,
		`{{ range . }}{
			Name: {{ .Name | fmt }},
			Val: iperfParam{
				protocol: {{ .Protocol }},
				kernel: {{ .Kernel }},
			},
			Fixture: {{ .Fixture | fmt }} ,
			ExtraSoftwareDeps: []string { {{ .Dep | fmt }} },
		},
		{{ end }}`,
		params)

	genparams.Ensure(t, "iperf.go", code)
}
