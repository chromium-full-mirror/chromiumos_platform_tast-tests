// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

(window as any).testApp = {};

interface ASN1Value {
  tag: number;
  headerLen: number;
  val: ByteString;
}

const enum Padding {
  None = 0,
  Include = 1,
}

const enum Tag {
  BOOLEAN = 1,
  INTEGER = 2,
  BITSTRING = 3,
  OCTETSTRING = 4,
  NULL = 5,
  OBJECT = 6,
  UTF8String = 12,
  NumericString = 18,
  PrintableString = 19,
  IA5String = 22,
  UTCTime = 23,
  GeneralizedTime = 24,

  CONSTRUCTED = 0x20,
  SEQUENCE = 0x30,
  SET = 0x31,
  CONTEXT_SPECIFIC = 0x80,
}

class ByteString {
  slice: Uint8Array;

  constructor(buffer: Uint8Array) {
    this.slice = buffer;
  }

  get data(): Uint8Array {
    return this.slice;
  }

  get length(): number {
    return this.slice.length;
  }

  get empty(): boolean {
    return this.slice.length == 0;
  }

  get hex(): string {
    const hexTable = '0123456789abcdef';
    let s = '';

    for (let i = 0; i < this.data.length; i++) {
      s += hexTable.charAt(this.data[i] >> 4);
      s += hexTable.charAt(this.data[i] & 15);
    }

    return s;
  }

  private base64Encode(chars: string, padding: Padding): string {
    const len3 = 3 * Math.floor(this.slice.length / 3);
    var chunks: string[] = [];

    for (let i = 0; i < len3; i += 3) {
      const v = (this.slice[i] << 16) + (this.slice[i + 1] << 8) +
        this.slice[i + 2];
      chunks.push(
        chars[v >> 18] + chars[(v >> 12) & 0x3f] + chars[(v >> 6) & 0x3f] +
        chars[v & 0x3f]);
    }

    const remainder = this.slice.length - len3;
    if (remainder == 1) {
      const v = this.slice[len3];
      chunks.push(chars[v >> 2] + chars[(v << 4) & 0x3f]);
      if (padding == Padding.Include) {
        chunks.push('==');
      }
    } else if (remainder == 2) {
      const v = (this.slice[len3] << 8) + this.slice[len3 + 1];
      chunks.push(
        chars[v >> 10] + chars[(v >> 4) & 0x3f] + chars[(v << 2) & 0x3f]);
      if (padding == Padding.Include) {
        chunks.push('=');
      }
    }

    return chunks.join('');
  }

  webSafeBase64(): string {
    const chars =
      'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_'
    return this.base64Encode(chars, Padding.None);
  }

  base64(): string {
    const chars =
      'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'
    return this.base64Encode(chars, Padding.Include);
  }

  compare(other: ByteString): -1 | 0 | 1 {
    if (this.length < other.length) {
      return -1;
    } else if (this.length > other.length) {
      return 1;
    }

    for (let i = 0; i < this.length; i++) {
      if (this.slice[i] < other.slice[i]) {
        return -1;
      } else if (this.slice[i] > other.slice[i]) {
        return 1;
      }
    }

    return 0;
  }

  getU8(): number {
    if (this.empty) {
      throw ('ByteString: empty during getU8');
    }

    const byte = this.slice[0];
    this.slice = this.slice.subarray(1);
    return byte;
  }

  skip(n: number) {
    if (this.length < n) {
      throw ('ByteString: too few bytes to skip');
    }
    this.slice = this.slice.subarray(n);
  }

  getBytes(n: number): Uint8Array {
    if (this.length < n) {
      throw ('ByteString: insufficient bytes in getBytes');
    }

    const ret = this.slice.subarray(0, n);
    this.slice = this.slice.subarray(n);
    return ret;
  }

  private getUnsigned(n: number): number {
    const bytes = this.getBytes(n);
    let value = 0;
    for (let i = 0; i < n; i++) {
      value <<= 8;
      value |= bytes[i];
    }
    return value;
  }

  getU16(): number {
    return this.getUnsigned(2);
  }

  getU32(): number {
    return this.getUnsigned(4);
  }

  private getASN1_(expectedTag: number, includeHeader: boolean) {
    if (this.empty) {
      throw 'getASN1: empty slice, expected tag ' + expectedTag;
    }
    const v = this.getAnyASN1();
    if (v.tag != expectedTag) {
      throw 'getASN1: got tag ' + v.tag + ', want ' + expectedTag;
    }
    if (!includeHeader) {
      v.val.skip(v.headerLen);
    }
    return v.val;
  }

  getASN1(expectedTag: number): ByteString {
    return this.getASN1_(expectedTag, false);
  }

  getASN1Element(expectedTag: number): ByteString {
    return this.getASN1_(expectedTag, true);
  }

