# Halaqaty Docker Development Setup

The repository Compose stack provides PostgreSQL and a persistent Flutter/Firebase CLI container. Firebase user accounts are created by the application; Firebase CLI login is only for project configuration.

## First use

```powershell
Copy-Item .env.example .env
docker compose build flutter-tools
docker compose up -d postgres flutter-tools
docker compose exec flutter-tools firebase login --no-localhost
docker compose exec flutter-tools flutterfire configure --project=halaqaty-test
```

Put local test values in `.env` when T048 is ready. Do not commit `.env`, Firebase service-account files, or tokens.

## Daily use

```powershell
docker compose up -d postgres flutter-tools
docker compose exec flutter-tools flutter test test
docker compose down
```

The optional local LiveKit server can be started with:

```powershell
docker compose --profile media up -d livekit
```

The backend currently requires a trusted `https`/`wss` LiveKit endpoint when media is enabled, so the local `--dev` LiveKit service is useful for provider-level development but does not replace a TLS LiveKit environment for the real T048 journey.
