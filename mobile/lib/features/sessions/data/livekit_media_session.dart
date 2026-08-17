import 'package:livekit_client/livekit_client.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';

/// The only mobile file allowed to import LiveKit SDK types (ADR-015).
class LiveKitMediaSession implements MediaSession {
  LiveKitMediaSession({Room? room})
      : _room = room ?? Room(roomOptions: const RoomOptions());
  final Room _room;

  @override
  Future<void> connect(MediaConnection connection) async {
    await disconnect();
    await _room.connect(connection.endpoint, connection.credential);
    await _room.localParticipant?.setMicrophoneEnabled(true);
  }

  @override
  Future<void> disconnect() => _room.disconnect();

  @override
  Future<void> setMicrophoneEnabled(bool enabled) =>
      _room.localParticipant?.setMicrophoneEnabled(enabled) ?? Future.value();
}