  getOptionalASN1(expectedTag: number): ByteString | null {
    if (this.slice.length < 1 || this.slice[0] != expectedTag) {
      return null;
    }
    return this.getASN1(expectedTag);
  }

  getAnyASN1(): ASN1Value {
    const header = new ByteString(this.slice);
    const tag = header.getU8();
    const lengthByte = header.getU8();

    if ((tag & 0x1f) == 0x1f) {
      throw 'getAnyASN1: long-form tag found';
    }

    let len = 0;
    let headerLen = 0;

    if ((lengthByte & 0x80) == 0) {
      // Short form length.
      len = lengthByte + 2;
      headerLen = 2;
    } else {
      // The high bit indicates that this is the long form, while the next 7
      // bits encode the number of subsequent octets used to encode the length
      // (ITU-T X.690 clause 8.1.3.5.b).
      const numBytes = lengthByte & 0x7f;

      // Bitwise operations are always on signed 32-bit two's complement
      // numbers.  This check ensures that we stay under this limit.  We could
      // do this in a better way, but there's no need to process very large
      // objects.
      if (numBytes == 0 || numBytes > 3) {
        throw 'getAnyASN1: bad ASN.1 long-form length';
      }
      const lengthBytes = header.getBytes(numBytes);
      for (let i = 0; i < numBytes; i++) {
        len <<= 8;
        len |= lengthBytes[i];
      }

      if (len < 128 || (len >> ((numBytes - 1) * 8)) == 0) {
        throw 'getAnyASN1: incorrectly encoded ASN.1 length';
      }

      headerLen = 2 + numBytes;
      len += headerLen;
    }

    if (this.slice.length < len) {
      throw 'getAnyASN1: too few bytes in input';
    }
    const prefix = this.slice.subarray(0, len);
    this.slice = this.slice.subarray(len);
    return { tag: tag, headerLen: headerLen, val: new ByteString(prefix) };
  }

  private getBase128Int(): number {
    let lookahead = this.slice.length;
    if (lookahead > 4) {
      lookahead = 4;
    }

    let len = 0;
    for (let i = 0; i < lookahead; i++) {
      if (!(this.slice[i] & 0x80)) {
        len = i + 1;
        break;
      }
    }

    if (len == 0) {
      throw 'base128 value too large';
    }

    let n = 0;
    let octets = this.getBytes(len);
    for (let i = 0; i < len; i++) {
      if ((n & 0xff000000) != 0) {
        throw 'base128 value too large';
      }
      n <<= 7;
      n |= octets[i] & 0x7f;
    }

    return n;
  }

  getASN1ObjectIdentifier(): number[] {
    let b = this.getASN1(Tag.OBJECT);
    let first = b.getBase128Int();
    let result = [0, 0];
    result[1] = first % 40;
    result[0] = (first - result[1]) / 40;

    while (!b.empty) {
      result.push(b.getBase128Int());
    }

    return result;
  }

  private getCBORHeader(): [number, number, ByteString] {
    const copy = new ByteString(this.slice);
    const a = this.getU8();
    const majorType = a >> 5;
    const info = a & 31;

    if (info < 24) {
      return [majorType, info, new ByteString(copy.getBytes(1))];
    } else if (info < 28) {
      const lengthLength = 1 << (info - 24);
      let data = this.getBytes(lengthLength);
      let value = 0;
      for (let i = 0; i < lengthLength; i++) {
        // Javascript has problems handling uint64s given the limited range of
        // a double.
        if (value > 35184372088831) {
          throw ('ByteString: cannot represent CBOR number');
        }

        // Not using bitwise operations to avoid truncating to 32 bits.
        value *= 256;
        value += data[i];
      }

      switch (lengthLength) {
        case 1:
          if (value < 24) {
            throw (
              'ByteString: value should have been encoded in single byte');
          }
          break;

        case 2:
          if (value < 256) {
            throw ('ByteString: non-minimal integer');
          }
          break;

        case 4:
          if (value < 65536) {
            throw ('ByteString: non-minimal integer');
          }
          break;

        case 8:
          if (value < 4294967296) {
            throw ('ByteString: non-minimal integer');
          }
          break;
      }

      return [
        majorType, value, new ByteString(copy.getBytes(1 + lengthLength))
      ];
    } else {
      throw ('ByteString: CBOR contains unhandled info value ' + info);
    }
  }

