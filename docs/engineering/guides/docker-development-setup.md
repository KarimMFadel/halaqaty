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

LiveKit endpoint validation is TLS-only for remote hosts (`https`/`wss`); plain `http`/`ws` is accepted only on loopback hosts (`localhost` or a loopback IP) for local development. The compose `--dev` LiveKit service is therefore sufficient for the real integration journeys run locally — for example `LIVEKIT_ENDPOINT=ws://localhost:7880` with `devkey`/`secret` when the API runs on the host. The real T048 journey was verified this way on 2026-09-01; see the [Firebase and LiveKit testing setup guide](firebase-livekit-testing-setup.md).
