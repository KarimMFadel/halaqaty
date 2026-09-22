import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_detail_screen.dart';
import 'package:halaqaty_mobile/features/sessions/application/circle_sessions_controller.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/application/session_room_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_room_screen.dart';

void main() {
  test('session-card discovery reuses the canonical circle sessions path',
      () async {
    final requests = <RequestOptions>[];
    final dio = Dio()..httpClientAdapter = _DiscoveryAdapter(requests);
    final client = SessionApiClient(dio);

    final sessions = await client.list(
      token: 'firebase-token',
      sessionId: 'backend-session',
      circleId: 'circle-1',
    );

    expect(requests.single.path, '/circles/circle-1/sessions');
    expect(requests.single.method, 'GET');
    expect(requests.single.headers['Authorization'], 'Bearer firebase-token');
    expect(sessions.single.status, 'scheduled');
  });

  test('session create reuses the canonical circle sessions path', () async {
    final requests = <RequestOptions>[];
    final dio = Dio()..httpClientAdapter = _DiscoveryAdapter(requests);
    final client = SessionApiClient(dio);

    final session = await client.create(
      token: 'firebase-token',
      sessionId: 'backend-session',
      circleId: 'circle-1',
    );

    expect(requests.single.path, '/circles/circle-1/sessions');
    expect(requests.single.method, 'POST');
    expect(session.id, 'session-1');
  });

  group('circle detail session section', () {
    testWidgets(
        'student taps a scheduled session card and lands in the room '
        'with join (single tap from circle detail)', (tester) async {
      final sessions = _FakeCircleSessionsController([
        CircleSessionsState(
          status: CircleSessionsStatus.ready,
          sessions: [_session('session-1', status: 'scheduled')],
        ),
      ]);
      await _pumpDetail(tester, sessions: sessions);
      await _revealSessions(tester);

      expect(find.byKey(const Key('circleSession-session-1')), findsOneWidget);
      expect(find.byKey(const Key('circleSessionCreate')), findsNothing);

      await tester
          .ensureVisible(find.byKey(const Key('circleSession-session-1')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('circleSession-session-1')));
      await tester.pumpAndSettle();

      final room =
          tester.widget<SessionRoomScreen>(find.byType(SessionRoomScreen));
      expect(room.sessionId, 'session-1');
      expect(room.canStart, isFalse);
    });

    testWidgets(
        'preserved room behavior: join from the pushed room still connects',
        (tester) async {
      final sessions = _FakeCircleSessionsController([
        CircleSessionsState(
          status: CircleSessionsStatus.ready,
          sessions: [_session('session-1', status: 'active')],
        ),
      ]);
      await _pumpDetail(tester, sessions: sessions);
      await _revealSessions(tester);

      await tester
          .ensureVisible(find.byKey(const Key('circleSession-session-1')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('circleSession-session-1')));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Join'));
      await tester.pumpAndSettle();

      expect(find.text('Connected. Audio is ready.'), findsOneWidget);
    });

    testWidgets('manager sees create and starts a scheduled session from it',
        (tester) async {
      final sessions = _FakeCircleSessionsController([
        CircleSessionsState(
          status: CircleSessionsStatus.ready,
          sessions: [_session('session-1', status: 'scheduled')],
        ),
      ]);
      await _pumpDetail(
        tester,
        sessions: sessions,
        role: CircleRole.teacher,
        userId: 'teacher-1',
      );
      await _revealSessions(tester);

      expect(find.byKey(const Key('circleSessionCreate')), findsOneWidget);

      await tester
          .ensureVisible(find.byKey(const Key('circleSession-session-1')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('circleSession-session-1')));
      await tester.pumpAndSettle();

      final room =
          tester.widget<SessionRoomScreen>(find.byType(SessionRoomScreen));
      expect(room.sessionId, 'session-1');
      expect(room.canStart, isTrue);
    });

    testWidgets(
        'manager create-then-open: create push lands in the room with start '
        '(Circles → circle → Create → Start)', (tester) async {
      final sessions = _FakeCircleSessionsController(
        [const CircleSessionsState(status: CircleSessionsStatus.ready)],
        created: _session('session-created', status: 'scheduled'),
      );
      await _pumpDetail(
        tester,
        sessions: sessions,
        role: CircleRole.teacher,
        userId: 'teacher-1',
      );
      await _revealSessions(tester);

      await tester.ensureVisible(find.byKey(const Key('circleSessionCreate')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('circleSessionCreate')));
      await tester.pumpAndSettle();

      expect(sessions.createCalls, 1);
      final room =
          tester.widget<SessionRoomScreen>(find.byType(SessionRoomScreen));
      expect(room.sessionId, 'session-created');
      expect(room.canStart, isTrue);
      expect(find.text('Start session'), findsOneWidget);
    });

    testWidgets('create failure surfaces a soft error without navigating',
        (tester) async {
      final sessions = _FakeCircleSessionsController(
        [const CircleSessionsState(status: CircleSessionsStatus.ready)],
        created: null,
      );
      await _pumpDetail(
        tester,
        sessions: sessions,
        role: CircleRole.teacher,
        userId: 'teacher-1',
      );
      await _revealSessions(tester);

      await tester.ensureVisible(find.byKey(const Key('circleSessionCreate')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('circleSessionCreate')));
      await tester.pumpAndSettle();

      expect(sessions.createCalls, 1);
      expect(find.byType(SessionRoomScreen), findsNothing);
      expect(find.byKey(const Key('halaqatyErrorSnackBar')), findsOneWidget);
    });

    testWidgets(
        'empty state shows a manager create hint or a member explanation',
        (tester) async {
      final managerSessions = _FakeCircleSessionsController(
          [const CircleSessionsState(status: CircleSessionsStatus.ready)]);
      await _pumpDetail(
        tester,
        sessions: managerSessions,
        role: CircleRole.teacher,
        userId: 'teacher-1',
      );
      await _revealSessions(tester);

      expect(find.byKey(const Key('circleSessionsEmpty')), findsOneWidget);
      expect(
          find.text('Create a session so students can join'), findsOneWidget);
      expect(find.byKey(const Key('circleSessionCreate')), findsOneWidget);

      await tester.pumpWidget(Container());
      final memberSessions = _FakeCircleSessionsController(
          [const CircleSessionsState(status: CircleSessionsStatus.ready)]);
      await _pumpDetail(tester, sessions: memberSessions);
      await _revealSessions(tester);

      expect(find.byKey(const Key('circleSessionsEmpty')), findsOneWidget);
      expect(
        find.text('Your teacher or supervisor will create a session when '
            'needed'),
        findsOneWidget,
      );
      expect(find.byKey(const Key('circleSessionCreate')), findsNothing);
    });

    testWidgets('shows the branded loading state while listing sessions',
        (tester) async {
      // No queued states: load() only records the call, so the initial
      // loading state stays visible. The branded loader animates forever, so
      // pump frames manually instead of pumpAndSettle.
      final sessions = _FakeCircleSessionsController(const []);
      await _pumpDetail(tester, sessions: sessions, settle: false);
      await tester.pump();
      await tester.scrollUntilVisible(
        find.byKey(const Key('circleSessionsSection')),
        200,
      );

      expect(find.byKey(const Key('circleSessionsLoading')), findsOneWidget);
    });

    testWidgets('retryable load error offers a working retry', (tester) async {
      final sessions = _FakeCircleSessionsController([
        const CircleSessionsState(
          status: CircleSessionsStatus.error,
          failure: CircleSessionsFailure.unknown,
        ),
        CircleSessionsState(
          status: CircleSessionsStatus.ready,
          sessions: [_session('session-1')],
        ),
      ]);
      await _pumpDetail(tester, sessions: sessions);
      await _revealSessions(tester);

      expect(find.byKey(const Key('circleSessionsError')), findsOneWidget);
      expect(find.text('Could not load sessions'), findsOneWidget);
      final retry = find.byKey(const Key('circleSessionsRetry'));
      await tester.ensureVisible(retry);
      await tester.pumpAndSettle();
      expect(retry, findsOneWidget);
      expect(tester.getSize(retry).height, greaterThanOrEqualTo(48));

      await tester.tap(retry);
      await tester.pumpAndSettle();
      await _revealSessions(tester);

      expect(sessions.loadCalls, 2);
      expect(find.byKey(const Key('circleSession-session-1')), findsOneWidget);
    });

    testWidgets('a permission failure explains without a retry affordance',
        (tester) async {
      final sessions = _FakeCircleSessionsController([
        const CircleSessionsState(
          status: CircleSessionsStatus.error,
          failure: CircleSessionsFailure.permission,
        ),
      ]);
      await _pumpDetail(tester, sessions: sessions);
      await _revealSessions(tester);

      expect(
        find.text('You do not have permission to view this circle’s sessions'),
        findsOneWidget,
      );
      expect(find.byKey(const Key('circleSessionsRetry')), findsNothing);
    });

    testWidgets(
        'offline failure retains the last list with actions paused and '
        'explained', (tester) async {
      final sessions = _FakeCircleSessionsController([
        CircleSessionsState(
          status: CircleSessionsStatus.error,
          failure: CircleSessionsFailure.network,
          sessions: [_session('session-1', status: 'active')],
        ),
      ]);
      await _pumpDetail(
        tester,
        sessions: sessions,
        role: CircleRole.teacher,
        userId: 'teacher-1',
      );
      await _revealSessions(tester);

      expect(find.byKey(const Key('circleSession-session-1')), findsOneWidget);
      final tile = tester
          .widget<ListTile>(find.byKey(const Key('circleSession-session-1')));
      expect(tile.enabled, isFalse);
      expect(
        find.textContaining('last loaded sessions'),
        findsOneWidget,
      );
      // Mutations are paused while offline.
      expect(find.byKey(const Key('circleSessionCreate')), findsNothing);

      await tester
          .ensureVisible(find.byKey(const Key('circleSession-session-1')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('circleSession-session-1')),
          warnIfMissed: false);
      await tester.pumpAndSettle();
      expect(find.byType(SessionRoomScreen), findsNothing);
    });

    testWidgets('archived circle keeps sessions read-only', (tester) async {
      final sessions = _FakeCircleSessionsController([
        CircleSessionsState(
          status: CircleSessionsStatus.ready,
          sessions: [_session('session-1', status: 'active')],
        ),
      ]);
      await _pumpDetail(
        tester,
        sessions: sessions,
        role: CircleRole.teacher,
        userId: 'teacher-1',
        archived: true,
      );
      await _revealSessions(tester);

      expect(find.byKey(const Key('circleSessionCreate')), findsNothing);
      final tile = tester
          .widget<ListTile>(find.byKey(const Key('circleSession-session-1')));
      expect(tile.enabled, isFalse);

      await tester
          .ensureVisible(find.byKey(const Key('circleSession-session-1')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('circleSession-session-1')),
          warnIfMissed: false);
      await tester.pumpAndSettle();
      expect(find.byType(SessionRoomScreen), findsNothing);
    });

    testWidgets('renders Arabic-first labels in RTL', (tester) async {
      final sessions = _FakeCircleSessionsController(
        [const CircleSessionsState(status: CircleSessionsStatus.ready)],
        created: _session('session-created', status: 'scheduled'),
      );
      await _pumpDetail(
        tester,
        sessions: sessions,
        role: CircleRole.teacher,
        userId: 'teacher-1',
        direction: TextDirection.rtl,
      );
      await _revealSessions(tester);

      expect(find.text('الجلسات'), findsOneWidget);
      expect(find.text('إنشاء جلسة'), findsOneWidget);
      expect(find.text('لا توجد جلسات حالياً'), findsOneWidget);
    });
  });
}

