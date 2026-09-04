# Command Code Bridge

Private repository. Do not publish.

```text
Command Code Bridge
├─ OpenAI-compatible Proxy   (scripts, apps, OpenAI clients, Sub2API)
└─ Pi Direct Adapter         (optional; talks to Command Code without the Proxy)
```

The Proxy is the product. "Sidecar" only describes a deployment next to
Sub2API.

## Status

- New Proxy canary target: `127.0.0.1:8788`
- Existing mini service on `127.0.0.1:8787` is not replaced by this repo
- GitHub visibility: private
- Public release: not authorized

## Proxy

```bash
export COMMANDCODE_API_KEY_FILE="$HOME/.config/commandcode-bridge/upstream.key"
export COMMANDCODE_PROXY_CLIENT_KEYS_FILE="$HOME/.config/commandcode-bridge/clients.json"
export COMMANDCODE_PROXY_ADDR=127.0.0.1:8788
go test -race ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/commandcode-bridge ./cmd/commandcode-bridge
./bin/commandcode-bridge
```

Client calls use a Proxy Client Key:

```bash
curl -sS http://127.0.0.1:8788/v1/models \
  -H "Authorization: Bearer <proxy-client-key>"
```

The Proxy reads the Command Code upstream key from its own secret source.
It does not forward the client Bearer to Command Code.

See `docs/proxy.md`, `docs/auth.md`, and `protocol/README.md`.

## Pi Direct Adapter

`adapters/pi` is a MIT-licensed local fork of
`safzanpirani/pi-commandcode-provider` at
`c41f3b2ee2fe226658da886dd36ba3f22cff0a43`. It keeps
`providers.commandcode.apiKey` in Pi's global `models.json` and does not
override that key while registering models.

Using the Proxy from Pi does not require this adapter.

## License

MIT. Third-party notices are in `NOTICE.md`.