  getCBOR(): any {
    const [major, value] = this.getCBORHeader();
    switch (major) {
      case 0:
        return value;
      case 1:
        return 0 - (1 + value);
      case 2:
        return this.getBytes(value);
      case 3:
        return new TextDecoder().decode(this.getBytes(value));
      case 4: {
        let ret = new Array(value);
        for (let i = 0; i < value; i++) {
          ret[i] = this.getCBOR();
        }
        return ret;
      }
      case 5:
        if (value == 0) {
          return {};
        }

        let copy = new ByteString(this.data);
        const [firstKeyMajor] = copy.getCBORHeader();

        if (firstKeyMajor == 3) {
          // String-keyed map.
          let lastKeyHeader: ByteString = new ByteString(new Uint8Array(0));
          let lastKeyBytes: ByteString = new ByteString(new Uint8Array(0));

          let ret: { [key: string]: any } = {};
          for (let i = 0; i < value; i++) {
            const [keyMajor, keyLength, keyHeader] = this.getCBORHeader();
            if (keyMajor != 3) {
              throw ('ByteString: non-string in string-valued map');
            }
            const keyBytes = new ByteString(this.getBytes(keyLength));

            if (i > 0) {
              const headerCmp = lastKeyHeader.compare(keyHeader);
              if (headerCmp > 0 ||
                (headerCmp == 0 && lastKeyBytes.compare(keyBytes) >= 0)) {
                throw (
                  'ByteString: map keys in wrong order: ' +
                  lastKeyHeader.hex + '/' + lastKeyBytes.hex + ' ' +
                  keyHeader.hex + '/' + keyBytes.hex);
              }
            }
            lastKeyHeader = keyHeader;
            lastKeyBytes = keyBytes;

            ret[keyBytes.parseUTF8()] = this.getCBOR();
          }

          return ret;
        } else if (firstKeyMajor == 0 || firstKeyMajor == 1) {
          // Number-keyed map.
          let lastKeyHeader: ByteString = new ByteString(new Uint8Array(0));

          let ret: { [key: number]: any } = {};
          for (let i = 0; i < value; i++) {
            let [keyMajor, keyValue, keyHeader] = this.getCBORHeader();
            if (keyMajor != 0 && keyMajor != 1) {
              throw ('ByteString: non-number in number-valued map');
            }

            if (i > 0 && lastKeyHeader.compare(keyHeader) >= 0) {
              throw (
                'ByteString: map keys in wrong order: ' +
                lastKeyHeader.hex + ' ' + keyHeader.hex);
            }
            lastKeyHeader = keyHeader;

            if (keyMajor == 1) {
              keyValue = 0 - (1 + keyValue);
            }

            ret[keyValue] = this.getCBOR();
          }

          return ret;
        } else {
          throw (
            'ByteString: map keyed by invalid major type ' + firstKeyMajor);
        }
      default:
        throw ('ByteString: unhandled major type ' + major);
    }
  }

  parseUTF8(): string {
    return (new TextDecoder('utf-8')).decode(this.slice)
  }
}

/**
 * ASN.1 builder, in the manner of BoringSSL's CBB (crypto byte builder).
 *
 * A |ByteBuilder| maintains a |Uint8Array| slice and appends to it on demand.
 * After appending all the necessary values, the |data| property returns a
 * slice containing the result. Utility functions are provided for appending
 * ASN.1 DER-formatted values.
 *
 * Several of the functions take a "continuation" parameter. This is a
 * function that makes calls to its argument in order to lay down the contents
 * of a value. Once the continuation returns, the length prefix will be
 * serialised. It is illegal to call methods on a parent ByteBuilder while a
 * continuation function is running.
 */
class ByteBuilder {
  slice: Uint8Array = new Uint8Array(128);
  len: number = 0;
  child: ByteBuilder | null = null;

  /**
   * @return {!Uint8Array} The constructed bytes
   */
  get data() {
    if (this.child != null) {
      throw Error('data access while child is pending');
    }
    return this.slice.subarray(0, this.len);
  }

  /**
   * Reallocates the slice to at least a given size.
   * @param {number} minNewSize The minimum resulting size of the slice.
   * @private
   */
  private realloc(minNewSize: number) {
    let newSize = 0;

    if (minNewSize > Number.MAX_SAFE_INTEGER - minNewSize) {
      // Cannot grow exponentially without overflow.
      newSize = minNewSize;
    } else {
      newSize = minNewSize * 2;
    }

    const newSlice = new Uint8Array(newSize);
    for (var i = 0; i < this.len; i++) {
      newSlice[i] = this.slice[i];
    }

    this.slice = newSlice;
  }

  /**
   * Extends the current slice by the given number of bytes.
   * @param {number} n The number of extra bytes needed in the slice.
   * @return {number} The offset of the new bytes.
   * @throws {Error}
   * @private
   */
  private extend(n: number) {
    if (this.child != null) {
      throw Error('write while child pending');
    }
    if (this.len > Number.MAX_SAFE_INTEGER - n) {
      throw Error('length overflow');
    }
    if (this.len + n > this.slice.length) {
      this.realloc(this.len + n);
    }

    const offset = this.len;
    this.len += n;
    return offset;
  }

  /**
   * Appends a uint8 to the slice.
   * @param {number} b The byte to append.
   * @throws {Error}
   * @private
   */
  addU8(b: number) {
    const offset = this.extend(1);
    this.slice[offset] = b;
  }

