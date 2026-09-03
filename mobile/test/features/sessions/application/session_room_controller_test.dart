import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/application/session_room_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';

void main() {
  test(
      'start connects through the neutral media boundary without storing credentials outside state',
      () async {
    final api = FakeSessionApi();
    final media = FakeMediaSession();
    final controller = SessionRoomController(
        api, () async => (token: 'token', sessionId: 'backend-session'), media,
        realtime: FakeRealtimeClient());
    addTearDown(controller.dispose);

    await controller.start('live-session-1');

    expect(controller.state.status, SessionRoomStatus.connected);
    expect(media.connections, 1);
    expect(media.lastConnection?.credential, 'short-lived-credential');
    expect(api.lastToken, 'token');
    expect(api.lastSessionId, 'backend-session');
  });

  test('join exposes an error state and does not connect media when API fails',
      () async {
    final media = FakeMediaSession();
    final controller = SessionRoomController(FailingSessionApi(),
        () async => (token: 'token', sessionId: 'session'), media,
        realtime: FakeRealtimeClient());
    addTearDown(controller.dispose);

    await controller.join('live-session-1');

    expect(controller.state.status, SessionRoomStatus.error);
    expect(controller.state.errorMessage, contains('join failed'));
    expect(media.connections, 0);
  });

  test('retry retains the first attempted session when credentials fail early',
      () async {
    var credentialAttempts = 0;
    final api = FakeSessionApi();
    final controller = SessionRoomController(
      api,
      () async {
        if (credentialAttempts++ == 0) {
          throw StateError('temporary credential failure');
        }
        return (token: 'token', sessionId: 'session');
      },
      FakeMediaSession(),
      realtime: FakeRealtimeClient(),
    );
    addTearDown(controller.dispose);

    await controller.join('live-session-1');
    expect(controller.state.status, SessionRoomStatus.error);
    expect(controller.state.recovery, SessionRoomRecovery.retryable);

    await controller.retry();

    expect(controller.state.status, SessionRoomStatus.connected);
  });

  test('connect loads the authoritative participants snapshot', () async {
    final api = FakeSessionApi();
    final controller = SessionRoomController(api,
        () async => (token: 'token', sessionId: 'session'), FakeMediaSession(),
        realtime: FakeRealtimeClient());
    addTearDown(controller.dispose);

    await controller.join('live-session-1');

    expect(controller.state.status, SessionRoomStatus.connected);
    expect(controller.state.participants.length, 2);
    final raised = controller.state.participants
        .singleWhere((p) => p.userId == 'student-1');
    expect(raised.isHandRaised, isTrue);
    expect(controller.state.isLocked, isFalse);
  });

  test('applies realtime events and skips consecutive duplicates', () async {
    final realtime = FakeRealtimeClient();
    final api = FakeSessionApi()
      ..participantsList = [_presentStudent(userId: 'student-1')];
    final controller = SessionRoomController(api,
        () async => (token: 'token', sessionId: 'session'), FakeMediaSession(),
        realtime: realtime);
    addTearDown(controller.dispose);
    await controller.join('live-session-1');

    realtime.emit(HandRaisedEvent(
        sessionId: 'live-session-1',
        participantId: 'student-1',
        participantName: 'عمر عبدالله',
        at: DateTime.utc(2026, 1, 1, 10)));
    expect(controller.state.participants.single.isHandRaised, isTrue);
    expect(controller.state.participants.single.handRaisedAt,
        DateTime.utc(2026, 1, 1, 10));

    // Duplicate delivery of the same raise must not move state.
    realtime.emit(HandRaisedEvent(
        sessionId: 'live-session-1',
        participantId: 'student-1',
        participantName: 'عمر عبدالله',
        at: DateTime.utc(2026, 1, 1, 11)));
    expect(controller.state.participants.single.handRaisedAt,
        DateTime.utc(2026, 1, 1, 10));

    realtime.emit(HandLoweredEvent(
        sessionId: 'live-session-1',
        participantId: 'student-1',
        participantName: 'عمر عبدالله',
        at: DateTime.utc(2026, 1, 1, 12)));
    expect(controller.state.participants.single.isHandRaised, isFalse);

    realtime.emit(ParticipantJoinedEvent(
        sessionId: 'live-session-1',
        userId: 'student-2',
        displayName: 'سارة',
        role: CircleRole.student));
    expect(controller.state.participants.length, 2);

    realtime.emit(
        ParticipantLeftEvent(sessionId: 'live-session-1', userId: 'student-1'));
    final left = controller.state.participants
        .singleWhere((p) => p.userId == 'student-1');
    expect(left.isCurrentlyPresent, isFalse);

    realtime.emit(LockChangedEvent(sessionId: 'live-session-1', locked: true));
    expect(controller.state.isLocked, isTrue);

    realtime.emit(
        SessionEndedEvent(sessionId: 'live-session-1', endReason: 'manual'));
    expect(controller.state.status, SessionRoomStatus.ended);
  });

  test('realtime snapshots replace participants authoritatively', () async {
    final realtime = FakeRealtimeClient();
    final controller = SessionRoomController(FakeSessionApi(),
        () async => (token: 'token', sessionId: 'session'), FakeMediaSession(),
        realtime: realtime);
    addTearDown(controller.dispose);
    await controller.join('live-session-1');

    realtime.emit(SessionSnapshotEvent(
        sessionId: 'live-session-1',
        isLocked: true,
        participants: [_presentStudent(userId: 'student-9')]));

    expect(controller.state.participants.single.userId, 'student-9');
    expect(controller.state.isLocked, isTrue);
  });

  test('ignores hand events for unknown participants', () async {
    final realtime = FakeRealtimeClient();
    final api = FakeSessionApi()
      ..participantsList = [_presentStudent(userId: 'student-1')];
    final controller = SessionRoomController(api,
        () async => (token: 'token', sessionId: 'session'), FakeMediaSession(),
        realtime: realtime);
    addTearDown(controller.dispose);
    await controller.join('live-session-1');

    realtime.emit(HandRaisedEvent(
        sessionId: 'live-session-1',
        participantId: 'unknown-1',
        participantName: 'مجهول',
        at: DateTime.utc(2026, 1, 1)));

    expect(controller.state.participants.single.isHandRaised, isFalse);
  });

  test('raise and lower hand send realtime commands', () async {
    final realtime = FakeRealtimeClient();
    final controller = SessionRoomController(FakeSessionApi(),
        () async => (token: 'token', sessionId: 'session'), FakeMediaSession(),
        realtime: realtime);
    addTearDown(controller.dispose);
    await controller.join('live-session-1');

    await controller.raiseHand();
    await controller.lowerHand();

    expect(realtime.raiseCalls, 1);
    expect(realtime.lowerCalls, 1);
  });

  test('moderator moderation actions call the API and update state', () async {
    final api = FakeSessionApi();
    final controller = SessionRoomController(api,
        () async => (token: 'token', sessionId: 'session'), FakeMediaSession(),
        realtime: FakeRealtimeClient(), isModerator: true);
    addTearDown(controller.dispose);
    await controller.join('live-session-1');

    await controller.setLock(true);
    expect(api.lockCalls, 1);
    expect(controller.state.isLocked, isTrue);

    await controller.muteAll();
    expect(api.muteAllCalls, 1);

    await controller.muteParticipant('student-1');
    expect(api.mutedUserIds, ['student-1']);

    await controller.unmuteParticipant('student-1');
    expect(api.unmutedUserIds, ['student-1']);

    await controller.removeParticipant('student-1');
    expect(api.removedUserIds, ['student-1']);

    await controller.endSession();
    expect(api.endCalls, 1);
    expect(controller.state.status, SessionRoomStatus.ended);
  });

  test('uses the server moderator decision on the production connection path',
      () async {
    final api = FakeSessionApi()..isModerator = true;
    final controller = SessionRoomController(api,
        () async => (token: 'token', sessionId: 'session'), FakeMediaSession(),
        realtime: FakeRealtimeClient());
    addTearDown(controller.dispose);
    await controller.join('live-session-1');

    expect(controller.state.isModerator, isTrue);
  });

  test('member moderation actions are denied without calling the API',
      () async {
    final api = FakeSessionApi();
    final controller = SessionRoomController(api,
        () async => (token: 'token', sessionId: 'session'), FakeMediaSession(),
        realtime: FakeRealtimeClient(), isModerator: false);
    addTearDown(controller.dispose);
    await controller.join('live-session-1');

    await controller.setLock(true);
    await controller.muteAll();
    await controller.muteParticipant('student-1');
    await controller.unmuteParticipant('student-1');
    await controller.removeParticipant('student-1');
    await controller.endSession();

    expect(api.lockCalls, 0);
    expect(api.muteAllCalls, 0);
    expect(api.mutedUserIds, isEmpty);
    expect(api.unmutedUserIds, isEmpty);
    expect(api.removedUserIds, isEmpty);
    expect(api.endCalls, 0);
    expect(controller.state.status, SessionRoomStatus.connected);
  });

  test('moderation failure surfaces an action error without leaving the room',
      () async {
    final api = FakeSessionApi()..failLock = true;
    final controller = SessionRoomController(api,
        () async => (token: 'token', sessionId: 'session'), FakeMediaSession(),
        realtime: FakeRealtimeClient(), isModerator: true);
    addTearDown(controller.dispose);
    await controller.join('live-session-1');

    await controller.setLock(true);

    expect(controller.state.status, SessionRoomStatus.connected);
    expect(controller.state.actionErrorMessage, contains('lock failed'));
  });
}

