ICIP-01
======

Basic Protocol 
-----

`draft` `mandatory`

This ICIP extends [NIP-01](https://github.com/nostr-protocol/nips/blob/master/01.md) with additional kinds and tags or with modifications/extensions to the existing [NIP-01](https://github.com/nostr-protocol/nips/blob/master/01.md) kinds/tags.

### Modifiable Note
A new kind `30175` addressable event that is a modifiable version of the kind `1` note is added.

Kind `30175` is a super set of kind `1`, meaning it has all the features of a kind `1` event, with the extra feature of being modifiable, so whenever a kind `1` is used, the new `30175` kind can be used as well.

The kind `30175` events also need 2 timestamp fields to be added:
1. `published_at`, mandatory, same as for [`30023` articles](https://github.com/nostr-protocol/nips/blob/master/23.md)
2. `editing_ended_at`, optional, if set Clients and Relays MUST reject updates made to the event after that timestamp

`d`, `published_at`, `editing_ended_at` tags MUST never be changed. Those are immutable, the first version of the event has the final versions of those tags.

###### Example
```json
{
  "kind": 30175,
  "created_at": 1675642635,
  "content": "Lorem [ipsum][nostr:nevent1qqst8cujky046negxgwwm5ynqwn53t8aqjr6afd8g59nfqwxpdhylpcpzamhxue69uhhyetvv9ujuetcv9khqmr99e3k7mg8arnc9] dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.\n\nRead more at nostr:naddr1qqzkjurnw4ksz9thwden5te0wfjkccte9ehx7um5wghx7un8qgs2d90kkcq3nk2jry62dyf50k0h36rhpdtd594my40w9pkal876jxgrqsqqqa28pccpzu.",
  "tags": [
    ["d", <UUIDv7>],
    ["published_at", "1296962229"],
    ["editing_ended_at", "1297962229"],
    ["e", "b3e392b11f5d4f28321cedd09303a748acfd0487aea5a7450b3481c60b6e4f87", "wss://relay.example.com"],
    ["a", "30023:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:ipsum", "wss://relay.nostr.org"]
  ],
  "pubkey": "...",
  "id": "..."
}
```
### Quotes

When quoting a kind `30175` modifiable note or a kind `30023` long-form note, a new `Q` tag MUST be used, which is a replica of the exiting [`q` tag](https://github.com/nostr-protocol/nips/blob/master/18.md#quote-reposts), but designed for addressable events instead
```json
["Q", "30175:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:01946ef2-e9ec-7524-8e80-7cd58f0bab14", <relay-url>, <pubkey>]
```
For reposting, a [kind `16`](https://github.com/nostr-protocol/nips/blob/master/18.md#generic-reposts) MUST still be used

### Replies

When creating a comment/reply to a kind `30175` modifiable note or a kind `30023` long-form note, a modified `a` tag must be used, adapted from the existing [`e` tag](https://github.com/nostr-protocol/nips/blob/master/10.md#marked-e-tags-preferred)

```json
["a", "30175:a695f6b60119d9521934a691347d9f78e8770b56da16bb255ee286ddf9fda919:01946ef2-e9ec-7524-8e80-7cd58f0bab14", <relay-url>, <marker>, <pubkey>]
```

### Extended profile

These are the extra fields not specified in [NIP-01](https://github.com/nostr-protocol/nips/blob/master/01.md) or [NIP-24](https://github.com/nostr-protocol/nips/blob/master/24.md) that may be present in the stringified JSON of metadata(kind 0) events:

* `location`
  * Users can input their geolocation here, freely. 
    * Relays and Clients MUST not validate this information
* `category`
  * Users can input a category for their profile that can define their main area of expertise or interest. 
    * Clients SHOULD make users select from a predefined list of categories, to standardize user interactions later on. Relays MUST not validate this information.
* `wallets`
  * A map of network<->address
    * Clients SHOULD validate this information. Relays MUST not validate this information. 
    * example:
    ```json
        {"wallets":{
          "ethereum": "0xaAAa85E5e95231af428d8e54b4B2916DE7283A41",
          "bitcoin": "15B94j49LNYjMdxamEfkpPrDeydzexmm8k"
        }}
    ```
* `who_can_message_you` _-- if this is not set then everyone can message you_ --
  * Clients SHOULD validate this information. Relays MUST not validate this information.
  * possible values:
    * `follows` _-- people you follow --_
    * `friends` _-- people you follow that follow you back --_
* `who_can_invite_you_to_groups` _-- if this is not set then everyone can message you_ --
  * Clients SHOULD validate this information. Relays MUST not validate this information.
  * possible values:
    * `follows` _-- people you follow --_ 
    * `friends` _-- people you follow that follow you back --_