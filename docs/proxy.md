# Command Code Proxy

Proxy is a first-class OpenAI-compatible product. Sidecar is only a deployment
shape when it runs next to Sub2API.

## Clients

Scripts, applications, ordinary OpenAI clients, and Sub2API all call:

```text
Authorization: Bearer <Proxy Client Key>
POST http://127.0.0.1:8788/v1/chat/completions
```

The Proxy authenticates that client key, records the client name, converts
OpenAI Chat Completions to Command Code `/alpha/generate`, and sends the
**upstream Command Code key** that only the Proxy holds.

Pi does not have to use this Proxy. The Direct Adapter in `adapters/pi` can
call Command Code with `providers.commandcode.apiKey` from Pi's global
`models.json`. Pi may also treat the Proxy as a normal `openai-completions`
provider.

## Endpoints

| Method | Path | Auth |
| --- | --- | --- |
| GET | `/healthz` | none |
| GET | `/readyz` | none |
| GET | `/v1/models` | Proxy Client Key |
| POST | `/v1/chat/completions` | Proxy Client Key |
| POST | `/v1/responses` | 404 `unsupported_endpoint` |

## Invariants

- Default listen address is loopback. External bind requires
  `COMMANDCODE_PROXY_ALLOW_EXTERNAL=1`.
- Client Bearer and upstream Bearer are never the same secret.
- Passthrough of the client key to Command Code is not implemented.
- Core does not persist raw secrets and does not print them in logs, errors,
  or receipts.
- Only an authoritative `tool-call` event becomes an executable OpenAI
  `tool_calls` item.
- Incomplete streams fail closed and do not emit `data: [DONE]`.
- Empty live model lists are fetch failures. Fallback is last-known-good, then
  the versioned static catalog. `/readyz` is `degraded` on fallback.

Canary listen address on mini is `127.0.0.1:8788`. The previous
`127.0.0.1:8787` service must stay up until cutover is explicitly approved.
