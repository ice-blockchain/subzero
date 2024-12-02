ICIP-51
======

Lists
-----

`draft` `optional`

This ICIP extends [NIP-51](https://raw.githubusercontent.com/nostr-protocol/nips/refs/heads/master/51.md) with
additional lists and sets.

## Standard lists

| name                         | kind  | description                                                                                                                            | expected tag items                                                                                                                                             |
|------------------------------|-------|----------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| On-Behalf of (Delegation)    | 10100 | [ICIP-2000](ICIP-2000.md) attestations list                                                                                            | `"p"` (pubkeys, with [ICIP-2000](ICIP-2000.md) attestation string)                                                                                             |
| Chats (including E2EE chats) | 11750 | [ICIP-3000](ICIP-3000.md) communities or [NIP-17](https://github.com/nostr-protocol/nips/blob/master/17.md) chats the user is apart of | `"h"` (kind:`31750` community definitions); encrypted `p` and `subject` tags for [NIP-17](https://github.com/nostr-protocol/nips/blob/master/17.md) E2EE chats |

## Sets

| name | kind | description | expected tag items |
|------|------|-------------|--------------------|
|      |      |             |                    |

## Examples

### An attestation _list_ with some delegated pubkeys

```json
  {
  "kind": "10100",
  "tags": [
    ["p", "477318cfb5427b9cfc66a9fa376150c1ddbc62115ae27cef72417eb959691396", "", "active:1674834236:1,7"]
  ],
  "content": "",
  ...
}
```

### A _list_ of all communities an user is in

```json
{
  "kind": 11750,
  "tags": [
    ["h", "01938730-22ea-7829-b705-c61463c8edeb"],
    ["h", "01938730-5ca3-7897-99b2-fb6fbadb6156"],
    ["encrypted"]
  ],
  "content": nip44_encrypt([
      [ "p", "one-on-one receiver pubkey1" ],
      [ "p", "one-on-one receiver pubkey2" ],
      [ "subject", "family & friends", "receiver1 pubkey", "receiver2 pubkey", "receiver3 pubkey"],
      [ "subject", "work", "receiver1 pubkey", "receiver2 pubkey",... "receiver25 pubkey"]
    ]),
  ...
}
````
