# Auth

Two key authorities:

1. **Proxy Client Key** — issued to scripts, apps, OpenAI clients, and Sub2API
   accounts. Independently revocable. Identified by client name.
2. **Command Code upstream Key** — held only by the Proxy. Never accepted as a
   client credential and never returned in API responses.

## Loading secrets

Upstream key, first non-empty source wins:

- `COMMANDCODE_API_KEY`
- `COMMANDCODE_API_KEY_FILE` (owner-only file)
- `COMMANDCODE_API_KEY_COMMAND`

Proxy client keys:

- `COMMANDCODE_PROXY_CLIENT_KEYS=name:key,other:key`
- `COMMANDCODE_PROXY_CLIENT_KEYS_FILE` JSON `[{"name":"...","key":"..."}]` or
  `name key` lines. The file must be owner-only.

The process refuses to start without at least one client key and one upstream
key. Raw secrets must not be committed.

## Sub2API

A Sub2API `openai/apikey` account should store a Proxy Client Key and point at
the Proxy base URL. It must not store the Command Code upstream key once the
canary path is accepted.
