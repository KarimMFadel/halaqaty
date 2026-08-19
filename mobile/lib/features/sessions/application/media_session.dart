import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/livekit_media_session.dart';

/// Provider-neutral audio session boundary consumed by room state/UI.
abstract interface class MediaSession {
  Future<void> connect(MediaConnection connection);
  Future<void> disconnect();
  Future<void> setMicrophoneEnabled(bool enabled);
}

final mediaSessionProvider =
    Provider<MediaSession>((ref) => LiveKitMediaSession());
