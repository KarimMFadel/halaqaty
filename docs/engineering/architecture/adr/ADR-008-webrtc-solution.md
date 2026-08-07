# ADR-008: WebRTC Solution for Live Audio Sessions

**Status:** Accepted  
**Date:** 2026-07-25  
**Deciders:** Karim (product owner)

---

## Context

Halaqaty requires real-time audio streaming for live Quran recitation sessions. The platform needs to support:

- **One-to-many audio broadcast** — the reciter speaks, and all circle members (typically 3-8 students) listen in real time with low latency (<300ms).
- **Turn-based two-way audio** — when a student's turn is active, they become the speaker and the teacher + other students become listeners.
- **Queue coordination** — audio routing must integrate with the session queue state machine managed by the Go backend.
- **Mobile-first** — Flutter clients (iOS and Android) as primary interface. Web client is a stretch goal, not MVP.
- **Reliability** — audio drops and reconnections are common in Islamic learning contexts where students may be in low-bandwidth or high-jitter environments (e.g., rural areas, mobile networks).

The team is currently one developer (Karim) using Copilot as the primary implementation assistant. The MVP targets ~50 concurrent users across multiple sessions. Operational complexity and time-to-production are critical constraints.

WebRTC is the industry-standard protocol for peer-to-peer real-time communication, but implementing WebRTC correctly — signaling, STUN/TURN servers, SFU/MCU media routing, mobile SDK integration, reconnection logic, audio encoding, and cross-platform compatibility — is a substantial engineering undertaking.

We must decide: **build the WebRTC infrastructure ourselves, use a managed WebRTC platform (LiveKit, Agora, Twilio, etc.), or adopt a simpler alternative protocol.**

---

## Options

### Option 1: Self-Implemented WebRTC with Pion (Go)