CircleResponse _circle({bool isArchived = false}) => CircleResponse(
      id: 'circle-1',
      name: 'Circle',
      inviteCode: 'HLQ-7X2K',
      inviteLink: 'https://halaqaty.app/join/HLQ-7X2K',
      isArchived: isArchived,
      createdAt: DateTime.utc(2026, 8, 1),
    );

SessionModel _session(String id, {String status = 'active'}) => SessionModel(
      id: id,
      circleId: 'circle-1',
      status: status,
      mediaMode: 'audio',
      participantCount: 2,
      isLocked: false,
    );

Future<void> _pumpDetail(
  WidgetTester tester, {
  required _FakeCircleSessionsController sessions,
  CircleRole role = CircleRole.student,
  String userId = 'student-1',
  bool archived = false,
  TextDirection direction = TextDirection.ltr,
  bool settle = true,
}) async {
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        circleDetailProvider('circle-1').overrideWith(
          (_) => Future.value(_circle(isArchived: archived)),
        ),
        circleMembersProvider('circle-1').overrideWith(
          (_) => Future.value([
            CircleMember(
              userId: userId,
              displayName: 'Member',
              role: role,
              joinedAt: DateTime.utc(2026, 8, 1),
            ),
          ]),
        ),
        circleSessionsControllerProvider('circle-1')
            .overrideWith((_) => sessions),
        _roomOverride('session-1'),
        _roomOverride('session-created'),
      ],
      child: MaterialApp(
        home: Directionality(
          textDirection: direction,
          child: CircleDetailScreen(
            circleId: 'circle-1',
            currentUserId: userId,
          ),
        ),
      ),
    ),
  );
  if (settle) {
    await tester.pumpAndSettle();
  } else {
    await tester.pump();
  }
}

