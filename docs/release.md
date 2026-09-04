# Release

The Proxy ships as a versioned binary. GitHub Releases are private because the
repository is private. Do not make the repository public.

## Tag

Tags are `vMAJOR.MINOR.PATCH`. Pushing a matching tag runs
`.github/workflows/release.yml`:

1. `go test -race ./...` and `go vet ./...`
2. Cross-compile `darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64`
3. Write `SHA256SUMS`
4. Create a GitHub Release and attach the binaries

```bash
git tag v0.1.0
git push origin v0.1.0
```

Local equivalent:

```bash
VERSION=v0.1.0 bash scripts/build-release.sh
./dist/commandcode-bridge_v0.1.0_$(uname -s | tr 'A-Z' 'a-z')_$(uname -m | sed 's/x86_64/amd64/') version
```

The binary reports version via `commandcode-bridge version`. Build metadata is
injected with `-ldflags` into `internal/buildinfo`.

## Install on mini

Download the `darwin_arm64` asset from the private Release, then:

```bash
install -m 755 commandcode-bridge_<version>_darwin_arm64 ~/.local/bin/commandcode-bridge
```

Secrets stay in `~/.config/commandcode-bridge/` and are not part of the Release.
