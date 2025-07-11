ICIP-44
======

Encrypted Payloads (Versioned)
------------------------------

`optional`

This ICIP extends [NIP-44](https://github.com/nostr-protocol/nips/blob/master/44.md) with support for payload compression.

### Padded content compression

Before the padded content is encrypted at step [5.](https://github.com/nostr-protocol/nips/blob/master/44.md#encryption) it will be compressed(consider this step 4'.)

The compression algorithm can be anything, although `brotli` or `zlib` are recommended as they provide great compression and are widely available across all operating systems.

This choice will be specified to the clients via a special `payload-compression` tag (I.E. `["payload-compression", "brotli"]`) that will be set in the final published event that uses encrypted payloads (I.E. kind `1059` of [NIP-59](https://github.com/nostr-protocol/nips/blob/master/59.md))