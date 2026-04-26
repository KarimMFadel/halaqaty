# Halaqaty — Deployment Strategy

> **Version:** 1.0 | **Status:** Planning Phase | **Last Updated:** 2026

**Related Documents:** [PRD.md](PRD.md) · [ARCHITECTURE.md](ARCHITECTURE.md) · [PLAN.md](PLAN.md)

---

## Table of Contents

1. [Deployment Philosophy](#1-deployment-philosophy)
2. [Phase 1 — MVP (10–50 Users)](#2-phase-1--mvp-10-50-users)
3. [Phase 2 — Growth (100–500 Users)](#3-phase-2--growth-100-500-users)
4. [Phase 3 — Scale (500–5,000 Users)](#4-phase-3--scale-500-5000-users)
5. [Phase 4 — Global (5,000+ Users)](#5-phase-4--global-5000-users)
6. [Docker Compose Configuration](#6-docker-compose-configuration)
7. [Monitoring Strategy](#7-monitoring-strategy)
8. [Backup Strategy](#8-backup-strategy)
9. [SSL/TLS and DNS](#9-ssltls-and-dns)
10. [CI/CD Pipeline](#10-cicd-pipeline)

---

## 1. Deployment Philosophy

**Start lean. Scale when needed. Never over-engineer early.**

Our deployment strategy follows these principles:
1. **Phase 1 runs on $8–12/month.** We will not invest in complex infrastructure before proving the product.
2. **Docker Compose first.** Easy to manage, easy to debug, sufficient for hundreds of users.
3. **Kubernetes when necessary.** Only when Docker Compose is a genuine bottleneck.
4. **Self-hosted first, cloud second.** Hetzner over AWS for cost efficiency. Move to AWS/GCP only when Hetzner capacity is insufficient.
5. **No vendor lock-in.** MinIO instead of S3, LiveKit instead of Twilio, Firebase only for Auth/FCM (easily replaceable).

---

## 2. Phase 1 — MVP (10–50 Users)

**Timeline:** Months 1–5 | **Cost:** ~$8–12/month

### Infrastructure

**Single Hetzner CX22 server:**

| Spec | Value |
|------|-------|
| Provider | Hetzner Cloud |
| Server Type | CX22 |
| vCPU | 2 AMD vCPU |
| RAM | 4 GB |
| Storage | 40 GB NVMe SSD |
| Bandwidth | 20 TB/month |
| Location | Nuremberg, Germany (low latency for EU/MENA) |
| **Cost** | **~$4.50–6/month** |

**Additional costs:**
- Cloudflare (Free tier): DNS + SSL/TLS
- Firebase (Free Spark plan): Auth + FCM (sufficient for < 10K MAU)
- Domain: ~$12/year (halaqaty.app or similar)
- **Total: ~$8–12/month** 🎯

### Services on Phase 1 Server

All services run in Docker containers on a single server:

```
╔═══════════════════════════════════════════════╗
║          Hetzner CX22 — Single Server         ║
║                                               ║
║  ┌──────────────────────────────────────────┐ ║
║  │          Nginx (Reverse Proxy)            │ ║
║  │  halaqaty.app → Go API container         │ ║
║  │  ws.halaqaty.app → Go WebSocket          │ ║
║  │  lk.halaqaty.app → LiveKit container     │ ║
║  │  files.halaqaty.app → MinIO container    │ ║
║  └──────────────────────────────────────────┘ ║
║                                               ║
║  ┌───────────┐  ┌───────────┐  ┌───────────┐ ║
║  │ Go Backend│  │ LiveKit   │  │   MinIO   │ ║
║  │ (API+WS)  │  │  Server   │  │  Server   │ ║
║  └───────────┘  └───────────┘  └───────────┘ ║
║                                               ║
║  ┌───────────────────────────────────────────┐ ║
║  │           PostgreSQL 16                   │ ║
║  │         (Primary Database)                │ ║
║  └───────────────────────────────────────────┘ ║
╚═══════════════════════════════════════════════╝
```

### Phase 1 Capacity Estimates

| Resource | Estimate | Notes |
|----------|---------|-------|
| Concurrent live sessions | 2–5 | MVP assumes audio-only sessions; video remains post-MVP feature-flagged |
| Simultaneous WebSocket connections | 100 | Go handles this easily |
| Database connections | 50 | PostgreSQL on same server |
| Daily active users | 50 | Phase 1 target |
| Storage (MinIO) | < 10 GB | Voice notes, images; 40 GB SSD sufficient |

### Phase 1 Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| RAM exhaustion during large sessions | Low | High | Monitor; add swap; upgrade to CX32 if needed |
| Accidental video enablement in MVP | Low | High | Keep video feature flag OFF by default until post-MVP capacity upgrade |
| Server downtime | Medium | High | Take daily snapshots (Hetzner feature, free) |
| LiveKit + Go + PostgreSQL on same server | Accepted | Medium | Acceptable for pilot; separate in Phase 2 |

---

## 3. Phase 2 — Growth (100–500 Users)

**Timeline:** Months 6–9 | **Cost:** ~$23–30/month

### Trigger to Move to Phase 2

Move when **any** of:
- Concurrent live sessions consistently > 5
- RAM usage on CX22 consistently > 70%
- Response times degrading (P95 > 500ms)
- Storage approaching 35 GB

### Infrastructure

**Two Hetzner servers:**

| Server | Type | Role | Cost |
|--------|------|------|------|
| App Server | CX32 (4 vCPU, 8 GB RAM) | Go Backend + PostgreSQL + MinIO | ~$10/month |
| LiveKit Server | CX32 (4 vCPU, 8 GB RAM) | LiveKit SFU (dedicated) | ~$10/month |
| **Cloudflare** | Free | DNS + CDN + SSL | $0 |
| **Total** | | | **~$20–25/month** |

### Why Separate LiveKit?

LiveKit SFU is CPU and bandwidth intensive during active sessions. By separating it:
- App server is not affected by session load spikes
- LiveKit server can be independently upgraded
- LiveKit can be geographically closer to users in future

### Phase 2 Architecture

```
┌──────────────────┐       ┌──────────────────┐
│    App Server    │       │  LiveKit Server   │
│  (Hetzner CX32)  │       │  (Hetzner CX32)   │
│                  │       │                  │
│  ┌────────────┐  │       │  ┌────────────┐  │
│  │ Go Backend │  │       │  │  LiveKit   │  │
│  │ (API + WS) │  │◄─────►│  │    SFU     │  │
│  └────────────┘  │       │  └────────────┘  │
│  ┌────────────┐  │       │                  │
│  │ PostgreSQL │  │       │  UDP: 7880-7900  │
│  │   (DB)     │  │       │  TCP: 7880       │
│  └────────────┘  │       └──────────────────┘
│  ┌────────────┐  │
│  │   MinIO    │  │
│  │  (Files)   │  │
│  └────────────┘  │
└──────────────────┘
```

### Phase 2 Capacity

| Resource | Estimate |
|----------|---------|
| Concurrent live sessions | 10–20 |
| Daily active users | 200–500 |
| Concurrent WebSocket connections | 500 |
| Storage | 50–200 GB (add Hetzner Volumes: ~$5/month per 100 GB) |

> Note: The above estimates assume audio-only sessions in MVP. Enabling video post-MVP materially increases bandwidth and SFU CPU needs; re-baseline before toggling the video feature flag.

---

## 4. Phase 3 — Scale (500–5,000 Users)

**Timeline:** Months 10–18 | **Cost:** ~$100–200/month

### Trigger to Move to Phase 3

Move when **any** of:
- Concurrent live sessions > 20
- PostgreSQL becomes a bottleneck (query latency P95 > 100ms)
- A single LiveKit server is insufficient for peak load
- Need for zero-downtime deployments (rolling updates)

### Infrastructure: Kubernetes on Hetzner or DigitalOcean

**Option A: Hetzner Kubernetes (HKS)** — Recommended (cost-efficient)
- 3-node cluster: CX32 (4 vCPU, 8 GB) × 3 = ~$30/month
- Hetzner managed Kubernetes control plane: free
- Hetzner Load Balancer: ~$6/month
- Hetzner Volumes for PostgreSQL: ~$20/month
- MinIO distributed on cluster: included
- **Total: ~$60–80/month**

**Option B: DigitalOcean Kubernetes (DOKS)** — Simpler managed experience
- 3-node cluster: s-2vcpu-4gb × 3 = ~$72/month
- Managed PostgreSQL: ~$30/month
- Spaces (S3-compatible, replaces MinIO): ~$5/month
- Load Balancer: ~$12/month
- **Total: ~$120–150/month**

### Phase 3 Architecture

```
                    ┌─────────────────┐
                    │  Cloudflare CDN  │
                    │   (Global Edge)  │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │  Load Balancer  │
                    │ (Hetzner/DO LB) │
                    └────────┬────────┘
                             │
        ┌────────────────────┼────────────────────┐
        │                    │                    │
┌───────▼───────┐   ┌────────▼──────┐   ┌────────▼──────┐
│  Go Backend   │   │  Go Backend   │   │  Go Backend   │
│   Pod 1       │   │    Pod 2      │   │    Pod 3      │
│  (API + WS)   │   │  (API + WS)  │   │  (API + WS)   │
└───────────────┘   └───────────────┘   └───────────────┘
        │                    │                    │
        └────────────────────┼────────────────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
    ┌─────────▼─────┐  ┌─────▼─────┐  ┌───▼─────────┐
    │  PostgreSQL   │  │  MinIO    │  │  LiveKit    │
    │  (Primary     │  │ (Dist.    │  │  (2+ nodes  │
    │  + Replica)   │  │  Storage) │  │  for scale) │
    └───────────────┘  └───────────┘  └─────────────┘
```

### Phase 3 Features Enabled by Kubernetes

- **Rolling deployments:** zero downtime updates
- **Auto-scaling:** Go backend pods scale up/down based on load
- **Health checks:** Kubernetes restarts crashed containers automatically
- **Horizontal Pod Autoscaler:** Go backend 2–10 pods based on CPU/connection count
- **Persistent Volumes:** PostgreSQL and MinIO data survives pod restarts

---

## 5. Phase 4 — Global (5,000+ Users)

**Timeline:** Month 18+ | **Cost:** $500+/month

### Infrastructure: AWS / GCP Multi-Region

At this scale, the platform serves users across multiple continents. Latency to the media server (LiveKit) becomes critical for audio quality.

**AWS Architecture:**
- EKS (Elastic Kubernetes Service) clusters in 2–3 regions
- RDS PostgreSQL (Multi-AZ): primary in eu-west-1, read replica in us-east-1
- S3 for file storage (replacing MinIO)
- CloudFront CDN for static assets
- LiveKit nodes deployed in multiple regions for geographic proximity
- Route 53 with latency-based routing → users connect to nearest LiveKit node
- **Cost:** $500–2,000+/month depending on usage

### Global LiveKit Topology

```
User in Egypt ────────────────────────► LiveKit Node (Frankfurt/Bahrain)
User in Malaysia ───────────────────────► LiveKit Node (Singapore)
User in USA ────────────────────────────► LiveKit Node (Virginia)

All LiveKit nodes → Go Backend → PostgreSQL (primary in EU)
```

---

## 6. Docker Compose Configuration

Below is the Phase 1 Docker Compose structure. Actual values use `.env` file for secrets.

```yaml
# docker-compose.yml (Phase 1)
version: '3.8'

services:
  # Go Backend (REST API + WebSocket)
  api:
    image: halaqaty/api:latest
    restart: always
    ports:
      - "8080:8080"
    environment:
      - DB_URL=${DB_URL}
      - FIREBASE_PROJECT_ID=${FIREBASE_PROJECT_ID}
      - LIVEKIT_API_KEY=${LIVEKIT_API_KEY}
      - LIVEKIT_API_SECRET=${LIVEKIT_API_SECRET}
      - LIVEKIT_HOST=livekit:7880
      - MINIO_ENDPOINT=minio:9000
      - MINIO_ACCESS_KEY=${MINIO_ACCESS_KEY}
      - MINIO_SECRET_KEY=${MINIO_SECRET_KEY}
    depends_on:
      - postgres
      - minio
    networks:
      - halaqaty

  # PostgreSQL Database
  postgres:
    image: postgres:16-alpine
    restart: always
    volumes:
      - postgres_data:/var/lib/postgresql/data
    environment:
      - POSTGRES_DB=halaqaty
      - POSTGRES_USER=${DB_USER}
      - POSTGRES_PASSWORD=${DB_PASSWORD}
    networks:
      - halaqaty

  # LiveKit SFU Server
  livekit:
    image: livekit/livekit-server:latest
    restart: always
    ports:
      - "7880:7880"         # HTTP/WebSocket
      - "7881:7881"         # RTC TCP
      - "50000-60000:50000-60000/udp"  # UDP for WebRTC
    volumes:
      - ./livekit.yaml:/etc/livekit.yaml
    command: --config /etc/livekit.yaml
    networks:
      - halaqaty

  # MinIO Object Storage
  minio:
    image: minio/minio:latest
    restart: always
    command: server /data --console-address ":9001"
    ports:
      - "9000:9000"
      - "9001:9001"
    volumes:
      - minio_data:/data
    environment:
      - MINIO_ROOT_USER=${MINIO_ACCESS_KEY}
      - MINIO_ROOT_PASSWORD=${MINIO_SECRET_KEY}
    networks:
      - halaqaty

  # Nginx Reverse Proxy
  nginx:
    image: nginx:alpine
    restart: always
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - certbot_data:/etc/letsencrypt
    depends_on:
      - api
      - livekit
      - minio
    networks:
      - halaqaty

volumes:
  postgres_data:
  minio_data:
  certbot_data:

networks:
  halaqaty:
    driver: bridge
```

### LiveKit Configuration (`livekit.yaml`)

```yaml
# livekit.yaml
port: 7880
rtc:
  tcp_port: 7881
  port_range_start: 50000
  port_range_end: 60000
  use_external_ip: true

keys:
  # Set via environment variables in production
  # halaqaty_api_key: "halaqaty_api_secret"

room:
  auto_create: false  # rooms only created via API
  empty_timeout: 300  # 5 min after all participants leave

audio:
  active_speaker_threshold: -30  # dBFS
```

---

## 7. Monitoring Strategy

### Phase 1: Minimal Monitoring

| Tool | Purpose | Cost |
|------|---------|------|
| Hetzner Cloud Graphs | CPU, RAM, network | Free (built-in) |
| UptimeRobot | HTTP uptime checks (every 5 min) | Free tier |
| PostgreSQL logs | Query analysis, slow queries | Built-in |
| Docker logs | Application logs | Built-in |

### Phase 2+: Full Observability Stack

```
┌─────────────────────────────────────────────────────┐
│                  Monitoring Stack                    │
│                                                     │
│  ┌──────────────┐    ┌──────────────┐               │
│  │  Prometheus  │    │   Grafana    │               │
│  │  (Metrics)   │───►│ (Dashboards) │               │
│  └──────────────┘    └──────────────┘               │
│         ▲                                           │
│  ┌──────┴───────┐    ┌──────────────┐               │
│  │  Go Backend  │    │    Loki      │               │
│  │   /metrics   │    │  (Logs)      │               │
│  │  (Prometheus │    └──────────────┘               │
│  │   exporter)  │           ▲                       │
│  └──────────────┘    ┌──────┴───────┐               │
│                      │  Promtail    │               │
│  ┌──────────────┐    │ (Log agent)  │               │
│  │  PostgreSQL  │    └──────────────┘               │
│  │  Exporter    │                                   │
│  └──────────────┘                                   │
└─────────────────────────────────────────────────────┘
```

**Key Metrics to Track:**

| Metric | Alert Threshold | Action |
|--------|----------------|--------|
| API response time P95 | > 500ms | Investigate slow queries |
| WebSocket connections | > 1000 (Phase 2) | Scale Go backend |
| Active LiveKit sessions | > 15 (Phase 2 CX32) | Add LiveKit node |
| PostgreSQL queries/sec | > 500 | Add read replica |
| RAM usage | > 80% | Upgrade server or scale |
| Disk usage | > 75% | Add volume or migrate |
| Error rate (5xx) | > 1% | Immediate investigation |

**Alerting:** Grafana AlertManager → Telegram bot notification to dev team channel.

---

## 8. Backup Strategy

### Database Backups

**Phase 1 (Docker Compose):**
```bash
# Daily cron job at 2:00 AM UTC
0 2 * * * docker exec halaqaty_postgres_1 \
  pg_dump -U halaqaty halaqaty | \
  gzip > /backups/halaqaty_$(date +%Y%m%d).sql.gz

# Rotate: keep 7 days local, upload to Hetzner Object Storage
0 3 * * * rclone copy /backups/ hetzner:halaqaty-backups/
find /backups/ -mtime +7 -delete
```

**Phase 2+:** Use PostgreSQL continuous archiving (WAL shipping) for point-in-time recovery.

### File Storage Backups (MinIO)

**Phase 1:** MinIO data is on the Hetzner server's SSD volume. Daily Hetzner server snapshots (~$0.01/hour) provide recovery point.

**Phase 2+:** MinIO replication between App Server and a dedicated storage server.

### Backup Retention Policy

| Backup Type | Retention | Location |
|------------|-----------|---------|
| PostgreSQL daily dump | 7 days | Hetzner Object Storage |
| PostgreSQL weekly dump | 4 weeks | Hetzner Object Storage |
| PostgreSQL monthly dump | 12 months | Hetzner Object Storage |
| Hetzner Server Snapshot | 3 snapshots | Hetzner (auto-rotated) |
| MinIO voice notes | Until deleted by user | MinIO (server volume) |

### Recovery Procedures

**Scenario 1: Server failure (Phase 1)**
1. Provision new CX22 on Hetzner
2. Restore from last Hetzner snapshot (RPO: 24 hours)
3. If snapshot unavailable, restore PostgreSQL from daily backup
4. Update Cloudflare DNS to new server IP

**Scenario 2: Data corruption**
1. Restore PostgreSQL from last clean daily backup
2. Point-in-time recovery available from Phase 2+

---

## 9. SSL/TLS and DNS

### DNS Setup (Cloudflare)

```
halaqaty.app         A      → Server IP (Proxied ✅)
www.halaqaty.app     CNAME  → halaqaty.app (Proxied ✅)
api.halaqaty.app     A      → Server IP (Proxied ✅)
lk.halaqaty.app      A      → LiveKit Server IP (DNS only ⚡)
files.halaqaty.app   A      → Server IP (Proxied ✅)
```

**Note:** LiveKit must be "DNS only" (not proxied) because WebRTC requires direct UDP connections — Cloudflare's proxy doesn't support UDP.

### SSL/TLS

- **halaqaty.app, api.halaqaty.app, files.halaqaty.app:** Cloudflare manages SSL (free, automatic)
- **lk.halaqaty.app:** Let's Encrypt via certbot (direct connection, bypasses Cloudflare proxy)
- TLS minimum version: 1.2
- HSTS: Strict-Transport-Security: max-age=31536000; includeSubDomains

### Cloudflare Settings

- **Security:** Bot Fight Mode ON, DDoS protection (automatic)
- **Performance:** Caching for static assets only; API is "Bypass" cache
- **WebSocket:** Enabled (required for our real-time features)
- **Firewall Rules:** Block countries/IPs as needed based on traffic analysis

---

## 10. CI/CD Pipeline

### Phase 1: Simple Pipeline (GitHub Actions)

```yaml
# .github/workflows/deploy.yml
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Build Go backend
        run: |
          go build -o halaqaty-api ./cmd/api
          
      - name: Build Docker image
        run: |
          docker build -t halaqaty/api:${{ github.sha }} .
          docker tag halaqaty/api:${{ github.sha }} halaqaty/api:latest
          
      - name: Push to registry
        run: docker push halaqaty/api:latest
        
      - name: Deploy to server
        run: |
          ssh deploy@${{ secrets.SERVER_IP }} \
            "cd /app && docker-compose pull && docker-compose up -d"
```

### Phase 2+: GitOps with ArgoCD

- Git push → GitHub Actions builds and pushes Docker image
- ArgoCD detects new image → applies Kubernetes manifests
- Blue-green or rolling deployment strategy
- Automatic rollback on health check failure

### Deployment Checklist (Per Release)

- [ ] All tests passing in CI
- [ ] Database migrations tested on staging
- [ ] Backup taken before migration
- [ ] Release notes prepared
- [ ] LiveKit configuration unchanged (or explicitly tested)
- [ ] Firebase Auth rules reviewed (if changed)
- [ ] Monitoring dashboards checked post-deploy

---

## Cost Summary

| Phase | Users | Monthly Cost | Key Change |
|-------|-------|-------------|-----------|
| Phase 1 (Months 1–5) | 10–50 | $8–12 | Single Hetzner CX22 |
| Phase 2 (Months 6–9) | 100–500 | $23–30 | Separate LiveKit server |
| Phase 3 (Months 10–18) | 500–5,000 | $100–200 | Kubernetes cluster |
| Phase 4 (Month 18+) | 5,000+ | $500–2,000+ | AWS/GCP multi-region |

**Revenue breakeven target:** 100 paying teachers at $5/month = $500/month (covers Phase 3 infrastructure)

---

*See [ARCHITECTURE.md](ARCHITECTURE.md) for the technical system design.*
