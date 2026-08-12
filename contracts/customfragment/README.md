# CustomFragment contracts

Tolk NFT collection/item contracts used by gramsrv's CustomFragment Web App.
The collection accepts only short-lived Ed25519-signed mint authorizations
issued by gramsrv and binds every authorization to the connected TON wallet.
Items implement the standard NFT getters and transfer messages.

This is a typed Tolk rewrite of the minting subset used by Telegram's GPL-3.0
Telemint contracts. It intentionally uses a new storage layout and therefore is
not address-compatible with the original FunC contracts.

```sh
acton build
acton test
acton wrapper
```
