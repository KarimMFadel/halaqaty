import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/application/session_room_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_room_screen.dart';

void main() {
  testWidgets(
      'moderator sees lock, mute-all, remove and end controls in RTL with hand state',
      (tester) async {
    final semantics = tester.ensureSemantics();
    final realtime = RecordingRealtimeClient();
    await tester.pumpWidget(_app(ModerationSessionApi(),
        realtime: realtime,
        canStart: true,
        isModerator: true,
        direction: TextDirection.rtl));
    await tester.tap(find.text('بدء الجلسة'));
    await tester.pumpAndSettle();

    expect(find.text('قفل الجلسة'), findsOneWidget);
    expect(find.text('كتم الجميع'), findsOneWidget);
    expect(find.text('إنهاء الجلسة'), findsOneWidget);
    expect(find.text('كتم'), findsWidgets);
    expect(find.text('إزالة'), findsWidgets);
    expect(find.text('عمر عبدالله'), findsOneWidget);
    expect(find.text('طالب'), findsOneWidget);
    expect(find.byIcon(Icons.pan_tool), findsOneWidget);
    expect(find.bySemanticsLabel('رافع اليد'), findsOneWidget);
    semantics.dispose();
  });

  testWidgets('member does not see moderation controls and can raise hand',
      (tester) async {
    final realtime = RecordingRealtimeClient();
    await tester.pumpWidget(_app(ModerationSessionApi(),
        realtime: realtime,
        canStart: false,
        isModerator: false,
        direction: TextDirection.rtl));
    await tester.tap(find.text('انضمام'));
    await tester.pumpAndSettle();

    expect(find.text('قفل الجلسة'), findsNothing);
    expect(find.text('كتم الجميع'), findsNothing);
    expect(find.text('إنهاء الجلسة'), findsNothing);
    expect(find.text('كتم'), findsNothing);
    expect(find.text('إزالة'), findsNothing);

    await tester.tap(find.text('رفع اليد'));
    await tester.pump();
    expect(realtime.raiseCalls, 1);
  });

  testWidgets('renders moderator controls and hand state in LTR',
      (tester) async {
    final semantics = tester.ensureSemantics();
    final realtime = RecordingRealtimeClient();
    await tester.pumpWidget(_app(ModerationSessionApi(),
        realtime: realtime,
        canStart: true,
        isModerator: true,
        direction: TextDirection.ltr));
    await tester.tap(find.text('Start session'));
    await tester.pumpAndSettle();

    expect(find.text('Lock session'), findsOneWidget);
    expect(find.text('Mute all'), findsOneWidget);
    expect(find.text('End session'), findsOneWidget);
    expect(find.text('Mute'), findsWidgets);
    expect(find.text('Remove'), findsWidgets);
    expect(find.text('Student'), findsOneWidget);
    expect(find.byIcon(Icons.pan_tool), findsOneWidget);
    expect(find.bySemanticsLabel('Hand raised'), findsOneWidget);
    semantics.dispose();
  });
}

Widget _app(SessionApiClient api,
        {required RecordingRealtimeClient realtime,
        required bool canStart,
        required bool isModerator,
        required TextDirection direction}) =>
    ProviderScope(
      overrides: [
        sessionRoomControllerProvider('session-1').overrideWith(
          (ref) => SessionRoomController(
              api,
              () async => (token: 'token', sessionId: 'session'),
              NoopMediaSession(),
              realtime: realtime,
              isModerator: isModerator),
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

class RecordingRealtimeClient implements RealtimeSessionClient {
  int raiseCalls = 0;
  int lowerCalls = 0;
  final StreamController<RealtimeSessionEvent> _events =
      StreamController<RealtimeSessionEvent>.broadcast();

  @override
  Stream<RealtimeSessionEvent> sessionEvents(String liveSessionId,
          {required String token, required String backendSessionId}) =>
      _events.stream;

  @override
  Future<void> raiseHand(String liveSessionId) async {
    raiseCalls++;
  }

  @override
  Future<void> lowerHand(String liveSessionId) async {
    lowerCalls++;
  }

  @override
  Future<void> dispose() => _events.close();
}

class ModerationSessionApi extends SessionApiClient {
  ModerationSessionApi() : super(Dio());

  @override
  Future<SessionConnection> start(
          {required String token,
          required String sessionId,
          required String liveSessionId}) async =>
      _connection();

  @override
  Future<SessionConnection> join(
          {required String token,
          required String sessionId,
          required String liveSessionId}) async =>
      _connection();

  @override
  Future<List<SessionParticipant>> participants(
          {required String token,
          required String sessionId,
          required String liveSessionId}) async =>
      _participants;
}

final _participants = [
  const SessionParticipant(
      userId: 'teacher-1',
      displayName: 'أحمد الشيخ',
      role: CircleRole.teacher,
      isCurrentlyPresent: true),
  SessionParticipant(
      userId: 'student-1',
      displayName: 'عمر عبدالله',
      role: CircleRole.student,
      isCurrentlyPresent: true,
      handRaisedAt: DateTime.utc(2026, 1, 1)),
];

SessionConnection _connection() => SessionConnection(
      session: const SessionModel(
          id: 'session-1',
          circleId: 'circle-1',
          status: 'active',
          mediaMode: 'audio',
          participantCount: 2,
          isLocked: false),
      mediaConnection: MediaConnection(
          endpoint: 'wss://media.example',
          credential: 'credential',
          expiresAt: DateTime.now().add(const Duration(minutes: 1))),
    );
