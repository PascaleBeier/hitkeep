# HitKeep Helm Chart

This chart deploys the HitKeep single-binary service on Kubernetes.

## Kubernetes Guide

### Rationale

HitKeep follows the standard Helm pattern used by major charts: the chart emits a Service and (optionally) an Ingress, while the reverse proxy/ingress controller is provided by the cluster. This keeps the chart simple, avoids bundling a proxy, and fits most Kubernetes setups.

### Install (OCI via GHCR)

```
helm install hitkeep oci://ghcr.io/pascalebeier/charts/hitkeep --version 2.8.1
```

### Minimal values

```
image:
  repository: ghcr.io/pascalebeier/hitkeep
  tag: "2.8.1"

env:
  HITKEEP_PUBLIC_URL: "https://analytics.example.com"

extraEnv:
  - name: HITKEEP_JWT_SECRET
    valueFrom:
      secretKeyRef:
        name: hitkeep-secrets
        key: jwt-secret
```

### Core configuration

- `domain`: convenience host for basic ingress (used when `ingress.hosts` is empty)
- `image.tag`: defaults to the chart app version, without a leading `v`
- `service.type`: `ClusterIP` (default), `LoadBalancer`, or `NodePort`
- `ingress.enabled`: creates an Ingress resource for your cluster's controller
- `ingress.className`: leave empty to use the cluster default
- `ingress.annotations`: controller-specific settings (cert-manager, auth, etc.)
- `env.HITKEEP_PUBLIC_URL`: set to the browser-visible URL, including any path prefix
- `extraEnv`: use this for secret-backed values such as `HITKEEP_JWT_SECRET`
- `persistence.*`: PVC settings for `/var/lib/hitkeep/data`
- Probes: liveness uses `/healthz`, readiness uses `/readyz`

By default, the chart stores the shared DuckDB database, tenant DuckDB databases, retention archives, QR Code graphic assets, and optional spam-list cache below the persistent mount:

- `HITKEEP_DB_PATH=/var/lib/hitkeep/data/hitkeep.db`
- `HITKEEP_DATA_PATH=/var/lib/hitkeep/data`
- `HITKEEP_ARCHIVE_PATH=/var/lib/hitkeep/data/archive`
- `HITKEEP_SPAM_FILTER_PATH=/var/lib/hitkeep/data/spam-filter.json`

Override those keys in `env` only when you also change the matching storage layout.

### Scaling & clustering

Set `replicaCount` to 2+ to enable clustering. The chart uses a StatefulSet with stable pod names and configures gossip discovery via a headless service.

```
replicaCount: 3
```

When clustered, the chart sets `HITKEEP_BIND_ADDR` to the pod IP and joins via the headless service.

### Ingress (standard cluster setup)

```
domain: analytics.example.com

ingress:
  enabled: true
  className: "" # use cluster default
```

### Advanced Ingress (multiple hosts/paths)

```
ingress:
  enabled: true
  hosts:
    - host: analytics.example.com
      paths:
        - path: /
          pathType: Prefix
```

### LoadBalancer (no ingress controller)

```
service:
  type: LoadBalancer

ingress:
  enabled: false
```

### TLS with cert-manager (Let’s Encrypt)

This assumes cert-manager is installed and a ClusterIssuer exists.

```
ingress:
  enabled: true
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
  tls:
    - hosts:
        - analytics.example.com
      secretName: hitkeep-tls
```

### Persistence

The chart mounts a PVC at `/var/lib/hitkeep/data` by default.

```
persistence:
  enabled: true
  size: 10Gi
  accessMode: ReadWriteOnce
```

### Optional MCP and AI configuration

HitKeep 2.8.1 can expose the read-only MCP endpoint and optional AI-backed product features when you configure them explicitly.

```
env:
  HITKEEP_MCP_ENABLED: "true"
  HITKEEP_MCP_PATH: "/mcp"
  HITKEEP_MCP_MAX_RANGE_DAYS: "366"
  HITKEEP_AI_ENABLED: "true"
  HITKEEP_AI_PROVIDER: "openai-compatible"
  HITKEEP_AI_MODEL: "gpt-4.1-mini"

extraEnv:
  - name: HITKEEP_AI_API_KEY
    valueFrom:
      secretKeyRef:
        name: hitkeep-ai
        key: api-key
```

### Optional backups and spam-list refresh

Local backups are disabled until `HITKEEP_BACKUP_PATH` is set. Keep local backup output on persistent storage or use an `s3://` destination with the S3 environment variables.

```
env:
  HITKEEP_BACKUP_PATH: "/var/lib/hitkeep/data/backups"
  HITKEEP_BACKUP_INTERVAL: "60"
  HITKEEP_BACKUP_RETENTION: "24"
  HITKEEP_SPAM_FILTER_AUTO_UPDATE: "true"
  HITKEEP_SPAM_FILTER_UPDATE_INTERVAL: "1440"
```
