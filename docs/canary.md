# Mini deploy

Current live Proxy is `127.0.0.1:8788`. The old `8787` LaunchAgent is unloaded.

```text
binary        ~/.local/bin/commandcode-bridge   (v0.1.0)
LaunchAgent   com.yoyo.commandcode-bridge
listen        127.0.0.1:8788
container URL http://host.docker.internal:8788/v1
secrets       ~/.config/commandcode-bridge/canary/  (owner-only, not git)
GitHub        private Release v0.1.0
```

Sub2API:

```text
account 1117 commandcode-go  priority 1  groups [31, 32]
base     http://host.docker.internal:8788/v1
auth     Proxy Client Key named sub2api-commandcode
```

Account 1118 / group 35 remain inactive leftovers from canary and must not be
rescheduled. Group 32 order is Command Code (1), iMile (10), Freebuff (50).

Pi Direct Adapter path on mini:

```text
~/.pi/agent/settings.json packages -> .../commandcode-bridge/adapters/pi
```

Agent Kit fleet source still points at `agent-extensions/packages/pi-commandcode-provider`
until an AGS PR changes `.ak/runtime.yaml`. Mini local Pi already uses the new path.
