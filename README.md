# subzero
## Starting
```bash
subzero --port=9998 --cert=./cmd/subzero/.testdata/localhost.crt --key=./cmd/subzero/.testdata/localhost.key --adnl-external-ip=127.0.0.1 --adnl-port=11512 --storage-root=./../.uploads --adnl-node-key=<hex> [--global-config-url=file://path/to/global.json]
```
Parameters:
* port - port to start https/ws server for user iteraction
* cert - tls cert for https / ws server
* key - tls key for https / ws server
* adnl-external-ip - node's external address (needed to serve storage uploads), other nodes connect to <adnl-external-ip>:<adnl-port>
* adnl-port - port to start adnl / storage server
* storage-root - root storage to store files `<storage-root>/<user's key>/files_here.ext`
* adnl-node-key - adnl key for the node in hex form (length: 64 bytes, 128 in hex), i.e `6cc91d96a67bcae7a7a4df91f9c04469f652cf007b33460c60c0649f1777df5703bec10efbd4520126e53d0d70552f873ba843d54352d59fa28989bdf3925a7d` = random if not specified
```go
_, key ,_ := ed25519.GenerateKey(nil)
fmt.Println(hex.EncodeToString(key))
```
* global-config-url - url (supports file:// schema) of global config (to fetch initial DHT nodes for storage), by default = mainnet url

## NIPs
NIPs | latest commit hash implemented | comments
--- | --- | --- 
[01](https://github.com/nostr-protocol/nips/blob/master/01.md) | [9971db3](https://github.com/nostr-protocol/nips/commit/9971db355164815c986251f8f89d1c7c70ec9e53)
[02](https://github.com/nostr-protocol/nips/blob/master/02.md) | [e655247](https://github.com/nostr-protocol/nips/commit/e6552476aa2e5ca7256be572a9aa226ec8a022ee) |
[05](https://github.com/nostr-protocol/nips/blob/master/05.md) | |
[09](https://github.com/nostr-protocol/nips/blob/master/09.md) | [5ae5a6d](https://github.com/nostr-protocol/nips/commit/5ae5a6d0553e34afc3cf19e96043f7e0e2b349ef) |
[10](https://github.com/nostr-protocol/nips/blob/master/10.md) | [c343175](https://github.com/nostr-protocol/nips/commit/c343175a32f492cd1a40749fdd7c523c083bdb19) |
[11](https://github.com/nostr-protocol/nips/blob/master/11.md) | [d67988e](https://github.com/nostr-protocol/nips/commit/d67988e64ee3c0a0df859ad557aa26fb7844f11e) |
[13](https://github.com/nostr-protocol/nips/blob/master/13.md) | [e7eb776](https://github.com/nostr-protocol/nips/commit/e7eb776288b424e8cd43080b33b21506942f91e0) |
[18](https://github.com/nostr-protocol/nips/blob/master/18.md) | [996ef45](https://github.com/nostr-protocol/nips/commit/996ef456057c6f91320411098c259c3b68f3cc77) |
[19](https://github.com/nostr-protocol/nips/blob/master/19.md) | |
[21](https://github.com/nostr-protocol/nips/blob/master/21.md) | |
[23](https://github.com/nostr-protocol/nips/blob/master/23.md) | [ca3c52e](https://github.com/nostr-protocol/nips/commit/ca3c52e3e74f0a4679f1c6c0d9ac6461ea748d2d) |
[24](https://github.com/nostr-protocol/nips/blob/master/24.md) | [e30eb40](https://github.com/nostr-protocol/nips/commit/e30eb40eefbcef319a74a5531473f412987fca6a) |
[25](https://github.com/nostr-protocol/nips/blob/master/25.md) | [e655247](https://github.com/nostr-protocol/nips/commit/e6552476aa2e5ca7256be572a9aa226ec8a022ee) |
[26](https://github.com/nostr-protocol/nips/blob/master/26.md) | |
[27](https://github.com/nostr-protocol/nips/blob/master/27.md) | |
[32](https://github.com/nostr-protocol/nips/blob/master/32.md) | [e655247](https://github.com/nostr-protocol/nips/commit/e6552476aa2e5ca7256be572a9aa226ec8a022ee) |
[40](https://github.com/nostr-protocol/nips/blob/master/40.md) | [5dcfe85](https://github.com/nostr-protocol/nips/commit/5dcfe85306434f21ecb1e7a47edd92b2e3e64f9a) |
[45](https://github.com/nostr-protocol/nips/blob/master/45.md) | [38af1ef](https://github.com/nostr-protocol/nips/commit/38af1efe779f9a26530c6171f292264b28c1eb43) |
[50](https://github.com/nostr-protocol/nips/blob/master/50.md) | [38af1ef](https://github.com/nostr-protocol/nips/commit/38af1efe779f9a26530c6171f292264b28c1eb43) |
[51](https://github.com/nostr-protocol/nips/blob/master/51.md) | [e655247](https://github.com/nostr-protocol/nips/commit/e6552476aa2e5ca7256be572a9aa226ec8a022ee) |
[56](https://github.com/nostr-protocol/nips/blob/master/56.md) | [e655247](https://github.com/nostr-protocol/nips/commit/e6552476aa2e5ca7256be572a9aa226ec8a022ee) |
[58](https://github.com/nostr-protocol/nips/blob/master/58.md) | [e655247](https://github.com/nostr-protocol/nips/commit/e6552476aa2e5ca7256be572a9aa226ec8a022ee) |
[65](https://github.com/nostr-protocol/nips/blob/master/65.md) | |
[90](https://github.com/nostr-protocol/nips/blob/master/90.md) | |
[92](https://github.com/nostr-protocol/nips/blob/master/92.md) | |
[94](https://github.com/nostr-protocol/nips/blob/master/94.md) | |
[96](https://github.com/nostr-protocol/nips/blob/master/96.md) | [4e73e94d417f16fa3451e58ef921cb3b512c6f8e](https://github.com/ice-blockchain/subzero/commit/130bac5adedf6563fe8d8e869f7e46b4cfb414e0)|
[98](https://github.com/nostr-protocol/nips/blob/master/98.md) | [ae0fd96907d0767f07fb54ca1de9f197c600cb27](https://github.com/ice-blockchain/subzero/commit/130bac5adedf6563fe8d8e869f7e46b4cfb414e0)|