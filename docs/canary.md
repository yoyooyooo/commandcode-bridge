# Mini canary

The previous OpenAI bridge remains on `127.0.0.1:8787` until cutover is
explicitly approved. This repository's Proxy canary listens on
`127.0.0.1:8788`.

## Host

```text
binary        ~/.local/bin/commandcode-bridge-canary
LaunchAgent   com.yoyo.commandcode-bridge-canary
listen        127.0.0.1:8788
container URL http://host.docker.internal:8788/v1
secrets       ~/.config/commandcode-bridge/canary/  (owner-only, not git)
```

Proxy Client Keys currently named `canary-scripts` and `sub2api-canary`.
The Command Code upstream key is read from Pi `models.json` via an owner-only
command; it is not stored in Sub2API.

## Isolated Sub2API path

Do not add this account to group 31 or 32. Do not change account 1117.

```text
group     id=35  name=commandcode-bridge-canary  platform=openai
account   id=1118 name=commandcode-bridge-canary  priority=90  groups=[35]
client    id=46  name=pi-commandcode-bridge-canary  group=35
base URL  http://host.docker.internal:8788/v1
protocol  extra.openai_responses_mode=force_chat_completions
models    deepseek-v4-flash / deepseek-v4-pro and slash aliases
mapping   deepseek-v4-flash → deepseek/deepseek-v4-flash
```

Verified 2026-09-04 on mini:

- host `/healthz` and Sub2API container `/healthz` on 8788: 200
- group key `/v1/models`: 401 without key, 401 wrong key, 200 with key 46; four canary models only
- non-stream and stream `/v1/chat/completions` `deepseek-v4-flash`: 200; stream and buffered both returned `PONG` when `max_completion_tokens>=64`
- Sub2API usage rows `431468`/`431469`/`431470`: `account_id=1118`, `api_key_id=46`, `group_id=35`, `upstream_endpoint=/v1/chat/completions`
- `/v1/responses` inbound was converted to `/v1/chat/completions` by Sub2API
- account 1117 still `group_ids=[31,32]`, base `http://host.docker.internal:8787/v1`, priority 20
- group 32 `account_count` remained 3

Rollback: unload `com.yoyo.commandcode-bridge-canary` and disable account 1118.
Leave `com.yoyo.commandcode-openai-bridge` running.