/// The sessions section is the last block on circle detail; scroll until the
/// lazily-built section enters the viewport before asserting or tapping.
Future<void> _revealSessions(WidgetTester tester) async {
  await tester.scrollUntilVisible(
    find.byKey(const Key('circleSessionsSection')),
    200,
  );
  await tester.pumpAndSettle();
}

Override _roomOverride(String id) =>
    sessionRoomControllerProvider(id).overrideWith(
      (ref) => SessionRoomController(
        _RoomApi(),
        () async => (token: 'token', sessionId: 'backend-session'),
        _NoopMediaSession(),
        realtime: _EmptyRealtimeClient(),
      ),
    );

class _FakeCircleSessionsController extends CircleSessionsController {
  _FakeCircleSessionsController(this._states, {SessionModel? created})
      : _created = created,
        super(
          SessionApiClient(Dio()),
          () async => (token: 'token', sessionId: 'backend-session'),
          circleId: 'circle-1',
        );

  final List<CircleSessionsState> _states;
  final SessionModel? _created;
  int loadCalls = 0;
  int createCalls = 0;

  @override
  Future<void> load() async {
    loadCalls++;
    if (_states.isNotEmpty) {
      state = _states.removeAt(0);
    }
  }

  @override
  Future<SessionModel?> create() async {
    createCalls++;
    return _created;
  }
}

