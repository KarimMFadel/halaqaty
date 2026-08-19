import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/application/session_room_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_room_screen.dart';

void main() {
  testWidgets('renders Arabic RTL loading and student join action',
      (tester) async {
    final api = WidgetSessionApi();
    await tester
        .pumpWidget(_app(api, canStart: false, direction: TextDirection.rtl));
    await tester.tap(find.text('انضمام'));
    await tester.pump();

    expect(find.byType(CircularProgressIndicator), findsOneWidget);
    await tester.pumpAndSettle();
    expect(find.text('تم الاتصال. الصوت متاح.'), findsOneWidget);
  });

  testWidgets('renders Arabic start/join error', (tester) async {
    await tester.pumpWidget(_app(FailingWidgetSessionApi(),
        canStart: true, direction: TextDirection.rtl));
    await tester.tap(find.text('بدء الجلسة'));
    await tester.pumpAndSettle();

    expect(find.text('تعذر الاتصال'), findsOneWidget);
    expect(find.textContaining('start failed'), findsNothing);
  });
}

Widget _app(SessionApiClient api,
        {required bool canStart, required TextDirection direction}) =>
    ProviderScope(
      overrides: [
        sessionRoomControllerProvider('session-1').overrideWith(
          (ref) => SessionRoomController(
              api,
              () async => (token: 'token', sessionId: 'session'),
              NoopMediaSession(),
              realtime: EmptyRealtimeClient()),
        ),
      ],
      child: MaterialApp(
        home: Directionality(
          textDirection: direction,
          child: SessionRoomScreen(sessionId: 'session-1', canStart: canStart),
        ),
      ),
    );

class NoopMediaSession implements MediaSession {
  @override
  Future<void> connect(MediaConnection connection) async {}
  @override
  Future<void> disconnect() async {}
  @override
  Future<void> setMicrophoneEnabled(bool enabled) async {}
}

class EmptyRealtimeClient implements RealtimeSessionClient {
  @override
  Stream<RealtimeSessionEvent> sessionEvents(String liveSessionId,
          {required String token, required String backendSessionId}) =>
      const Stream.empty();

  @override
  Future<void> raiseHand(String liveSessionId) async {}

  @override
  Future<void> lowerHand(String liveSessionId) async {}

  @override
  Future<void> dispose() async {}
}

class WidgetSessionApi extends SessionApiClient {
  WidgetSessionApi() : super(Dio());
  @override
  Future<SessionConnection> join(
          {required String token,
          required String sessionId,
          required String liveSessionId}) =>
      Future<SessionConnection>.delayed(
          const Duration(milliseconds: 50), _connection);

  @override
  Future<List<SessionParticipant>> participants(
          {required String token,
          required String sessionId,
          required String liveSessionId}) async =>
      const [];
}

class FailingWidgetSessionApi extends SessionApiClient {
  FailingWidgetSessionApi() : super(Dio());
  @override
  Future<SessionConnection> start(
          {required String token,
          required String sessionId,
          required String liveSessionId}) =>
      Future.error(StateError('start failed'));
}

SessionConnection _connection() => SessionConnection(
      session: const SessionModel(
          id: 'session-1',
          circleId: 'circle-1',
          status: 'active',
          mediaMode: 'audio',
          participantCount: 1,
          isLocked: false),
      mediaConnection: MediaConnection(
          endpoint: 'wss://media.example',
          credential: 'credential',
          expiresAt: DateTime.now().add(const Duration(minutes: 1))),
    );
