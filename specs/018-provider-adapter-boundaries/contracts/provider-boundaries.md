# Internal Provider Boundary Contracts

These are proposed internal signatures for F-018, not new public API endpoints. Existing canonical REST/WS contracts and schema remain unchanged.

## Chat-owned storage contract

Defined in `backend/internal/chat/media_store.go`:

```go
type ObjectPutInput struct {
    ObjectKey string
    MIMEType string
    SizeBytes int64
    Body io.Reader
}

type ObjectStore interface {
    Put(context.Context, ObjectPutInput) error
    PresignGet(context.Context, string, time.Duration) (*url.URL, error)
    ApplyDeleteMarker(context.Context, string) error
    RemoveLatestDeleteMarker(context.Context, string) error
    EnsureChatBucketVersioned(context.Context) error
}

func NewMediaStore(store ObjectStore, opTimeout time.Duration) *MediaStore
```

The existing wrapper methods `Stage(ctx, StageInput) (string,error)`, `PresignGet`, `ApplyDeleteMarker`, `RemoveLatestDeleteMarker`, and `EnsureChatBucketVersioned` keep their consumer behavior. `Stage` derives the key, lowercases/trims and allowlists MIME, and rejects nonpositive size before calling `Put`; it never accepts a caller filename/key. The lower-level input is internal trusted composition, not a public upload DTO. Every storage operation receives the established deadline-bearing context.

`backend/internal/chat/minio/adapter.go` provides proposed `NewAdapter(cfg config.ChatMediaConfig) (*Adapter,error)` and implements `chat.ObjectStore`. It owns SDK construction/options, bucket, versionless signing/removal, retained version listing, and exact marker ID removal. Existing missing/unversioned bucket sentinel errors remain recognizable by `errors.Is`. Startup never creates/enables a bucket. Apply/recovery behavior follows the existing implementation, including exact object-key filtering, latest matching internal marker selection, no-op absence, error propagation, and retained-byte protection; this refactor does not redesign listing semantics. No version ID crosses the neutral contract. Adapter-local `RemoveDeleteMarker(ctx,key,versionID)` may remain for existing infrastructure fixtures.

## Live session media

Existing `sessions.SessionMediaGateway` and its operations/neutral types remain unchanged. Proposed production constructor in `backend/internal/sessions/livekit/adapter.go`:

```go
func NewConfiguredAdapter(cfg config.LiveKitConfig, policy config.AudioPolicy) *Adapter
```

It creates the SDK room client inside that package, then uses existing `NewAdapter` test injection. Main loads the same config and injects directly. Webhook verification, audio policy, lifecycle reconciliation, and participant credentials are unchanged.

## Mobile

`application/media_session.dart` keeps existing `connect(MediaConnection)`, `disconnect()`, and `setMicrophoneEnabled(bool)` operations. Its only model dependency is extracted `domain/media_connection.dart`, retaining endpoint/credential/expiry and JSON behavior. `domain/session_models.dart` reexports that type, retaining compatibility through `session_api_client.dart`'s existing model reexport.

`application/media_session_provider.dart` owns the existing `mediaSessionProvider` Riverpod declaration and direct `LiveKitMediaSession` construction. Existing consumers import wiring explicitly; SDK imports remain confined to `data/livekit_media_session.dart`.

## Architecture guard limits

LiveKit production imports are allowed only in `backend/internal/sessions/livekit`; MinIO production imports only in `backend/internal/chat/minio`. Scan `backend/internal` and `backend/cmd/api`; infrastructure tests may use SDKs for real-service setup and version assertions. Mobile SDK imports remain confined to the single existing adapter; interface/type transitive dependencies must exclude API clients and composition. Guards protect approved dependency direction and complement existing behavior/infrastructure coverage.