class FakeMediaSession implements MediaSession {
  int connections = 0;
  MediaConnection? lastConnection;

  @override
  Future<void> connect(MediaConnection connection) async {
    connections++;
    lastConnection = connection;
  }

  @override
  Future<void> disconnect() async {}

  @override
  Future<void> setMicrophoneEnabled(bool enabled) async {}
}

class FakeRealtimeClient implements RealtimeSessionClient {
  // Sync delivery so event application is observable right after emit().
  final StreamController<RealtimeSessionEvent> _events =
      StreamController<RealtimeSessionEvent>(sync: true);
  int raiseCalls = 0;
  int lowerCalls = 0;

  void emit(RealtimeSessionEvent event) => _events.add(event);

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

class FakeSessionApi extends SessionApiClient {
  FakeSessionApi() : super(Dio());
  bool isModerator = false;
  String? lastToken;
  String? lastSessionId;
  List<SessionParticipant> participantsList = [
    SessionParticipant(
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
  int lockCalls = 0;
  int endCalls = 0;
  int muteAllCalls = 0;
  bool failLock = false;
  final List<String> mutedUserIds = [];
  final List<String> unmutedUserIds = [];
  final List<String> removedUserIds = [];

  @override
  Future<SessionConnection> start(
      {required String token,
      required String sessionId,
      required String liveSessionId}) async {
    lastToken = token;
    lastSessionId = sessionId;
    return _connection(isModerator: isModerator);
  }

  @override
  Future<SessionConnection> join(
          {required String token,
          required String sessionId,
          required String liveSessionId}) async =>
      _connection(isModerator: isModerator);

  @override
  Future<List<SessionParticipant>> participants(
          {required String token,
          required String sessionId,
          required String liveSessionId}) async =>
      participantsList;

  @override
  Future<SessionModel> setLock(
      {required String token,
      required String sessionId,
      required String liveSessionId,
      required bool locked}) async {
    lockCalls++;
    if (failLock) throw StateError('lock failed');
    return _session(locked: locked);
  }

  @override
  Future<SessionModel> end(
      {required String token,
      required String sessionId,
      required String liveSessionId}) async {
    endCalls++;
    return _session(status: 'ended', locked: false);
  }

  @override
  Future<void> muteAll(
      {required String token,
      required String sessionId,
      required String liveSessionId}) async {
    muteAllCalls++;
  }

  @override
  Future<void> muteParticipant(
      {required String token,
      required String sessionId,
      required String liveSessionId,
      required String userId}) async {
    mutedUserIds.add(userId);
  }

  @override
  Future<void> unmuteParticipant(
      {required String token,
      required String sessionId,
      required String liveSessionId,
      required String userId}) async {
    unmutedUserIds.add(userId);
  }

  @override
  Future<void> removeParticipant(
      {required String token,
      required String sessionId,
      required String liveSessionId,
      required String userId}) async {
    removedUserIds.add(userId);
  }
}

class FailingSessionApi extends SessionApiClient {
  FailingSessionApi() : super(Dio());

  @override
  Future<SessionConnection> join(
          {required String token,
          required String sessionId,
          required String liveSessionId}) =>
      Future.error(StateError('join failed'));
}

SessionParticipant _presentStudent({required String userId}) =>
    SessionParticipant(
        userId: userId,
        displayName: 'عمر عبدالله',
        role: CircleRole.student,
        isCurrentlyPresent: true);

SessionConnection _connection({bool isModerator = false}) => SessionConnection(
      session: _session(),
      isModerator: isModerator,
      mediaConnection: MediaConnection(
          endpoint: 'wss://media.example',
          credential: 'short-lived-credential',
          expiresAt: DateTime.now().add(const Duration(minutes: 1))),
    );

SessionModel _session({String status = 'active', bool locked = false}) =>
    SessionModel(
        id: 'live-session-1',
        circleId: 'circle-1',
        status: status,
        mediaMode: 'audio',
        participantCount: 2,
        isLocked: locked);
