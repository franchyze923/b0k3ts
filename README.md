# B0K3TS

**B0K3TS** is a lightweight UI-first service for managing **S3-compatible buckets** and **Rook/Ceph ObjectBucketClaims (OBCs)**—built because the Rook/Ceph toolbox doesn’t ship with a friendly bucket management UI.

It lets you:

- Connect to **S3-compatible** endpoints (Ceph RGW, MinIO, etc.)
- List / upload / download / delete objects
- Connect to a Kubernetes cluster (in-cluster or via uploaded kubeconfig)
- Create (“apply”), list, and delete **ObjectBucketClaims**
- Fetch bucket credentials from the **Secret** associated with an OBC-created bucket

---

## Features

### S3 (Buckets & Objects)
- Store multiple bucket connections
- List saved connections (with simple authorization checks)
- Upload objects (multipart)
- Download objects
- Delete objects
- List objects in a bucket

### Kubernetes (Rook/Ceph OBCs)
- Upload and store named kubeconfigs (server-side)
- List OBCs in a namespace
- Server-Side Apply an OBC manifest (YAML)
- Delete an OBC
- Extract S3 credentials from the bucket Secret (common OBC pattern)

---

## API Overview

- Base path: `/api/v1`
- Content types:
  - JSON for most requests
  - `multipart/form-data` for file uploads
  - `application/yaml` for Kubernetes “apply” manifests
- Auth:
  - Many endpoints expect an `Authorization` header (token-based), depending on your auth configuration.

---

## Health

### `GET /api/v1/healthz`
```
bash
curl -i "http://<host>:<port>/api/v1/healthz"
```
---

## Auth APIs

### OIDC
#### `GET /api/v1/oidc/login`
Starts the OIDC login flow.
```
bash
curl -i "http://<host>:<port>/api/v1/oidc/login"
```
#### `GET /api/v1/oidc/callback`
OIDC callback endpoint (typically hit by the IdP redirect).

#### `POST /api/v1/oidc/authenticate`
Exchanges/validates auth and returns a session/token (implementation-dependent).
```
bash
curl -X POST "http://<host>:<port>/api/v1/oidc/authenticate" \
-H "Content-Type: application/json" \
-d '{}'
```
#### `GET /api/v1/oidc/config`
Fetch current OIDC configuration.
```
bash
curl "http://<host>:<port>/api/v1/oidc/config"
```
#### `POST /api/v1/oidc/configure`
Configure OIDC.
```
bash
curl -X POST "http://<host>:<port>/api/v1/oidc/configure" \
-H "Content-Type: application/json" \
-d '{
"issuer": "<oidc-issuer-url>",
"client_id": "<client-id>",
"client_secret": "<client-secret-placeholder>",
"redirect_url": "http://<host>:<port>/api/v1/oidc/callback"
}'
```
### Local Auth
#### `POST /api/v1/local/login`
```
bash
curl -X POST "http://<host>:<port>/api/v1/local/login" \
-H "Content-Type: application/json" \
-d '{
"username": "<username>",
"password": "<password-placeholder>"
}'
```
#### `POST /api/v1/local/login_redirect`
Used by the UI to complete a redirect-style login flow.

#### `POST /api/v1/local/authenticate`
Validates local auth and returns a session/token (implementation-dependent).

> Tip: Once authenticated, pass the token via:
>
> `-H "Authorization: Bearer <token-placeholder>"`

---

## Bucket Connection APIs (S3)

### `POST /api/v1/buckets/add_connection`
Adds (stores) a bucket connection configuration.
```
bash
curl -X POST "http://<host>:<port>/api/v1/buckets/add_connection" \
-H "Content-Type: application/json" \
-H "Authorization: Bearer <token-placeholder>" \
-d '{
"bucket_id": "dev-ceph",
"endpoint": "<s3-endpoint-host:port>",
"access_key_id": "<access-key-id-placeholder>",
"secret_access_key": "<secret-access-key-placeholder>",
"secure": false,
"bucket_name": "<bucket-name>",
"location": "<region-or-location>",
"authorized_users": ["user@example.com"],
"authorized_groups": ["my-team"]
}'
```
### `GET /api/v1/buckets/list_connections`
Lists saved connections the current user is authorized to see.
```
bash
curl "http://<host>:<port>/api/v1/buckets/list_connections" \
-H "Authorization: Bearer <token-placeholder>"
```
### `POST /api/v1/buckets/delete_connection`
Deletes a stored connection by `bucket_id`.
```
bash
curl -X POST "http://<host>:<port>/api/v1/buckets/delete_connection" \
-H "Content-Type: application/json" \
-H "Authorization: Bearer <token-placeholder>" \
-d '{
"bucket_id": "dev-ceph"
}'
```
---

