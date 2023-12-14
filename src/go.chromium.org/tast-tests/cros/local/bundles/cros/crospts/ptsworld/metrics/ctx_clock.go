// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

const ctxClockTitle = "ctx_clock"

// ctxClockCrosboltNameTable maps the PTS description to crosbolt name.
var ctxClockCrosboltNameTable = map[string]string{
	"Context Switch Time": "Context_Switch_Time",
}
