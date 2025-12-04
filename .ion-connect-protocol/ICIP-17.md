ICIP-17
======

Private Direct Messages
-----

`draft` `optional`

This ICIP extends [NIP-17](https://github.com/nostr-protocol/nips/blob/master/17.md) with additional kinds and tags or with modifications/extensions to the existing [NIP-17](https://github.com/nostr-protocol/nips/blob/master/17.md) kinds/tags.

### Modifiable Direct Message Kind
A new kind `30014` addressable event that is the modifiable version of the kind `14` event is added. It is essentially a clone of the original, but with the added benefit of being addressable/modifiable based on its `d` tag. Thus, kind `14` and `30014` events are interchangeable and 100% compatible with each other. `30014` being a clone of `14`, this implies that all rules and validation of `14` MUST also apply to `30014`, I.E. it must also never be signed.

###### Example
```json
{
  "id": "<usual hash>",
  "pubkey": "<sender-pubkey>",
  "created_at": "<current-time>",
  "kind": 30014,
  "tags": [
    ["d", "<unique message UUIDv7>"],
    ["p", "<receiver-1-pubkey>", "<relay-url>"],
    ["p", "<receiver-2-pubkey>", "<relay-url>"],
    ["a", "30014:<pubkey of parent message>:<parent kind 30014 d tag>", "<relay-url>"] // if this is a reply
    ["subject", "<conversation-title>"],
    // rest of tags...
  ],
  "content": "<message-in-plain-text>",
}
```

### Blocking/Muting Users

A new kind `1757` immutable event is added, that MUST be used only as a rumor. The gift wrap MUST not have any expiration set.

It is meant to signal that the author of that event has blocked some user by specifying that user in the `p` tag.

###### Example
```json
{
  "pubkey": "<sender-pubkey, the one doing the blocking>",
  "kind": 1757,
  "tags": [
    ["b", "<sender materKey>"],
    ["p", "<receiver-pubkey/masterKey> the one being blocked"]
  ],
  "content": "<optional message-in-plain-text, specifying the reason of the blocking>",
  // rest of the fields ...
}
```

### Archiving Conversations

A new kind `2175` immutable event is added, that MUST be used only as a rumor. The gift wrap MUST not have any expiration set.

It is meant to signal that the author of that event has archived some conversation by specifying that conversation's identifier in the `h` tag.

###### Example
```json
{
  "pubkey": "<sender-pubkey, the one doing the archiving>",
  "kind": 2175,
  "tags": [
    ["b", "<sender materKey>"],
    ["h", "<conversation id that's being archived>"]
  ],
  "content": "<optional message-in-plain-text, specifying the reason of the archiving>",
  // rest of the fields ...
}
```