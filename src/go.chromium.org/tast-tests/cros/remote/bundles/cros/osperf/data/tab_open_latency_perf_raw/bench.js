// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

function getInnerTextOfTab(tabId) {
  return new Promise((resolve) => {
    chrome.scripting.executeScript(
        {
          func: () => {
            return document.body.innerText;
          },
          target: {
            tabId: tabId,
          }
        },
        (injectionResults) => {
          for (const frameResult of injectionResults) {
            const result = frameResult.result;
            resolve(result);
          }
        });
  });
}

async function getInnerTextOfUrl(url) {
  const tab = await chrome.tabs.create({url: url, active: false});
  const text = await getInnerTextOfTab(tab.id);
  await chrome.tabs.remove(tab.id);
  return text;
}

function extractBiosInfoAttr(biosInfoLines, key) {
  return biosInfoLines.filter((s) => s.startsWith(key))[0]
      .split(' = ')[1]
      .split('#')[0]
      .trim();
}

const runCycle = async function(numTabs) {
  // returns: tab open latencies: [Number; numTabs]
  const result = [];
  const tabIdToBeRemovedList = [];
  for (let i = 0; i < numTabs; i++) {
    const t0 = performance.now();
    // Open a tab with a data URL, to make sure that it gets an unique security
    // context (which means the tab is created in a new process.)
    // c.f.
    // https://developer.mozilla.org/en-US/docs/Web/Security/Same-origin_policy#inherited_origins
    const tid = (await chrome.tabs.create({
                  url: 'data:,Hello%2C%20World%21',
                  active: false
                })).id;
    while (true) {
      const t = await chrome.tabs.get(tid);
      if (t.status === 'complete') {
        tabIdToBeRemovedList.push(tid);
        break;
      }
    }
    const t1 = performance.now();
    const diff = t1 - t0;
    result.push(diff);
  }
  for (const tabId of tabIdToBeRemovedList) {
    do {
      try {
        await chrome.tabs.remove(tabId);
      } catch {
        console.log('Remove failed. Retrying.')
        continue;
      }
    } while (0)
  }
  return result;
};

const runBench = async function(numCycles, numTabs) {
  // returns: latencies: [[Number; iterCount]; numCycles]
  const result = [];
  for (let i = 0; i < numCycles; i++) {
    result.push(await runCycle(numTabs));
  }
  return result;
};

const log = (div, s) => {
  div.innerText += s;
  div.innerText += '\n';
};

document.addEventListener('DOMContentLoaded', function() {
  const takeLogButton = document.getElementById('takeLogButton');
  const benchButton = document.getElementById('benchButton');
  const copyResultButton = document.getElementById('copyResultButton');
  const shortResultPre = document.getElementById('shortResultPre');
  const benchResultPre = document.getElementById('benchResultPre');

  const startBench = async () => {
    const numTabsInput = document.getElementById('numTabsInput');
    const numTabs = parseInt(numTabsInput.value);

    const numCyclesInput = document.getElementById('numCyclesInput');
    const numCycles = parseInt(numCyclesInput.value);

    const numIterationsInput = document.getElementById('numIterations');
    const numIterations = parseInt(numIterationsInput.value);

    const result = [];
    let iterCount = 0;
    const runBenchAndProcess = async () => {
      iterCount++;
      const r = await runBench(numCycles, numTabs);
      result.push(r.flat());
      log(benchResultPre,
          `${(new Date()).toISOString()},${iterCount},${r}`);
    };
    const t0 = performance.now();
    for (let i = 0; i < numIterations; i++) {
      await runBenchAndProcess();
    }
    const t1 = performance.now();
    const totalDuration = t1 - t0;
    log(shortResultPre,
        `${(new Date()).toISOString()},${result.length},${numIterations},${
            totalDuration}`);
    const info = {
      tab_open_latencies: result.flat(),
      total_duration: totalDuration,
      num_iterations: numIterations,
      result_log: benchResultPre.innerText,
    };
    console.log(info);
    return info;
  };
  window.startBench = startBench;
  benchButton.addEventListener('click', startBench);
  async function takeLog(benchResultPre) {
    const biosInfo = await getInnerTextOfUrl('file:///var/log/bios_info.txt');
    const biosInfoLines = biosInfo.split('\n');
    const hwid = extractBiosInfoAttr(biosInfoLines, 'hwid');
    const fwid = extractBiosInfoAttr(biosInfoLines, 'fwid');
    log(benchResultPre, `0,hwid,${hwid}`);
    log(benchResultPre, `0,fwid,${fwid}`);
  }
  takeLogButton.addEventListener('click', async () => {
    await takeLog(benchResultPre);
  });
  copyResultButton.addEventListener('click', async () => {
    navigator.clipboard.writeText(benchResultPre.innerText)
        .then(
            () => {
              const old = copyResultButton.innerText;
              copyResultButton.innerText = 'OK!';
              setTimeout(() => {
                copyResultButton.innerText = old;
              }, 1000);
            },
            () => {
              const old = copyResultButton.innerText;
              copyResultButton.innerText = 'Failed...';
              setTimeout(() => {
                copyResultButton.innerText = old;
              }, 1000);
            },
        );
  });
});
