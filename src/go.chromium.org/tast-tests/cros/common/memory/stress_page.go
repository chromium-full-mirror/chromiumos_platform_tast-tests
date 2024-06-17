// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"fmt"
	"html/template"

	"go.chromium.org/tast-tests/cros/common/utils"
)

const htmlTemplate = `
<!DOCTYPE html>
<html>
<head>
  <title>Alloc memory</title>
  <script>
function alloc(sizeMB, randomRatio) {
  const PAGE_SIZE = 4096;
  const FLOAT64_BYTES = 8;
  const FLOAT64_PER_PAGE = PAGE_SIZE / FLOAT64_BYTES;
  const MB = 1024 * 1024;
  const totalCount = sizeMB * MB / FLOAT64_BYTES;
  const randomCount = FLOAT64_PER_PAGE * randomRatio;
  // Using Float64Array as each element of Float64Array should consume 64
  // bits memory.
  const array = new Float64Array(totalCount);
  for (let i = 0; i < array.length; i++) {
    if (i % FLOAT64_PER_PAGE < randomCount) {
      array[i] = Math.random();
    } else {
      array[i] = 0.0;
    }
  }
  return array;
}

function main() {
  {{.}}
  const startTime = new Date();
  // Assigns the content to document to avoid optimization of unused data.
  document.out = alloc(allocMb, randomRatio);
  const ellapse = (new Date() - startTime) / 1000;
  // Shows the loading time for manual test.
  const content = "Allocating " + allocMb + " MB takes " + ellapse + " seconds";
  document.getElementById('display').textContent = content;
}

// Allocates javascript objects after the first rendering.
window.addEventListener('DOMContentLoaded', () => { setTimeout(main); });
  </script>
</head>
<body>
<div id="display">
  Allocating memory...
</div>
</body>
</html>
`

// CompileMemoryStressDataURL creates a data URL for memory stress test.
//
// Each data url is treated as different origin in Chrome even if the url is the same. With that, we
// can ensure that renderer processes are created per tab.
func CompileMemoryStressDataURL(allocMiB int, ratio float32) string {
	mainFunctionParams := fmt.Sprintf("const allocMb = %d; const randomRatio = %.3f;", allocMiB, ratio)
	tmpl, err := template.New("html").Parse(htmlTemplate)
	if err != nil {
		// The template must never fails.
		panic(err)
	}
	htmlBuffer := bytes.NewBuffer(nil)
	if err := tmpl.Execute(htmlBuffer, template.JS(mainFunctionParams)); err != nil {
		// The template must never fails.
		panic(err)
	}
	return utils.CompileHTMLDataURL(htmlBuffer.Bytes())
}
