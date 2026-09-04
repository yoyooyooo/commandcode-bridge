# Mini canary

The previous OpenAI bridge remains on `127.0.0.1:8787` until cutover is
explicitly approved. This repository's Proxy canary listens on
`127.0.0.1:8788`.

Runtime files live outside git:

```text
~/.config/commandcode-bridge/canary/
~/.local/bin/commandcode-bridge-canary
~/Library/LaunchAgents/com.yoyo.commandcode-bridge-canary.plist
```

Do not replace account 1117 or group 32 during canary. Create an isolated
Sub2API account/group only after host and container health, client auth
positive/negative tests, and a live Command Code smoke succeed.

Rollback: unload the canary LaunchAgent. Leave `com.yoyo.commandcode-openai-bridge`
running.