  /**
   * Appends a length prefixed value to the slice.
   * @param {number} lenLen The number of length-prefix bytes.
   * @param {boolean} isASN1 True iff an ASN.1 length should be prefixed.
   * @param {function(ByteBuilder)} k A function to construct the contents.
   * @throws {Error}
   * @private
   */
  private addLengthPrefixed(
    lenLen: number, isASN1: boolean, k: (child: ByteBuilder) => void) {
    let offset = this.extend(lenLen);
    let child = new ByteBuilder();
    child.slice = this.slice;
    child.len = this.len;
    this.child = child;
    k(child);

    var length = child.len - lenLen - offset;
    if (length > 0x7fffffff) {
      // If a number larger than this is used with a shift operation in
      // Javascript, the result is incorrect.
      throw Error('length too large');
    }

    if (isASN1) {
      // In the case of ASN.1 a single byte was reserved for
      // the length. The contents of the array may need to be
      // shifted along if the length needs more than that.
      if (lenLen != 1) {
        throw Error('internal error');
      }

      let lenByte = 0;
      if (length > 0xffffff) {
        lenLen = 5;
        lenByte = 0x80 | 4;
      } else if (length > 0xffff) {
        lenLen = 4;
        lenByte = 0x80 | 3;
      } else if (length > 0xff) {
        lenLen = 3;
        lenByte = 0x80 | 2;
      } else if (length > 0x7f) {
        lenLen = 2;
        lenByte = 0x80 | 1;
      } else {
        lenLen = 1;
        lenByte = length;
        length = 0;
      }

      child.slice[offset] = lenByte;
      const extraBytesNeeded = lenLen - 1;
      if (extraBytesNeeded > 0) {
        child.extend(extraBytesNeeded);
        child.slice.copyWithin(offset + lenLen, offset + 1, child.len);
      }

      offset++;
      lenLen = extraBytesNeeded;
    }

    let l = length;
    for (let i = lenLen - 1; i >= 0; i--) {
      child.slice[offset + i] = l;
      l >>= 8;
    }

    if (l != 0) {
      throw Error('pending child length exceeds reserved space');
    }

    this.slice = child.slice;
    this.len = child.len;
    this.child = null;
  }

  /**
   * Appends an ASN.1 element to the slice.
   * @param {number} tag The ASN.1 tag value (must be < 31).
   * @param {function(ByteBuilder)} k A function to construct the contents.
   * @throws {Error}
   */
  addASN1(tag: number, k: (child: ByteBuilder) => void) {
    if (tag > 255) {
      throw Error('high-tag values not supported');
    }
    this.addU8(tag);
    this.addLengthPrefixed(1, true, k);
  }

  /**
   * Appends an ASN.1 INTEGER to the slice.
   * @param {number} n The value of the integer. Must be within the range of
   *     an int32.
   * @throws {Error}
   */
  addASN1Int(n: number) {
    if (n < (0x80000000 << 0) || n > 0x7fffffff) {
      // Numbers this large (or small) cannot be correctly shifted in
      // Javascript.
      throw Error('integer out of encodable range');
    }

    let length = 1;
    for (let nn = n; nn >= 0x80 || nn <= -0x80; nn >>= 8) {
      length++;
    }

    this.addASN1(Tag.INTEGER, (b) => {
      for (let i = length - 1; i >= 0; i--) {
        b.addU8((n >> (8 * i)) & 0xff);
      }
    });
  }

  /**
   * Appends a non-negative ASN.1 INTEGER to the slice given its big-endian
   *     encoding. This can be useful when interacting with the WebCrypto API.
   * @param {!Uint8Array} bytes The big-endian encoding of the integer.
   * @throws {Error}
   */
  addASN1BigInt(bytes: Uint8Array) {
    // Zero is represented as a single zero byte, rather than no bytes.
    if (bytes.length == 0) {
      bytes = new Uint8Array(1);
    }

    // Leading zero bytes need to be removed, unless that would make the
    // number negative.
    while (bytes.length >= 2 && bytes[0] == 0 && (bytes[1] & 0x80) == 0) {
      bytes = bytes.slice(1);
    }

    // If the MSB is set, the number will be considered to be negative. Thus
    // a zero prefix is needed in that case.
    if (bytes.length > 0 && (bytes[0] & 0x80) == 0x80) {
      if (bytes.length > Number.MAX_SAFE_INTEGER - 1) {
        throw Error('bigint array too long');
      }
      let newBytes = new Uint8Array(bytes.length + 1);
      newBytes.set(bytes, 1);
      bytes = newBytes;
    }

    this.addASN1(Tag.INTEGER, (b) => b.addBytes(bytes));
  }

