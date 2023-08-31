// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

chrome.runtime.onMessageExternal.addListener(async (req, sender, res) => {
  let result = [];
  let data = "Extension sample message";

  try {
    if (req.type === "diagnostics") {
      result = result.concat(await handleDiagnostics(req.category));
    } else {
      result = data;
    }
  } catch (error) {
    result = result.concat([error.message])
  }

  return res(result);
});

const poll_diag = async (routine_id) => {
  param = { id: routine_id, command: 'status' }
  return await chrome.os.diagnostics.getRoutineUpdate(param)
}

const delay = ms => new Promise(res => setTimeout(res, ms));
const process_and_poll_continuous = async (routine_id) => {
  let result
  result = await poll_diag(routine_id)
  while (result.status == 'running') {
    await delay(1000);
    result = await poll_diag(routine_id)
  }

  return result
}

const handleDiagnostics = async (category) => {
  switch (category) {
    case 'audio_driver':
      resp = await chrome.os.diagnostics.runAudioDriverRoutine()
      break;
  }

  return await process_and_poll_continuous(resp.id)
}
