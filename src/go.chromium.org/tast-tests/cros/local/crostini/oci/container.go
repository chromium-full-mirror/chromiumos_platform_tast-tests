// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package oci

import (
	"fmt"
	"runtime"
	"strings"
)

// For docker.io/library/hello-world
const (
	HelloWorldImage     = "hello-world.tar"
	HelloWorldContainer = "hello-world"
	HelloWorldString    = "Hello from Docker!"
)

// ImageForArch returns the name of the container image for the host architecture.
func ImageForArch(imageName string) string {
	base := strings.TrimSuffix(imageName, ".tar")
	return fmt.Sprintf("%s-%s.tar", base, runtime.GOARCH)
}
