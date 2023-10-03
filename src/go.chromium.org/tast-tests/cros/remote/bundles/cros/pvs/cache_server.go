// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pvs

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/pvs/pvsutils"
	"go.chromium.org/tast/core/testing"
)

const cacheServerPort = 3335
const tarFileToDownload = "chromeos-moblab-pvs-dev/jackgelinas/hello.tar.gz"
const fileToExtract = "hi.txt"
const wantFileSha = "98ea6e4f216f2fb4b69fff9b3a44842c38686ca685f3f55dc48c5d3fb1107be4"

func init() {
	testing.AddTest(&testing.Test{
		Func:         CacheServer,
		Desc:         "Verifies cache server can fulfill a basic api request",
		BugComponent: "b:1110659",
		Contacts: []string{
			"chromeos-pvs-eng@google.com",
			"jackgelinas@google.com",
		},
		Attr:    []string{"group:pvs", "pvs_shop_daily"},
		Timeout: 20 * time.Minute,
		Fixture: "pvsShopUnpack",
	})
}

// CacheServer validates cache server works as correctly by calling the extract
// endpoint
func CacheServer(ctx context.Context, s *testing.State) {
	dut := s.DUT().Conn()
	extractURL := fmt.Sprintf("http://localhost:%v/extract/%v?file=%v", cacheServerPort, tarFileToDownload, fileToExtract)
	extractCmd := fmt.Sprintf("curl %v", extractURL)
	fileContents, _, err := pvsutils.RunAsRoot(ctx, dut, extractCmd)
	if err != nil {
		s.Fatal("Error occured when calling extract endpoint: ", err)
	}
	gotFileSha := fmt.Sprintf("%x", sha256.Sum256([]byte(fileContents)))
	if gotFileSha != wantFileSha {
		s.Fatalf("The file sha's do not match: got %q, want %q", gotFileSha, wantFileSha)
	}
}
