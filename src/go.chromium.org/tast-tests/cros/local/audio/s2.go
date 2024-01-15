// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"encoding/json"
)

// S2 is defined at
// https://source.chromium.org/chromiumos/chromiumos/codesearch/+/main:src/third_party/adhd/cras/server/s2/src/lib.rs
type S2 struct {
	Input struct {
		DLCManagerReady bool `json:"dlc_manager_ready"`
	}
	Output struct{}
}

func (c *Cras) dumpS2(ctx context.Context) (*S2, error) {
	var s2String string
	err := c.call(ctx, "DumpS2AsJSON").Store(&s2String)
	if err != nil {
		return nil, err
	}
	s2 := &S2{}
	err = json.Unmarshal([]byte(s2String), s2)
	return s2, err
}
