// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package capture provides a helper to capture and parse traffic from an
// interface while running tests. It is based on the google.com/gopacket
// library.
//
// Example:
//
//	func test() {
//	    c := capture.NewCapturer("eth0")
//	    if err := c.Start(ctx); err != nil {
//	        ...
//	    }
//	    defer c.Stop()
//
//	    ...
//
//	    for p := range c.Packets() {
//	        ...
//	    }
//	}
package capture
