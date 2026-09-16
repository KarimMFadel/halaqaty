import 'package:halaqaty_mobile/features/sessions/domain/media_connection.dart';

/// Provider-neutral audio session boundary consumed by room state/UI.
abstract interface class MediaSession {
  Future<void> connect(MediaConnection connection);
  Future<void> disconnect();
  Future<void> setMicrophoneEnabled(bool enabled);
}