class _RoomApi extends SessionApiClient {
  _RoomApi() : super(Dio());

  @override
  Future<SessionConnection> join({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      _connection();

  @override
  Future<SessionConnection> start({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      _connection();

  @override
  Future<List<SessionParticipant>> participants({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      const [];
}

SessionConnection _connection() => SessionConnection(
      session: _session('session-1', status: 'active'),
      mediaConnection: MediaConnection(
        endpoint: 'wss://media.example',
        credential: 'credential',
        expiresAt: DateTime.now().add(const Duration(minutes: 1)),
      ),
    );

class _NoopMediaSession implements MediaSession {
  @override
  Future<void> connect(MediaConnection connection) async {}
  @override
  Future<void> disconnect() async {}
  @override
  Future<void> setMicrophoneEnabled(bool enabled) async {}
}

class _EmptyRealtimeClient implements RealtimeSessionClient {
  // An open stream: a completed stream would trip the room's reconnect path.
  final StreamController<RealtimeSessionEvent> _events =
      StreamController<RealtimeSessionEvent>.broadcast();

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

class _DiscoveryAdapter implements HttpClientAdapter {
  _DiscoveryAdapter(this.requests);
  final List<RequestOptions> requests;

  @override
  void close({bool force = false}) {}

  @override
  Future<ResponseBody> fetch(RequestOptions options,
      Stream<List<int>>? requestStream, Future? cancelFuture) async {
    requests.add(options);
    final body = options.method == 'POST'
        ? '{"id":"session-1","circle_id":"circle-1","status":"scheduled","media_mode":"audio_only","participant_count":0,"is_locked":false}'
        : '{"data":[{"id":"session-1","circle_id":"circle-1","status":"scheduled","media_mode":"audio_only","participant_count":0,"is_locked":false}]}';
    return ResponseBody.fromString(
      body,
      200,
      headers: <String, List<String>>{
        Headers.contentTypeHeader: <String>['application/json'],
      },
    );
  }
}
