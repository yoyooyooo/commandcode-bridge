# NOTICE

## Command Code Bridge

Copyright (c) 2026 Jackie / yoyooyooo

Licensed under the MIT License. See `LICENSE`.

The OpenAI-compatible Proxy, protocol fixtures, and conformance tests in this
repository are original work. They independently implement observed Command
Code `/alpha/generate` behavior and OpenAI Chat Completions projection.

## Pi Direct Adapter

`adapters/pi` is a locally maintained fork of:

- Project: https://github.com/safzanpirani/pi-commandcode-provider
- Upstream commit: `c41f3b2ee2fe226658da886dd36ba3f22cff0a43`
- License: MIT
- Copyright (c) 2026 Safzan Pirani

The upstream MIT license text is preserved at `adapters/pi/LICENSE`. Local
changes keep API-key ownership in Pi's global `providers.commandcode.apiKey`
configuration and add a regression test for that boundary.

## Claim limits

Command Code does not publish a stable public API for `/alpha/generate`.
Wire observations are snapshots against a specific CLI version recorded in
`protocol/command-code-version`. They can drift.

This repository does not include source from unlicensed third-party proxies.