  /**
   * Appends a base128-encoded integer to the slice.
   * @param {number} n The value of the integer. Must be non-negative and
   *     within the range of an int32.
   * @throws {Error}
   * @private
   */
  private addBase128Int(n: number) {
    if (n < 0 || n > 0x7fffffff) {
      // Cannot encode negative numbers and large numbers cannot be shifted in
      // Javascript.
      throw Error('integer out of encodable range');
    }

    let length = 0;
    if (n == 0) {
      length = 1;
    } else {
      for (let i = n; i > 0; i >>= 7) {
        length++;
      }
    }

    for (let i = length - 1; i >= 0; i--) {
      let octet = 0x7f & (n >> (7 * i));
      if (i != 0) {
        octet |= 0x80;
      }
      this.addU8(octet);
    }
  }

  /**
   * Appends an OBJECT IDENTIFIER to the slice.
   * @param {Array<number>} oid The OID as a list of integer elements.
   * @throws {Error}
   */
  addASN1ObjectIdentifier(oid: number[]) {
    if (oid.length < 2 || oid[0] > 2 || (oid[0] <= 1 && oid[1] >= 40)) {
      throw Error('invalid OID');
    }

    this.addASN1(Tag.OBJECT, (b) => {
      b.addBase128Int(oid[0] * 40 + oid[1]);
      for (var i = 2; i < oid.length; i++) {
        b.addBase128Int(oid[i]);
      }
    });
  }

  /**
   * Appends an ASN.1 NULL to the slice.
   * @throws {Error}
   */
  addASN1Null() {
    const offset = this.extend(2);
    this.slice[offset] = Tag.NULL;
    this.slice[offset + 1] = 0;
  }

  /**
   * Appends an ASN.1 PrintableString to the slice.
   * @param {string} s The contents of the string.
   * @throws {Error}
   */
  addASN1PrintableString(s: string) {
    var buf = new Uint8Array(s.length);
    for (var i = 0; i < s.length; i++) {
      const code = s.charCodeAt(i);
      if ((code < 97 && code > 122) &&  // a-z
        (code < 65 && code > 90) &&   // A-Z
        ' \'()+,-/:=?'.indexOf(String.fromCharCode(code)) == -1) {
        throw Error(
          'cannot encode \'' + String.fromCharCode(code) + '\' in' +
          ' PrintableString');
      }

      buf[i] = code;
    }

    this.addASN1(Tag.PrintableString, (b) => {
      b.addBytes(buf);
    });
  }

  /**
   * Appends an ASN.1 UTF8String to the slice.
   * @param {string} s The contents of the string.
   * @throws {Error}
   */
  addASN1UTF8String(s: string) {
    this.addASN1(Tag.UTF8String, (b) => {
      b.addBytes((new TextEncoder()).encode(s));
    });
  }

  /**
   * Appends an ASN.1 BIT STRING to the slice.
   * @param {!Uint8Array} bytes The contents, which must be a whole number of
   *     bytes.
   * @throws {Error}
   */
  addASN1BitString(bytes: Uint8Array) {
    this.addASN1(Tag.BITSTRING, (b) => {
      b.addU8(0);  // no superfluous bits in encoding.
      b.addBytes(bytes);
    });
  }

  /**
   * Appends raw data to the slice.
   * @param {string} s The contents to append. All character values must
   *     be < 256.
   * @throws {Error}
   */
  addBytesFromString(s: string) {
    const buf = new Uint8Array(s.length);
    for (var i = 0; i < s.length; i++) {
      const code = s.charCodeAt(i);
      if (code > 255) {
        throw Error('out-of-range character in string of bytes');
      }
      buf[i] = code;
    }

    this.addBytes(buf);
  }

  /**
   * Appends raw bytes to the slice.
   * @param {!Array<number>|!Uint8Array} bytes Data to append.
   * @throws {Error}
   */
  addBytes(bytes: Uint8Array) {
    const offset = this.extend(bytes.length);
    for (var i = 0; i < bytes.length; i++) {
      this.slice[offset + i] = bytes[i];
    }
  }
};

function arrayEqual(a: number[], b: number[]): boolean {
  if (a.length != b.length) {
    return false;
  }

  for (let i = 0; i < a.length; i++) {
    if (a[i] != b[i]) {
      return false;
    }
  }

  return true;
}

function webSafeBase64ToNormal(s: string): string {
  return s.replace(/-/g, '+').replace(/_/g, '/');
}

function decodeWebSafeBase64(s: string): ByteString {
  let bytes = atob(webSafeBase64ToNormal(s));
  let buffer = new ArrayBuffer(bytes.length);
  let ret = new Uint8Array(buffer);
  for (let i = 0; i < bytes.length; i++) {
    ret[i] = bytes.charCodeAt(i);
  }
  return new ByteString(ret);
}

function randomChallenge(len: number): Uint8Array {
  let challenge = new Uint8Array(len);
  window.crypto.getRandomValues(challenge);
  return challenge;
}