**Implementation approach:**
- Use [Pion WebRTC](https://github.com/pion/webrtc) Go library to implement signaling server and Selective Forwarding Unit (SFU) logic inside the Go backend.
- Flutter clients use [flutter-webrtc](https://pub.dev/packages/flutter_webrtc) SDK for media capture, encoding, and peer connection management.
- Build custom signaling protocol over WebSocket for offer/answer/ICE candidate exchange.
- Deploy open-source TURN server (e.g., coturn) for NAT traversal in restrictive networks.
- Implement reconnection logic, audio track routing, and queue-driven speaker transitions in application code.

**Technical control:**
- Full ownership of signaling protocol — can optimize for Halaqaty's queue state machine without external API constraints.
- Audio routing logic lives entirely in the `backend/internal/sessions/` package. No third-party SDK lifecycle to coordinate.
- No per-minute or per-participant pricing. Infrastructure costs are fixed (server + bandwidth).

**Complexity:**
| Component | Effort Estimate | Risk |
|---|---|---|
| **Pion SFU implementation** | 2-3 weeks | High — easy to introduce subtle audio routing bugs, packet loss under load. |
| **Mobile SDK integration** | 1-2 weeks | Medium — flutter-webrtc has production users but documentation is sparse. iOS background audio requires platform channel work. |
| **TURN server deployment** | 3-5 days | Medium — coturn is stable but requires TLS cert management, firewall tuning, and capacity planning. |
| **Reconnection + queue sync** | 1 week | High — state machine coordination between WebSocket queue updates and WebRTC peer connection state is error-prone. |
| **Cross-network testing** | Ongoing | High — NAT traversal, mobile network handoffs, and codec negotiation failures emerge only in production. |

**Operational burden:**
- Must monitor TURN server capacity, WebRTC connection success rates, and audio quality metrics (packet loss, jitter) directly. No built-in observability.
- Debugging audio issues requires packet capture (Wireshark) and deep WebRTC protocol knowledge. Not typical Go backend skillset.
- Security updates to Pion, coturn, and flutter-webrtc must be tracked and deployed manually. No managed upgrade path.

**Risks:**
- **Quality of Service** — SFU load balancing, adaptive bitrate, and echo cancellation require domain expertise. Easy to ship a "works on my WiFi" implementation that fails in rural mobile contexts.
- **Time to MVP** — 4-6 weeks of focused work just to reach feature parity with a managed platform's "hello world" example. Delays all downstream features (queue, progress tracking, etc.).
- **Technical debt** — if scaling past 50 users requires a migration to a managed platform anyway, the initial SFU build becomes throwaway work.

---

### Option 2: Managed WebRTC Platform — LiveKit

**Implementation approach:**
- Deploy [LiveKit](https://livekit.io) open-source server (single Docker container) via docker-compose on the same server as the Go backend for MVP.
- Go backend issues short-lived JWT access tokens via `backend/internal/sessions/livekit.go` adapter. Tokens encode room name, participant identity, and permissions (publish/subscribe).
- Flutter app uses [livekit-client-sdk-flutter](https://pub.dev/packages/livekit_client) to join rooms, publish/subscribe to audio tracks. SDK handles reconnection, encoding, and track management automatically.
- TURN/STUN servers are bundled with LiveKit. No separate coturn deployment.
- Queue state machine in Go backend triggers "update participant permissions" API calls to LiveKit when a student's turn starts/ends. LiveKit enforces audio track routing based on these permissions.

**Integration points:**
```
backend/internal/sessions/
├── livekit.go              ← JWT generation, room creation, participant permission updates
├── queue_coordinator.go    ← emits "student_turn_active" event → calls livekit.UpdateParticipant()
└── service.go              ← exposes GetLiveKitToken(sessionID, userID) to HTTP handler

mobile/lib/features/session/providers/
└── livekit_room_provider.dart  ← wraps LiveKit SDK, syncs with queue state from backend WebSocket
```

**Technical benefits:**
- **Production-grade SFU** — battle-tested in video conferencing apps. Handles mobile network handoffs, simulcast, and adaptive bitrate out of the box.
- **Mobile SDK maturity** — livekit-client-sdk-flutter is actively maintained, supports background audio, and has extensive examples. iOS audio session management is handled.
- **Built-in observability** — LiveKit server exposes Prometheus metrics (track quality, connection state) and participant webhooks (join/leave/reconnect events). Grafana dashboard available.
- **Open-source control** — can self-host on MVP infrastructure. No vendor lock-in; can inspect and patch Go server source if needed.

**Operational simplicity:**
- Single `livekit/livekit-server` Docker image added to `docker-compose.yml`. No separate TURN server, no TLS cert juggling.
- Reconnection, audio encoding, and NAT traversal are LiveKit's responsibility, not application code. Reduces Go backend attack surface.
- Migration path to LiveKit Cloud (managed SaaS) exists if server management becomes a burden post-MVP.

**Costs:**
| Deployment Model | Cost | When to Use |
|---|---|---|
| **Self-hosted (MVP)** | $0 software + existing server resources | <100 concurrent users, single-region. |
| **LiveKit Cloud** | ~$0.01/minute/participant after free tier | >100 users, multi-region, or when operational toil exceeds Karim's capacity. |

**Risks:**
- **Vendor API surface** — queue logic must coordinate with LiveKit's room/participant API. If LiveKit changes these APIs, backend integration code must adapt (though open-source means we can pin a version).
- **Learning curve** — team must understand LiveKit's token claims, participant permissions model, and webhook event schema. Roughly 2-3 days to internalize. Faster than learning WebRTC protocol internals.
- **Self-hosting complexity** — while simpler than Pion+coturn, still requires monitoring a new service (LiveKit server). Acceptable trade for MVP; revisit if server ops become a bottleneck.

**Effort estimate:**
| Task | Time |
|---|---|
| LiveKit server deployment (docker-compose) | 1 day |
| Go backend token issuance + room management | 2-3 days |
| Flutter SDK integration + queue sync | 3-4 days |
| E2E testing (mobile + queue coordination) | 2 days |
| **Total** | **~1.5 weeks** |

---

### Option 3: Managed WebRTC Platform — Agora.io

**Implementation approach:**
- Use [Agora RTC SDK](https://www.agora.io) for Flutter and Go server integration.
- Fully managed SaaS — no self-hosting option. Signaling, TURN, and SFU all run on Agora's global infrastructure.
- Go backend generates Agora access tokens (similar to LiveKit). Agora SDK handles everything client-side.

**Advantages:**
- **Zero infrastructure** — no Docker container to deploy, no server to monitor. Agora handles all scaling, redundancy, and global edge routing.
- **Battle-tested scale** — Agora powers apps with millions of concurrent users (e.g., Clubhouse-style apps). Reliability is best-in-class.
- **Official Flutter SDK** — [agora_rtc_engine](https://pub.dev/packages/agora_rtc_engine) is maintained by Agora, not community. Better documentation and support than flutter-webrtc.

**Disadvantages:**
- **Cost** — Pricing starts at ~$0.99 per 1,000 minutes. For MVP with 50 users × 1 hour/day × 30 days = 1,500 hours = 90,000 minutes/month → **~$90/month**. Not prohibitive, but recurring.
- **Vendor lock-in** — no self-hosting option. Migration off Agora requires rewriting all WebRTC integration code.
- **Opaque internals** — cannot inspect or debug SFU behavior. If audio quality issues arise, must rely on Agora support tickets.

**When to choose this:**
- If time-to-market is absolute priority and budget allows for $100-500/month operational costs.
- If team lacks Go/WebRTC expertise and prefers to outsource reliability to a vendor with SLA guarantees.

**Effort estimate:** Similar to LiveKit (~1.5 weeks), but with no Docker deployment time.

---

### Option 4: Alternative Protocol — Server-Sent Events (SSE) + Media Streams API

**Implementation approach:**
- Skip WebRTC peer connections. Instead, use Flutter's Media Streams API to capture audio on the client, encode to Opus, and POST audio chunks to the Go backend via HTTP.
- Go backend broadcasts audio chunks to all listeners via Server-Sent Events (SSE) long-lived HTTP connections.
- No TURN/STUN servers needed — everything is HTTP over TCP.

**Advantages:**
- **Extreme simplicity** — no Pion library, no WebRTC negotiation, no ICE candidates. Pure HTTP.
- **Firewall-friendly** — SSE works anywhere HTTP works. No UDP or exotic ports required.

**Disadvantages:**
- **Latency** — TCP head-of-line blocking, HTTP request/response overhead, and lack of jitter buffers add 500ms-2s latency. Unacceptable for interactive turn-based recitation.
- **No mobile background audio** — iOS Safari and Chrome do not allow audio capture in background tabs via Media Streams API. Requires foreground app, which breaks queue notifications.
- **Scalability ceiling** — server must relay every audio chunk to every listener. At 10 concurrent sessions × 8 participants × 50kbps audio = 4Mbps outbound per session. Single server exhausted at ~20 concurrent sessions.

**Verdict:** Not viable for Halaqaty's latency and mobile requirements. Mentioned only for completeness.

---

## Decision

We will use **LiveKit (self-hosted)** for WebRTC audio streaming in live Quran recitation sessions.

**Implementation plan:**
- Deploy LiveKit open-source server as a Docker container via `docker-compose.yml` on the same server as the Go backend for MVP.
- Go backend (`backend/internal/sessions/livekit.go`) will issue JWT access tokens with room permissions and manage participant state via LiveKit's Go SDK.
- Flutter mobile app will integrate `livekit-client-sdk-flutter` for audio capture, streaming, and playback.
- Queue state machine in `backend/internal/sessions/queue_coordinator.go` will trigger participant permission updates (publish/subscribe rights) via LiveKit API when student turns change.
- LiveKit's bundled TURN/STUN servers handle NAT traversal - no separate coturn deployment needed.
- Monitor LiveKit metrics via its built-in Prometheus exporter and participant webhooks.

---

## Comparison Matrix

| Criteria | Self-Implemented Pion | LiveKit (Self-Hosted) | Agora.io (SaaS) | SSE + Media Streams |
|---|---|---|---|---|
| **Time to MVP** | 4-6 weeks | 1.5 weeks | 1.5 weeks | 1 week |
| **Audio latency** | <200ms (if done right) | <200ms | <150ms | 500ms-2s |
| **Mobile SDK quality** | Medium (flutter-webrtc) | High (official LiveKit SDK) | Highest (vendor SDK) | Low (iOS background issues) |
| **Infrastructure cost (MVP)** | $0 | $0 (self-hosted) | ~$90/month | $0 |
| **Operational burden** | High (TURN + SFU monitoring) | Medium (LiveKit container) | None | Medium (HTTP scaling) |
| **Vendor lock-in risk** | None | Low (open-source) | High | None |
| **Debuggability** | Full (we own the code) | High (open-source + logs) | Low (opaque SaaS) | Full |
| **Scaling ceiling (self-hosted)** | ~50 users (single server) | ~100 users (single server) | Unlimited (Agora's infra) | ~20 sessions |
| **Quality of Service** | Unknown (we build it) | Proven (production apps) | Best-in-class | Poor (latency) |

---

## Key Questions to Decide

1. **Is 1.5 weeks integration time acceptable vs. 4-6 weeks custom build?** How critical is speed-to-market for the MVP pilot?
2. **Is self-hosting a hard requirement, or is $100/month for Agora acceptable?** Budget vs. operational simplicity trade-off.
3. **Do we anticipate needing to customize WebRTC behavior beyond standard permissions/routing?** If yes, Pion gives full control. If no, LiveKit's API is sufficient.
4. **What is the team's appetite for operating a TURN server and debugging WebRTC issues in production?** Pion requires this expertise; LiveKit/Agora abstract it away.
5. **Is there a risk of scaling past 100 concurrent users within 6 months of MVP launch?** If yes, starting with a managed platform (LiveKit Cloud or Agora) avoids a mid-flight migration.

---

## Recommendation Framework

| If your top priority is... | Choose... |
|---|---|
| **Minimum viable cost** | LiveKit (self-hosted) |
| **Fastest time to production** | LiveKit (self-hosted) or Agora |
| **Maximum technical control** | Self-implemented Pion |
| **Zero operational burden** | Agora |
| **Avoiding vendor lock-in** | LiveKit (self-hosted) — open-source exit path |
| **Best audio quality guarantees** | Agora |

---

## Consequences

**Positive:**
- **Fast time to MVP** — 1.5 weeks integration time vs. 4-6 weeks for custom Pion implementation. Gets live audio streaming into users' hands faster.
- **Production-grade reliability** — LiveKit's SFU is battle-tested in real-world video conferencing apps. Mobile network handoffs, reconnection logic, and adaptive bitrate are proven.
- **Mobile SDK quality** — `livekit-client-sdk-flutter` is actively maintained with official support for iOS background audio, Android lifecycle management, and comprehensive examples.
- **Zero infrastructure cost for MVP** — Self-hosted deployment uses existing server resources. No per-minute pricing.
- **Low operational burden** — Single Docker container to monitor. LiveKit handles TURN/STUN, audio encoding, and reconnection internally. Go backend only manages tokens and permissions.
- **Built-in observability** — Prometheus metrics for track quality, connection state, and participant events out of the box. Can integrate with Grafana for dashboards.
- **Open-source exit path** — Not vendor-locked. Can inspect/patch LiveKit server source if needed. Migration to LiveKit Cloud (SaaS) available if self-hosting becomes a burden post-MVP.
- **Integration with queue state machine** — LiveKit's participant permissions API maps cleanly to Halaqaty's turn-based queue model. When a student's turn activates, backend calls `livekit.UpdateParticipant()` to grant publish rights.

**Negative:**
- **New service to operate** — Adds LiveKit container to `docker-compose.yml`. Must monitor LiveKit server health, disk usage (for recordings if enabled), and connection metrics. Acceptable trade-off for MVP; revisit if ops burden exceeds capacity.
- **Learning curve** — Team must understand LiveKit's JWT token claims, room/participant API, and webhook event schema. Estimated 2-3 days to internalize. Still faster than learning WebRTC protocol internals.
- **Dependency on LiveKit API stability** — Queue coordination logic in Go backend depends on LiveKit's participant permissions model. If LiveKit changes these APIs in future versions, integration code must adapt. Mitigated by pinning LiveKit server version in `docker-compose.yml` and testing upgrades in staging.
- **Self-hosting complexity vs. SaaS** — While simpler than Pion+coturn, still requires server capacity planning for concurrent sessions. At MVP scale (~50 users), single server is sufficient. Must monitor resource usage and plan for horizontal scaling (multiple LiveKit servers behind load balancer) if usage exceeds 100 concurrent participants.

**Technical debt to monitor:**
- If the platform scales to multi-region deployments (e.g., separate servers for Middle East and Southeast Asia), self-hosted LiveKit requires manual replication and routing logic. At that scale, migration to LiveKit Cloud (managed edge network) should be evaluated. The API contract is identical, so migration is primarily infrastructure, not code.

**Alternatives revisited:**
- **Pion rejected** due to time-to-MVP (4-6 weeks) and operational complexity (TURN server, custom SFU debugging). If MVP proves that audio quality/latency must be micro-optimized beyond LiveKit's capabilities, can revisit post-MVP with real production data.
- **Agora rejected** due to preference for self-hosting and avoiding recurring per-minute costs at MVP stage. If operational burden of self-hosted LiveKit becomes unsustainable, Agora remains a viable fallback (though requires rewriting Flutter integration).
- **SSE rejected** due to unacceptable latency (500ms-2s) for interactive recitation sessions.

---

## References

- `../ARCHITECTURE.md` — session queue state machine that WebRTC audio routing must integrate with
- `ADR-001-modular-monolith.md` — `backend/internal/sessions/` package owns WebRTC coordination logic regardless of chosen option
- [Pion WebRTC](https://github.com/pion/webrtc) — Go WebRTC library for Option 1
- [LiveKit](https://livekit.io) — open-source WebRTC platform for Option 2
- [Agora.io](https://www.agora.io) — managed WebRTC SaaS for Option 3
- [livekit-client-sdk-flutter](https://pub.dev/packages/livekit_client) — Flutter SDK for LiveKit
- [agora_rtc_engine](https://pub.dev/packages/agora_rtc_engine) — Flutter SDK for Agora
- [flutter-webrtc](https://pub.dev/packages/flutter_webrtc) — Community WebRTC SDK for custom Pion integration
