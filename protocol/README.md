# Command Code wire contract

This directory is the language-neutral protocol SSoT for Command Code Bridge.

- Proxy (Go) and Pi Direct Adapter (TypeScript) must share these fixtures,
  the static catalog, and the CLI version file.
- The contract is observed, not vendor-published. Treat it as a snapshot.
- Version marker: `command-code-version` (currently `0.52.1`).

## Surfaces

| Path | Shape | Notes |
| --- | --- | --- |
| `POST /alpha/generate` | custom NDJSON | generation path used by the official CLI; not plan-gated on Go |
| `GET /provider/v1/models` | OpenAI-style list | used for dynamic catalog |
| `POST /provider/v1/chat/completions` | OpenAI | plan-gated; Proxy does not use this for generation |
| `POST /v1/responses` on the Proxy | n/a | Proxy returns 404 `unsupported_endpoint` |

## Request envelope

`POST /alpha/generate` requires a schema-strict `config` object plus `params`.
Neutral defaults are acceptable. `params.stream` is `true` even when the Proxy
client asked for a buffered OpenAI response; the Proxy reduces the upstream
event stream.

Messages use Vercel AI SDK `ModelMessage` shape, not Anthropic blocks.

## Event stream

Upstream commonly emits bare NDJSON. The Proxy decoder also accepts SSE
(`data:`), multi-line `data:` fields, and a final frame without a trailing
newline. `[DONE]` is an OpenAI projection token, not a Command Code event.

| Event | Authority |
| --- | --- |
| `reasoning-delta` / `text-delta` | content |
| `tool-input-start` / `tool-input-delta` / `tool-input-end` | provisional telemetry only |
| `tool-call` | only authoritative executable tool call |
| `finish-step` | step boundary; legal completion if the stream then ends |
| `finish` | stream terminal |
| `error` / `abort` | fail-closed |
| EOF without `finish-step` or `finish` | fail-closed |
| malformed JSON | fail-closed |

## Tool calls

- Executable `tool_calls` are emitted only from an authoritative `tool-call`
  event whose `input` (or `args` fallback) is a complete JSON object.
- Parallel calls are keyed by stable `id` / `toolCallId`; later duplicates are
  ignored.
- Scalar, null, array, or malformed arguments fail the stream. They are never
  coerced into an executable call.
- Large integers must survive as decimal JSON, not float64.

## Images

- `data:` URLs with `mediaType` are accepted.
- Remote `http(s)` image URLs are rejected by default.

## Catalog

Dynamic fetch is preferred. An empty live list is a fetch failure, not a valid
catalog. Fallback order: last-known-good, then versioned static JSON in
`catalog/`. Ready state is `degraded` while serving fallback.