function trimASN1Int(i: ByteString): ByteString {
  while (i.length > 0 && i.data[0] == 0) {
    i.skip(1);
  }
  return i;
}

function ecdsaSignatureToWebCryptoFormat(signature: ByteString): Uint8Array {
  const seq = signature.getASN1(Tag.SEQUENCE);
  const r = trimASN1Int(seq.getASN1(Tag.INTEGER));
  const s = trimASN1Int(seq.getASN1(Tag.INTEGER));

  let ret = new Uint8Array(new ArrayBuffer(64));
  ret.set(r.data, 32 - r.length);
  ret.set(s.data, 64 - s.length);
  return ret;
}

function coseKeyToEccPublicKey(coseKey: any): ByteString | null {
  const KEY_TYPE = 1;
  const EC2 = 2;
  const ALGORITHM = 3;
  const ES256 = -7;
  const CURVE = -1;
  const P256 = 1;
  const X = -2;
  const Y = -3;

  if (!(KEY_TYPE in coseKey) || !(ALGORITHM in coseKey)) {
    return null;
  }

  if (coseKey[KEY_TYPE] == EC2 && coseKey[ALGORITHM] == ES256 &&
    CURVE in coseKey && X in coseKey && Y in coseKey &&
    coseKey[CURVE] == P256) {
    const x = coseKey[X];
    const y = coseKey[Y];
    if (!(x instanceof Uint8Array) || x.length != 32 ||
      !(y instanceof Uint8Array) || y.length != 32) {
      return null;
    }

    let ret = new Uint8Array(new ArrayBuffer(65));
    ret[0] = 0x04;
    ret.set(x, 1);
    ret.set(y, 33);
    return new ByteString(ret);
  }

  return null;
}

function coseKeyToRsaPublicKey(coseKey: any): ByteString | null {
  const KEY_TYPE = 1;
  const RSA = 3;
  const ALGORITHM = 3;
  const RS256 = -257;
  const N = -1;
  const E = -2;

  if (!(KEY_TYPE in coseKey) || !(ALGORITHM in coseKey)) {
    return null;
  }

  if (coseKey[KEY_TYPE] !== RSA || coseKey[ALGORITHM] !== RS256 ||
    !(N in coseKey) || !(E in coseKey)) {
    return null;
  }

  const n = coseKey[N];
  const e = coseKey[E];
  if (!(n instanceof Uint8Array) || !(e instanceof Uint8Array)) {
    return null;
  }

  let b = new ByteBuilder();
  b.addASN1(Tag.SEQUENCE, (b) => {
    b.addASN1BigInt(n);
    b.addASN1BigInt(e);
  });

  return new ByteString(b.data);
}

function eccKeyToSPKI(x962: ByteString): Uint8Array {
  let b = new ByteBuilder();
  b.addASN1(Tag.SEQUENCE, (b) => {
    b.addASN1(Tag.SEQUENCE, (b) => {
      b.addASN1ObjectIdentifier([1, 2, 840, 10045, 2, 1]);     // ecPublicKey
      b.addASN1ObjectIdentifier([1, 2, 840, 10045, 3, 1, 7]);  // P-256
    });
    b.addASN1BitString(x962.data);
  });
  return b.data;
}

function rsaKeyToSPKI(rsa: ByteString): Uint8Array {
  let b = new ByteBuilder();
  b.addASN1(Tag.SEQUENCE, (b) => {
    b.addASN1(Tag.SEQUENCE, (b) => {
      b.addASN1ObjectIdentifier([1, 2, 840, 113549, 1, 1, 1]);     // rsaEncryption
      b.addASN1Null();
    });
    b.addASN1BitString(rsa.data);
  });
  return b.data;
}

function getSerialNumber(derCert: ByteString): ByteString {
  derCert = derCert.getASN1(Tag.SEQUENCE).getASN1(Tag.SEQUENCE);
  const version = derCert.getOptionalASN1(
    Tag.CONSTRUCTED | Tag.CONTEXT_SPECIFIC | 0);  // version
  const sn = derCert.getASN1(Tag.INTEGER);        // serialNumber
  return sn;
}

const rpid = (function () {
  let rpid = document.location.host;
  const colonIndex = rpid.indexOf(':');
  if (colonIndex != -1) {
    rpid = rpid.substr(0, colonIndex);
  }
  return rpid;
})();

interface WebauthnRegistrationConfig {
  attestation?: 'none' | 'direct' | 'enterprise';
  authenticatorAttachment?: 'cross-platform' | 'platform';
  challengeLength?: number;
  exclude?: Uint8Array;
  uv?: 'required' | 'preferred' | 'discouraged';
  user?: PublicKeyCredentialUserEntity;
}

