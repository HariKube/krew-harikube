# krew-harikube

This Krew plugin adds a thin `kubectl harikube` alias layer over native
`kubectl` verbs, so you can use standard kubectl-style commands instead of
making raw endpoint calls yourself.

## Supported verbs

- `get` → `kubectl get`
- `list` → `kubectl get`
- `watch` → `kubectl watch`
- `create` → `kubectl create`
- `update` → `kubectl apply`
- `delete` → `kubectl delete`

## Usage

```bash
kubectl harikube get pods
kubectl harikube list deployments
kubectl harikube watch pods -n default
kubectl harikube create -f resource.yaml
kubectl harikube update -f resource.yaml
kubectl harikube delete pod example
```

## Build

```bash
go build -o kubectl-harikube .
```
