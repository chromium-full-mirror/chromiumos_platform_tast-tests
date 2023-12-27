// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

const JANUARY_1_2023 = new Date(2023, 0, 1);

async function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

class ProviderError extends Error {
  constructor(code) {
    console.warn('ProviderError', code)
    super(code);
    this.code = code;
  }
};

class Metadata {
  constructor(options) {
    this.options = options;
  }

  set(key, value) {
    if (this.options[key] && value !== undefined) {
      this[key] = value;
    }
  }
}

class Entity {
  constructor(name) {
    this.name = name;
    this.mtime = JANUARY_1_2023;
  }

  getSize() {
    return 0;
  }

  getMetadata(options) {
    const m = new Metadata(options);
    m.set('name', this.name);
    m.set('modificationTime', this.mtime);
    m.set('size', this.getSize());
    return m;
  }
};

class Dir extends Entity {
  constructor(name, children) {
    super(name);
    this.children = children;
  }

  getMetadata(options) {
    const m = super.getMetadata(options);
    m.set('isDirectory', true);
    return m;
  }
};

class File extends Entity {
  getMetadata(options) {
    const m = super.getMetadata(options);
    m.set('isDirectory', false);
    return m;
  }

  async read(options) {
    return new ArrayBuffer();
  }
};

class BigFizzBuzzFile extends File {
  constructor(name) {
    super(name);
  }

  getSize() {
    return 1234567;
  }

  async read(options) {
    let a = new Uint8Array(options.length);
    for (let i = 0; i < a.byteLength; i++) {
      let o = i + options.offset;
      // "Fizz-Buzz", but every 8192nd byte is also the 'page number'. (256 *
      // 8192) is more than the 1234567 that is this file's hard-coded size.
      if ((o % 8192) == 0) {
        a[i] = o / 8192;
      } else if ((o % 15) == 0) {
        a[i] = 0x78;  // 0x78 is ASCII 'x'.
      } else if ((o % 3) == 0) {
        a[i] = 0x66;  // 0x66 is ASCII 'f'.
      } else if ((o % 5) == 0) {
        a[i] = 0x62;  // 0x62 is ASCII 'b'.
      } else {
        a[i] = 0x30 + (o % 10);  // 0x30 is ASCII '0'.
      }
    }
    return a;
  }
};

class TextFile extends File {
  constructor(name, text) {
    super(name);
    this.array = new TextEncoder().encode(text);
  }

  getSize() {
    return this.array.byteLength;
  }

  async read(options) {
    return this.array.subarray(
      options.offset,
      options.offset + options.length
    );
  }
};

class SlowTextFile extends TextFile {
  constructor(name, text, delay) {
    super(name, text);
    this.delay = delay;
  }
  async read(options) {
    await sleep(this.delay);
    return await super.read(options);
  }
}

function createDirWithNFiles(name, n) {
  let a = new Array();
  for (let i = 0; i < n; i++) {
    a.push(new TextFile('foo' + i + '.txt', ''));
  }
  return new Dir(name, a);
}

let ROOT = new Dir('Test Filesystem', [
  createDirWithNFiles('123files', 123),
  new BigFizzBuzzFile('big-fizz-buzz-0.txt'),
  new BigFizzBuzzFile('big-fizz-buzz-1.txt'),
  new TextFile('empty.txt', ''),
  new TextFile('roses.txt', 'Roses are red.\n'),
  new SlowTextFile('slow-2s.txt', 'Two seconds delay.\n', 2000),
  new TextFile('this-is-the-fsp-extension.txt', ''),
]);

const REQ = {};

function getByPath(path) {
  if (path === '' || path === '/') {
    return ROOT;
  }
  let node = ROOT;
  for (const name of path.split('/').slice(1)) {
    node = node.children.find(c => c.name == name);
    if (!node) {
      return null;
    }
  }
  return node;
}

async function mount() {
  return await chrome.fileSystemProvider.mount({
    fileSystemId: 'FileSystemProviderTastExtension',
    displayName: 'FSPTastExt',
    writable: false,
  });
}

async function unmount(options) {
  return await chrome.fileSystemProvider.unmount({
    fileSystemId: options.fileSystemId,
  });
}

async function getMetadata(options) {
  const entry = getByPath(options.entryPath);
  if (!entry) {
    throw new ProviderError('NOT_FOUND');
  }
  return [entry.getMetadata(options)];
}

async function openFile(options) {
  REQ[options.requestId] = options.filePath;
}

async function readFile(options) {
  const path = REQ[options.openRequestId];
  if (!path) {
    throw new ProviderError('NOT_FOUND');
  }
  const entry = getByPath(path);
  if (!entry || !(entry instanceof File)) {
    throw new ProviderError('NOT_FOUND');
  }
  return [
    /*data=*/await entry.read(options),
    /*hasMore=*/false
  ];
}

async function closeFile(options) {
  if (REQ[options.openRequestId]) {
    delete REQ[options.openRequestId];
  }
}

async function readDirectory(options) {
  const entry = getByPath(options.directoryPath);
  if (!entry || !(entry instanceof Dir)) {
    throw new ProviderError('NOT_FOUND');
  }
  return [
    /*entries=*/entry.children.map(c => c.getMetadata(options)),
    /*hasMore=*/false
  ];
}

function asyncHandler(fn) {
  return (options, successCallback, errorCallback) => {
    fn(options).then((result) => {
      if (result === undefined) {
        successCallback();
      } else {
        successCallback(...result);
      }
    }).catch(e => {
      errorCallback((e instanceof ProviderError) ? e.code : 'FAILED');
    });
  }
}

if (chrome.fileSystemProvider) {
  for (const [name, handler] of [
      ['onUnmountRequested', unmount],
      ['onGetMetadataRequested', getMetadata],
      ['onOpenFileRequested', openFile],
      ['onReadFileRequested', readFile],
      ['onCloseFileRequested', closeFile],
      ['onReadDirectoryRequested', readDirectory],
  ]) {
    const evt = chrome.fileSystemProvider[name];
    if (evt) {
      evt.addListener(asyncHandler(handler));
    }
  }

  self.addEventListener('activate', e => {
    e.waitUntil(mount());
  });
}