interface WebauthnCredential {
  credentialIDB64: string;
  publicKey: {
    dataB64: string,
    keyType: 'rsa' | 'ecc';
  };
  attachment?: string;
  aaguidB64?: string;
  transports?: AuthenticatorTransport[];
  uv?: boolean;
  multiCredBits?: number;
  attestationFormat?: string;
  attestationStatement?: object;
}

async function webauthnRegistration(config: WebauthnRegistrationConfig):
  Promise<WebauthnCredential> {
  if (!navigator.credentials) {
    throw 'No credential manager support in this browser';
  }

  const challenge = randomChallenge(config.challengeLength || 32);
  const challengeB64 = (new ByteString(challenge)).webSafeBase64();

  let user: PublicKeyCredentialUserEntity = {
    id: new Uint8Array([0, 1, 2, 3, 4, 5, 6, 7]),
    name: 'test user',
    displayName: 'test user',
  };
  if (config.user) {
    user = config.user;
  }

  let makePublicKey: PublicKeyCredentialCreationOptions = {
    rp: {
      id: rpid,
      name: rpid,
    },
    user,
    challenge,
    pubKeyCredParams: [
      { type: 'public-key', alg: -7 },
      { type: 'public-key', alg: -257 }
    ],
    attestation: config.attestation || 'none',
    timeout: 60000,
    authenticatorSelection: {
      userVerification: config.uv || 'discouraged',
      authenticatorAttachment: config.authenticatorAttachment,
    },
  };

  if (config.exclude) {
    makePublicKey.excludeCredentials =
      [{ type: 'public-key', id: config.exclude }];
  }
  console.log(makePublicKey);

  const genericCred =
    await navigator.credentials.create({ publicKey: makePublicKey });
  console.log(genericCred);

  const cred = genericCred as PublicKeyCredential;
  const extensionResults = cred.getClientExtensionResults();
  const response = <AuthenticatorAttestationResponse>cred.response;
  const cbor = new ByteString(new Uint8Array(response.attestationObject));
  const header = cbor.getCBOR();
  const authData = new ByteString(header['authData'] as Uint8Array);

  const signedData = new ByteBuilder();
  signedData.addBytes(authData.data);
  const clientDataJSONHash = await window.crypto.subtle.digest(
    { name: 'SHA-256' }, response.clientDataJSON);
  signedData.addBytes(new Uint8Array(clientDataJSONHash));

  const multiCredBits = (authData.data[32] & 0b00011000) >> 3;
  const rpIDHash = authData.getBytes(32);
  const flags = authData.getU8();
  const signatureCounter = authData.getU32();
  const aaguid = new ByteString(authData.getBytes(16));
  const credentialIDLength = authData.getU16();
  const credentialID = authData.getBytes(credentialIDLength);
  const coseKey = authData.getCBOR();
  let rawPublicKey: ByteString | null;
  let spkiPublicKey: Uint8Array;
  let keyType: "rsa" | "ecc";
  rawPublicKey = coseKeyToEccPublicKey(coseKey);
  if (rawPublicKey == null) {
    rawPublicKey = coseKeyToRsaPublicKey(coseKey);
    if (rawPublicKey == null) {
      throw 'Failed to parse public key';
    }
    spkiPublicKey = rsaKeyToSPKI(rawPublicKey);
    keyType = "rsa";
  } else {
    spkiPublicKey = eccKeyToSPKI(rawPublicKey);
    keyType = "ecc";
  }
  const attachment = (cred as any).authenticatorAttachment;
  const uv = (flags & 4) == 4;

  const attestationFormat = header['fmt'];
  const attestationStatement = header['attStmt'];

  if (config.attestation && config.attestation !== "none") {
    if (header['fmt'] === "none") {
      throw 'Attestation is expected but not provided';
    } else if (header['fmt'] !== "fido-u2f") {
      throw 'Can\'t parse attestation format ' + header['fmt'];
    }
    const cert = new ByteString(attestationStatement["x5c"][0]);
    const sn = getSerialNumber(cert);
    console.log("sn: ", sn.slice);
  } else if (config.attestation && config.attestation === "none") {
    if (header['fmt'] !== "none") {
      throw 'Attestation is not expected but provided';
    }
  }

  if (config.authenticatorAttachment && attachment !== config.authenticatorAttachment) {
    throw 'Reported attachment is not ' + config.authenticatorAttachment + ' but ' + attachment;
  }

  if (config.uv === "required" && !uv) {
    throw 'UV is required but not provided';
  }

  return {
    credentialIDB64: (new ByteString(credentialID)).webSafeBase64(),
    publicKey: {
      dataB64: (new ByteString(spkiPublicKey)).webSafeBase64(),
      keyType,
    },
    attachment,
    aaguidB64: aaguid.webSafeBase64(),
    transports: (response as any).getTransports(),
    uv,
    multiCredBits,
    attestationFormat,
    attestationStatement,
  };
}

(window as any).testApp.register = webauthnRegistration;

