# b0k3ts

# APIs

## Upload a named kubeconfig

```bash
curl -X POST \
  -F "file=@</path/to/kubeconfig.yaml>" \
  "http://<host>:<port>/api/v1/kubernetes/kubeconfigs/<name of config>"
```