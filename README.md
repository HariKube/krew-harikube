# krew-harikube

This plugin wraps HariKube aggregation API endpoints behind
`kubectl harikube <verb> ...`, so you do not have to build raw `/apis/...`
paths by hand.

## Supported verbs

- `get` → `GET /apis/<group>/<version>/...`
- `list` → `GET /apis/<group>/<version>/...`
- `watch` → `GET /apis/<group>/<version>/...?watch=true`
- `create` → `POST /apis/<group>/<version>/...`
- `update` → `PUT /apis/<group>/<version>/...`
- `delete` → `DELETE /apis/<group>/<version>/...`

Resource names are resolved from aggregation API discovery, including:

- full resource names
- singular names
- short names
- qualified forms like `widgets.example.com` and `widgets.v1.example.com`

## Usage

```bash
kubectl harikube get widgets.example.com sample -n demo
kubectl harikube list widgets -n demo
kubectl harikube watch wdg -n demo
kubectl harikube create -f widget.yaml
kubectl harikube update -f widget.yaml
kubectl harikube delete widgets sample -n demo
```

Supported flags:

- `-n`, `--namespace`
- `-A`, `--all-namespaces` (cluster-scoped resources only)
- `-l`, `--selector`
- `--field-selector`
- `-f`, `--filename`
- `-o`, `--output` (`json` or `yaml`)

## Install

### Download a release with curl

Linux `amd64` example:

```bash
VERSION=v0.1.0
curl -fsSL -o kubectl-harikube.tar.gz \
  "https://github.com/HariKube/krew-harikube/releases/download/${VERSION}/kubectl-harikube_linux_amd64.tar.gz"
tar -xzf kubectl-harikube.tar.gz
install -m 0755 ./kubectl-harikube ~/.local/bin/kubectl-harikube
```

Release assets are published for:

- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`
- `windows/amd64`
- `windows/arm64`

Each release also includes a `checksums.txt` file for verification.

```bash
curl -fsSL -o checksums.txt \
  "https://github.com/HariKube/krew-harikube/releases/download/${VERSION}/checksums.txt"
awk '$2 == "kubectl-harikube_linux_amd64.tar.gz" { print }' checksums.txt | sha256sum -c -
```

### Build locally

Build the plugin binary and place it somewhere on your `PATH` as
`kubectl-harikube`:

```bash
go build -o kubectl-harikube .
install -m 0755 ./kubectl-harikube ~/.local/bin/kubectl-harikube
```

## Build

```bash
go build -o kubectl-harikube .
```
