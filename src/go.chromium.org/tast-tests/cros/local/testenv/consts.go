// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testenv

// Environment identifiers
const (
	Prod    = "prod"
	Preprod = "preprod"
	Fake    = "fake"
)

// ValidEnvs is a list of the environments used for the end-to-end testing
var ValidEnvs = []string{Prod, Preprod}

// MagicURL is a test URL locally served by mitmproxy to check that a mitmproxy starts with a root certificate successfully.
// If a git hook complains about the term used, you might want to bypass the check with --no-verify for a change to this file.
const MagicURL = "https://mitm.it/"

// DumpHTTPFlowURL is a URL to be used to dump HTTPFlow.
// It is defined in dump_http_flow.py.
const DumpHTTPFlowURL = "http://proxy_server:8080/traffic"
