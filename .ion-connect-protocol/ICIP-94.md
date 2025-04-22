ICIP-94
======

File Metadata
-----

`draft` `mandatory`

This ICIP extends [NIP-94](https://github.com/nostr-protocol/nips/blob/master/94.md) with additional tags/fields or with modifications/extensions to the existing [NIP-94](https://github.com/nostr-protocol/nips/blob/master/94.md) tags/fields.

### New `duration` tag

A new `duration` tag (or tag field if used in an [`imeta`](https://github.com/nostr-protocol/nips/blob/master/92.md) tag) which represents the duration, in seconds, of a video file.

### New `encryption-key` tag

A new `encryption-key` tag (or tag field if used in an [`imeta`](https://github.com/nostr-protocol/nips/blob/master/92.md) tag) which is used to decrypt the contents of the uploaded file and has the following format: 

`["encryption-key", <theActualKey>, <nonce>, <encryptionAlg>]`

#### This new tag MUST be used only inside rumor events, it MUST NOT be used for any other purpose.

###### Example
```json
[
  "imeta",
  "url https://nostr.build/i/my-video.mp4",
  "m video/mp4",
  "blurhash eVF$^OI:${M{o#*0-nNFxakD-?xVM}WEWB%iNKxvR-oetmo#R-aen$",
  "dim 3024x4032",
  "alt A scenic video overlooking the coast of Costa Rica",
  "x <sha256 hash as specified in NIP 94>",
  "fallback https://nostrcheck.me/alt1.mp4",
  "fallback https://void.cat/alt1.mp4",
  "expiration 1600000000",
  "duration 3700",
  "encryption-key 776beff2851db06f4c0226acf3b6c53c23fd616d183bb2221b56a5aed5166693 876beff2851db06f4c022601 aes-gcm"
]
```