# Agent TLS for backend gRPC

Place the coordinator agent CA certificate here:

```
deploy/secrets/agents-ca.crt
```

Copy from mytonprovider-coordinator deploy secrets (`coordinator/deploy/secrets/agents-ca.crt`).

Set in `deploy/.env`:

- `AGENT_ENDPOINTS` — comma-separated `host:8443` (Tailscale IP or reachable host)
- `AGENT_AUTH_TOKEN` — same value as on agents and coordinator
- `AGENT_CA_CERT_FILE=/run/secrets/agents-ca.crt`

## Rollout order (staging/production)

1. Deploy updated **agents** (both instances) — must include `RequestStorageInfo` RPC
2. Deploy **mytonstorage-backend** with agent env above
3. Rebuild **coordinator** on new contracts stubs (optional, no logic change)
4. Verify: `POST /api/v1/providers/offers`, then upload → paid → notify logs without DHT timeout

Backend no longer listens on UDP 16167. `tonutils-storage` UDP 47431 remains required for bag overlay.

Set `TONUTILS_STORAGE_EXTERNAL_IP` in `deploy/.env` to the host's public IPv4 or DNS name (e.g. `vm05.proxmox.ip2dns.net`). The storage entrypoint patches `config.json` on start (`ExternalIP` + `ListenAddr`). Without it, bags seed locally but providers cannot find the node in DHT (`peers: 0`).