interface WebauthnAssertionConfig {
  keys?: Array<WebauthnCredential>;
  challengeLength?: number;
  uv?: 'discouraged' | 'preferred' | 'required';
}

interface WebauthnAssertion {
  credentialIDB64: string,
  attachment: string,
  index: number;
  up: boolean;
  uv: boolean;
  userID: ArrayBuffer | null;
  multiCredBits: number;
}

async function webauthnAssertion(config: WebauthnAssertionConfig):
  Promise<WebauthnAssertion> {
  if (!navigator.credentials) {
    throw 'No credential manager support in this browser';
  }

  const challenge = randomChallenge(config.challengeLength || 32);

  let extensions: any = {};

  const getPublicKey: PublicKeyCredentialRequestOptions = {
    challenge,
    timeout: 60000,
    rpId: rpid,
    extensions,
    userVerification: config.uv || 'discouraged',
  };

  if (config.keys) {
    getPublicKey.allowCredentials = [];
    for (const key of config.keys) {
      let entry: PublicKeyCredentialDescriptor = {
        type: 'public-key',
        id: decodeWebSafeBase64(key.credentialIDB64).slice
      };
      if (key.transports) {
        entry.transports = key.transports;
      }
      getPublicKey.allowCredentials.push(entry);
    }
  }

  const genericCred =
    await navigator.credentials.get({ publicKey: getPublicKey });
  const cred = genericCred as PublicKeyCredential;

  const credID = new ByteString(new Uint8Array(cred.rawId));
  let index = -1;

  if (config.keys) {
    for (let i = 0; i < config.keys.length; i++) {
      if (decodeWebSafeBase64(config.keys[i].credentialIDB64).compare(credID) == 0) {
        index = i;
      }
    }

    if (index == -1) {
      throw 'Returned credential ID not expected';
    }
  }

  const response = <AuthenticatorAssertionResponse>cred.response;
  const clientDataJSON =
    new ByteString(new Uint8Array(response.clientDataJSON));
  const clientDataJSONHash = await window.crypto.subtle.digest(
    { name: 'SHA-256' }, clientDataJSON.data);
  const authData = new ByteString(new Uint8Array(response.authenticatorData));
  const multiCredBits = (authData.data[32] & 0b00011000) >> 3;
  const authDataHash =
    await window.crypto.subtle.digest({ name: 'SHA-256' }, authData.data);

  let b = new ByteBuilder();
  b.addBytes(authData.data);
  b.addBytes(new Uint8Array(clientDataJSONHash));

  if (config.keys && index != -1) {
    let importConfig, verifyConfig, sigWebCryptoFormat;
    const keyType = config.keys[index].publicKey.keyType || 'ecc';
    if (keyType == 'ecc') {
      importConfig = { name: 'ECDSA', namedCurve: 'P-256' };
      verifyConfig = { name: 'ECDSA', hash: { name: 'SHA-256' } };
      sigWebCryptoFormat = ecdsaSignatureToWebCryptoFormat(
        new ByteString(new Uint8Array(response.signature)));
    } else if (keyType == 'rsa') {
      importConfig = { name: 'RSASSA-PKCS1-v1_5', hash: { name: 'SHA-256' } };
      verifyConfig = { name: 'RSASSA-PKCS1-v1_5', hash: { name: 'SHA-256' } };
      sigWebCryptoFormat =
        new Uint8Array(response.signature);
    } else {
      throw 'Key type ' + keyType + ' not supported';
    }
    const data = decodeWebSafeBase64(config.keys[index].publicKey.dataB64).slice;
    const key = await window.crypto.subtle.importKey(
      'spki', data, importConfig, false, ['verify']);
    const sigOk = await window.crypto.subtle.verify(
      verifyConfig, key, sigWebCryptoFormat, b.data);
    if (!sigOk) {
      throw 'Signature failure';
    }
  }

  const rpIDHash = authData.getBytes(32);
  const extensionResults = cred.getClientExtensionResults();
  const calculatedRPIDHash =
    new ByteString(new Uint8Array(await window.crypto.subtle.digest(
      { name: 'SHA-256' },
      new TextEncoder().encode(rpid))));
  if (new ByteString(rpIDHash).compare(calculatedRPIDHash) != 0) {
    throw 'Mismatched RP ID';
  }

  const flags = authData.getU8();
  const sigCounter = authData.getU32();
  const info = clientDataJSON.parseUTF8();

  const up = (flags & 1) == 1;
  const uv = (flags & 4) == 4;

  if (!up) {
    throw 'UP is required but not provided';
  }

  if (config.uv === "required" && !uv) {
    throw 'UV is required but not provided';
  }

  return {
    credentialIDB64: credID.webSafeBase64(),
    attachment: (cred as any).authenticatorAttachment,
    index,
    up,
    uv,
    userID: response.userHandle,
    multiCredBits,
  };
}

(window as any).testApp.assert = webauthnAssertion;
