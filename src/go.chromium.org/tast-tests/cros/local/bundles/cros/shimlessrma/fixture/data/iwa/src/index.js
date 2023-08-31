// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

const extensionId = "ihhhlnmoacmgjdemohllacakjndgpcii";

const print = (str, textarea_name) => {
  displayDiv = document.getElementById("display");
  textArea = document.createElement("textarea");
  textArea.setAttribute("class", textarea_name);
  textArea.value = JSON.stringify(str, null, 2);
  displayDiv.appendChild(textArea);
}

const make_request = (request, textarea_name) => {
  try {
    window.chrome.runtime.sendMessage(
      extensionId,
      request,
      (response) => {
        print(response, textarea_name)
      }
    );
  } catch(error) {
    print(error.message, textarea_name)
  }
}

document.getElementById('check-extension').addEventListener('click', event => {
  const request = {};
  make_request(request, 'check-extension-textarea');
});

const diag_categories = [
  "audio_driver",
  "cpu_floating_point_accuracy",
]

function listener(type, category, event) {
  const request = { type: type, category: category };
  make_request(request, `${type}-${category}-textarea`);
}

for (category of diag_categories) {
  buttonsDiv = document.getElementById("buttons");
  button = document.createElement("button");
  button.setAttribute("class", `diagnostics-${category}-button`);
  button.addEventListener('click',
      listener.bind(null, 'diagnostics', category));
  buttonsDiv.appendChild(button);
}
