import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/group_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';
import 'package:halaqaty_mobile/features/chat/presentation/group_chat_screen.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/application/session_room_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/domain/session_models.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_room_screen.dart';
import '../../helpers/stub_auth_notifier.dart';

void main() {
  testWidgets('connected session opens its circle chat independently of media',
      (tester) async {
    await tester.pumpWidget(_app());

    await tester.tap(find.text('Join'));
    await tester.pumpAndSettle();
    expect(find.text('Connected. Audio is ready.'), findsOneWidget);
    await tester.tap(find.text('Chat'));
    await tester.pumpAndSettle();

    expect(find.byType(GroupChatScreen), findsOneWidget);
  });
}

Widget _app() => ProviderScope(
      overrides: [
        authControllerProvider.overrideWith((_) => StubAuthNotifier()),
        sessionRoomControllerProvider('session-1').overrideWith(
          (_) => SessionRoomController(
            _SessionApi(),
            () async => (token: 'token', sessionId: 'backend-session'),
            _MediaSession(),
            realtime: _SessionRealtime(),
          ),
        ),
        groupChatControllerProvider('circle-1').overrideWith(
          (_) => GroupChatController(
            _ChatApi(),
            () async => (
              token: 'token',
              sessionId: 'backend-session',
              userId: 'user-1'
            ),
            realtime: _ChatRealtime(),
          ),
        ),
      ],
      child: const MaterialApp(
        home: Directionality(
          textDirection: TextDirection.ltr,
          child: SessionRoomScreen(sessionId: 'session-1'),
        ),
      ),
    );

class _SessionApi extends SessionApiClient {
  _SessionApi() : super(Dio());

  @override
  Future<SessionConnection> join({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      SessionConnection(
        session: const SessionModel(
          id: 'session-1',
          circleId: 'circle-1',
          status: 'active',
          mediaMode: 'audio',
          participantCount: 1,
          isLocked: false,
        ),
        mediaConnection: MediaConnection(
          endpoint: 'wss://media.example',
          credential: 'credential',
          expiresAt: DateTime.utc(2026, 9, 12, 12),
        ),
      );

  @override
  Future<List<SessionParticipant>> participants({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      const [];
}

class _MediaSession implements MediaSession {
  @override
  Future<void> connect(MediaConnection connection) async {}
  @override
  Future<void> disconnect() async {}
  @override
  Future<void> setMicrophoneEnabled(bool enabled) async {}
}

class _SessionRealtime implements RealtimeSessionClient {
  final _events = StreamController<RealtimeSessionEvent>.broadcast();

  @override
  Stream<RealtimeSessionEvent> sessionEvents(String liveSessionId,
          {required String token, required String backendSessionId}) =>
      _events.stream;
  @override
  Future<void> raiseHand(String liveSessionId) async {}
  @override
  Future<void> lowerHand(String liveSessionId) async {}
  @override
  Future<void> dispose() => _events.close();
}

class _ChatApi extends ChatApiClient {
  _ChatApi() : super(Dio());

  @override
  Future<ChatMessagePage> listMessages({
    required String token,
    required String sessionId,
    required String circleId,
    int? limit,
    String? before,
  }) async =>
      const ChatMessagePage(messages: [], hasMore: false);
}

class _ChatRealtime implements ChatRealtimeClient {
  final _events = StreamController<ChatRealtimeEvent>.broadcast();

  @override
  Stream<ChatRealtimeEvent> circleChatEvents(String circleId,
          {required String token, required String backendSessionId}) =>
      _events.stream;
  @override
  Future<void> dispose() => _events.close();
}
