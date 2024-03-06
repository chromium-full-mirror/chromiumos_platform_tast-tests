// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

var (
	// PassThroughPreprodGFE is a default map used to redirect all of *.google.com hosts to the pre-prod of Google frontend
	// Pass it in to the `testenv.RedirectMap` option for use.
	// A key-value pair should be pre-defined in vars/testenv.Google.yaml (private access required).
	PassThroughPreprodGFE = map[string]string{
		"google-prod":     "gfe-preprod",
		"googleapis-prod": "gfe-preprod",
	}
)