## Object APIs (S3)

### `POST /api/v1/objects/upload` (multipart)
Uploads an object to the configured bucket.

- Form fields:
  - `bucket`: bucket identifier used to find stored config
  - `name`: object key (supports “folders” via `/`)
  - `file`: the file payload
```
bash
curl -X POST "http://<host>:<port>/api/v1/objects/upload" \
-H "Authorization: Bearer <token-placeholder>" \
-F "bucket=dev-ceph" \
-F "name=path/to/my-file.bin" \
-F "file=@</path/to/local-file.bin>"
```
### `POST /api/v1/objects/list`
Lists objects in the bucket (recursive listing).
```
bash
curl -X POST "http://<host>:<port>/api/v1/objects/list" \
-H "Content-Type: application/json" \
-H "Authorization: Bearer <token-placeholder>" \
-d '{
"bucket": "dev-ceph",
"prefix": ""
}'
```
### `POST /api/v1/objects/download`
Downloads an object by key.
```
bash
curl -X POST "http://<host>:<port>/api/v1/objects/download" \
-H "Content-Type: application/json" \
-H "Authorization: Bearer <token-placeholder>" \
-d '{
"bucket": "dev-ceph",
"filename": "path/to/my-file.bin"
}' \
--output my-file.bin
```
### `POST /api/v1/objects/delete`
Deletes an object by key.
```
bash
curl -X POST "http://<host>:<port>/api/v1/objects/delete" \
-H "Content-Type: application/json" \
-H "Authorization: Bearer <token-placeholder>" \
-d '{
"bucket": "dev-ceph",
"filename": "path/to/my-file.bin"
}'
```
---

## Kubernetes APIs (ObjectBucketClaims)

These endpoints support **two modes**:

- `mode=incluster` (default-friendly for running inside a cluster)
- `mode=kubeconfig&config=<name>` (use a named kubeconfig uploaded to the server)

### Kubeconfig management

#### `POST /api/v1/kubernetes/kubeconfigs/:name` (multipart or raw)
Upload a named kubeconfig:
```
bash
curl -X POST \
-F "file=@</path/to/kubeconfig.yaml>" \
"http://<host>:<port>/api/v1/kubernetes/kubeconfigs/<name>"
```
#### `GET /api/v1/kubernetes/kubeconfigs`
List stored kubeconfigs:
```
bash
curl "http://<host>:<port>/api/v1/kubernetes/kubeconfigs"
```
#### `DELETE /api/v1/kubernetes/kubeconfigs/:name`
Delete a stored kubeconfig:
```
bash
curl -X DELETE "http://<host>:<port>/api/v1/kubernetes/kubeconfigs/<name>"
```
---

### OBC operations

#### `GET /api/v1/kubernetes/obc/:namespace`
List OBCs (in-cluster):
```
bash
curl "http://<host>:<port>/api/v1/kubernetes/obc/<namespace>?mode=incluster"
```
List OBCs (using uploaded kubeconfig):
```
bash
curl "http://<host>:<port>/api/v1/kubernetes/obc/<namespace>?mode=kubeconfig&config=<name>"
```
#### `POST /api/v1/kubernetes/obc/:namespace/apply`
Apply an OBC (Server-Side Apply). Send raw YAML:
```
bash
curl -X POST \
"http://<host>:<port>/api/v1/kubernetes/obc/<namespace>/apply?mode=kubeconfig&config=<name>" \
-H "Content-Type: application/yaml" \
--data-binary @</path/to/obc.yaml>
```
#### `DELETE /api/v1/kubernetes/obc/:namespace/:name`
Delete an OBC:
```
bash
curl -X DELETE \
"http://<host>:<port>/api/v1/kubernetes/obc/<namespace>/<obc-name>?mode=kubeconfig&config=<name>"
```
#### `GET /api/v1/kubernetes/obc/:namespace/:bucket/secret`
Extract S3 credentials from the Secret (often the Secret name matches the bucket name):
```
bash
curl \
"http://<host>:<port>/api/v1/kubernetes/obc/<namespace>/<bucket-name>/secret?mode=kubeconfig&config=<name>"
```
---

## Notes / Gotchas

- **Do not commit real credentials** to Git. Use placeholders in examples and supply secrets via your runtime configuration.
- If you’re running the server **outside** the cluster, use the **kubeconfig** mode.
- For object upload, the `name` field is the **object key**, so `folder1/file.txt` is perfectly valid.

---

## License

