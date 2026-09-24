import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:halaqaty_mobile/core/theme/halaqaty_theme.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_detail_screen.dart';
import 'package:halaqaty_mobile/features/sessions/application/circle_sessions_controller.dart';
import 'package:halaqaty_mobile/features/sessions/application/media_session.dart';
import 'package:halaqaty_mobile/features/sessions/application/queue_controller.dart';
import 'package:halaqaty_mobile/features/sessions/application/session_room_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/realtime_session_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_room_screen.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_ui_labels.dart';

/// Wave 2 (US3) visual evidence harness: renders the circle sessions section
/// and the session room (student and manager) with stubbed providers and
/// captures the screenshot acceptance matrix states in Arabic RTL / English
/// LTR x light / dark. Fakes mirror `queue_flow_test.dart` and
/// `session_discovery_test.dart`; every state gets fresh stubs and the
/// previous tree is detached before the next capture.
const _sessionId = 'session-1';
const _circleId = 'circle-1';

void main() {
  final binding = IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  /// Pumps until [finder] stays visible for 3 consecutive frames.
  Future<void> pumpUntilStable(WidgetTester tester, Finder sentinel) async {
    var stable = 0;
    for (var i = 0; i < 60 && stable < 3; i++) {
      await tester.pump(const Duration(milliseconds: 200));
      stable = sentinel.evaluate().isNotEmpty ? stable + 1 : 0;
    }
    if (stable < 3) throw StateError('Sentinel never stabilized: $sentinel');
  }

  Future<void> pumpScreen(
    WidgetTester tester,
    Widget screen,
    Locale locale,
    Brightness brightness,
    List<Override> overrides,
  ) async {
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pump();
    await tester.pumpWidget(
      ProviderScope(
        overrides: overrides,
        child: MaterialApp(
          debugShowCheckedModeBanner: false,
          theme: halaqatyLightTheme(),
          darkTheme: halaqatyDarkTheme(),
          themeMode:
              brightness == Brightness.dark ? ThemeMode.dark : ThemeMode.light,
          // Root Directionality via builder (as in `main.dart`) so dialogs,
          // which open above the Navigator, inherit the active direction.
          builder: (context, child) => Directionality(
            textDirection: locale.languageCode == 'ar'
                ? TextDirection.rtl
                : TextDirection.ltr,
            child: child!,
          ),
          home: screen,
        ),
      ),
    );
  }

  Future<void> capture(
    WidgetTester tester,
    String state,
    Widget screen,
    Locale locale,
    Brightness brightness,
    List<Override> overrides,
    Finder sentinel, {
    Future<void> Function()? prepare,
  }) async {
    await pumpScreen(tester, screen, locale, brightness, overrides);
    if (prepare != null) await prepare();
    await pumpUntilStable(tester, sentinel);
    await binding.takeScreenshot(
      'wave2_${state}_${locale.languageCode}_'
      '${brightness == Brightness.dark ? 'dark' : 'light'}',
    );
  }

  List<Override> circleOverrides(
    Locale locale, {
    required CircleRole role,
    required String userId,
    required _FakeCircleSessions sessions,
  }) =>
      [
        circleDetailProvider(_circleId).overrideWith(
          (_) => Future.value(_circle(locale)),
        ),
        circleMembersProvider(_circleId).overrideWith(
          (_) => Future.value([
            CircleMember(
              userId: userId,
              displayName: role == CircleRole.student
                  ? _studentAName(locale)
                  : _managerName(locale),
              role: role,
              joinedAt: DateTime.utc(2026, 1, 1),
            ),
          ]),
        ),
        circleSessionsControllerProvider(_circleId)
            .overrideWith((_) => sessions),
      ];

  for (final locale in const [Locale('ar'), Locale('en')]) {
    testWidgets('wave2_sessions_matrix_${locale.languageCode}', (tester) async {
      await binding.convertFlutterSurfaceToImage();
      final rtl = locale.languageCode == 'ar';
      for (final brightness in Brightness.values) {
        // --- Circle sessions section (circle detail) -----------------------
        Future<void> scrollToSessions() async {
          // Let the async detail providers resolve so the ListView exists
          // before scrolling to the lazily-built sessions section.
          await pumpUntilStable(
            tester,
            find.text(locale.languageCode == 'ar'
                ? 'حلقة الإتقان'
                : 'Mastery Circle'),
          );
          await tester.scrollUntilVisible(
            find.byKey(const Key('circleSessionsSection')),
            200,
          );
        }

        await capture(
          tester,
          'circle_sessions_empty_member',
          const CircleDetailScreen(
              circleId: _circleId, currentUserId: 'user-student-a'),
          locale,
          brightness,
          circleOverrides(
            locale,
            role: CircleRole.student,
            userId: 'user-student-a',
            sessions: _FakeCircleSessions(const [
              CircleSessionsState(status: CircleSessionsStatus.ready)
            ]),
          ),
          find.byKey(const Key('circleSessionsEmpty')),
          prepare: scrollToSessions,
        );

        await capture(
          tester,
          'circle_sessions_error',
          const CircleDetailScreen(
              circleId: _circleId, currentUserId: 'user-student-a'),
          locale,
          brightness,
          circleOverrides(
            locale,
            role: CircleRole.student,
            userId: 'user-student-a',
            sessions: _FakeCircleSessions(const [
              CircleSessionsState(
                status: CircleSessionsStatus.error,
                failure: CircleSessionsFailure.unknown,
              ),
            ]),
          ),
          find.byKey(const Key('circleSessionsError')),
          prepare: scrollToSessions,
        );

        await capture(
          tester,
          'circle_sessions_loaded_manager',
          const CircleDetailScreen(
              circleId: _circleId, currentUserId: 'user-manager'),
          locale,
          brightness,
          circleOverrides(
            locale,
            role: CircleRole.teacher,
            userId: 'user-manager',
            sessions: _FakeCircleSessions([
              CircleSessionsState(
                status: CircleSessionsStatus.ready,
                sessions: [
                  _sessionTile('session-scheduled', 'scheduled', 0),
                  _sessionTile('session-active', 'active', 5),
                ],
              ),
            ]),
          ),
          find.byKey(const Key('circleSessionCreate')),
          prepare: scrollToSessions,
        );

        // --- Student room ---------------------------------------------------
        final studentEntries = [
          _entryJson(
              'entry-1', 'user-student-b', _studentBName(locale), 1, 'waiting'),
          _entryJson(
              'entry-2', 'user-student-a', _studentAName(locale), 2, 'waiting'),
        ];

        final studentConnected = await _RoomFixture.connected(
          snapshot: _queueSnapshot(entries: studentEntries),
          currentUserId: 'user-student-a',
          participants: _participants(locale),
        );
        await capture(
          tester,
          'student_connected',
          const SessionRoomScreen(sessionId: _sessionId),
          locale,
          brightness,
          [_roomOverride(studentConnected)],
          find.text(
              rtl ? SessionUiLabels.connected : 'Connected. Audio is ready.'),
        );

        final studentTurn = await _RoomFixture.connected(
          snapshot: _queueSnapshot(
            selectedEntryId: 'entry-1',
            entries: [
              _entryJson('entry-1', 'user-student-a', _studentAName(locale), 1,
                  'reciting'),
              _entryJson('entry-2', 'user-student-b', _studentBName(locale), 2,
                  'waiting'),
            ],
          ),
          currentUserId: 'user-student-a',
          participants: _participants(locale),
        );
        await capture(
          tester,
          'student_current_turn',
          const SessionRoomScreen(sessionId: _sessionId),
          locale,
          brightness,
          [_roomOverride(studentTurn)],
          find.text(rtl ? SessionUiLabels.reciting : 'Reciting'),
        );

        final studentOptOut = await _RoomFixture.connected(
          snapshot: _queueSnapshot(entries: studentEntries),
          currentUserId: 'user-student-a',
          participants: _participants(locale),
          optOutEntryJson: studentEntries[1],
        );
        await studentOptOut.queue!.requestOptOut();
        await capture(
          tester,
          'student_optout_pending',
          const SessionRoomScreen(sessionId: _sessionId),
          locale,
          brightness,
          [_roomOverride(studentOptOut)],
          find.text(rtl
              ? SessionUiLabels.optOutPending
              : 'Awaiting teacher approval'),
        );

        final studentReconnect = await _RoomFixture.connected(
          snapshot: _queueSnapshot(entries: studentEntries),
          currentUserId: 'user-student-a',
          participants: _participants(locale),
        );
        studentReconnect.api.hangJoin = true;
        unawaited(studentReconnect.room.retry());
        await capture(
          tester,
          'student_reconnecting',
          const SessionRoomScreen(sessionId: _sessionId),
          locale,
          brightness,
          [_roomOverride(studentReconnect)],
          find.text(rtl
              ? SessionUiLabels.queueReconnecting
              : 'Reconnecting to queue...'),
        );

        final terminalApi = _VisualSessionApi(terminal: true);
        final terminalRoom = SessionRoomController(
          terminalApi,
          _credentials,
          _VisualMedia(),
          realtime: _VisualRealtime(),
          currentUserId: 'user-student-a',
        );
        await terminalRoom.join(_sessionId);
        await capture(
          tester,
          'student_terminal',
          const SessionRoomScreen(sessionId: _sessionId),
          locale,
          brightness,
          [
            sessionRoomControllerProvider(_sessionId)
                .overrideWith((_) => terminalRoom),
          ],
          find.byKey(const Key('sessionRoomLeave')),
        );

        // --- Manager room ---------------------------------------------------
        final managerEntries = [
          _entryJson(
              'entry-1', 'user-student-a', _studentAName(locale), 1, 'waiting'),
          _entryJson(
              'entry-2', 'user-student-b', _studentBName(locale), 2, 'waiting'),
        ];

        final managerReady = await _RoomFixture.connected(
          manager: true,
          snapshot: _queueSnapshot(entries: managerEntries),
          participants: _participants(locale),
        );
        await capture(
          tester,
          'manager_queue_ready',
          const SessionRoomScreen(sessionId: _sessionId, canStart: true),
          locale,
          brightness,
          [_roomOverride(managerReady)],
          find.text(rtl ? SessionUiLabels.queueTitle : 'Recitation queue'),
        );

        final managerGrading = await _RoomFixture.connected(
          manager: true,
          snapshot: _queueSnapshot(
            gradingRequired: true,
            selectedEntryId: 'entry-1',
            entries: [
              _entryJson('entry-1', 'user-student-a', _studentAName(locale), 1,
                  'reciting'),
              _entryJson('entry-2', 'user-student-b', _studentBName(locale), 2,
                  'waiting'),
            ],
          ),
          participants: _participants(locale),
        );
        final gradingTitle =
            find.text(rtl ? 'تقييم التلاوة' : 'Grade recitation');
        await capture(
          tester,
          'manager_grading',
          const SessionRoomScreen(sessionId: _sessionId, canStart: true),
          locale,
          brightness,
          [_roomOverride(managerGrading)],
          gradingTitle,
          prepare: () => tester.ensureVisible(gradingTitle),
        );

        final managerModeration = await _RoomFixture.connected(
          manager: true,
          snapshot: _queueSnapshot(entries: managerEntries),
          participants: _participants(locale),
        );
        const endSessionKey = Key('sessionRoomEndSession');
        // Anchor the scroll at the participants header: centering the
        // end-session button leaves the top moderation row half-clipped.
        final participantsHeader =
            find.text(rtl ? SessionUiLabels.participantsTitle : 'Participants');
        await capture(
          tester,
          'manager_moderation',
          const SessionRoomScreen(sessionId: _sessionId, canStart: true),
          locale,
          brightness,
          [_roomOverride(managerModeration)],
          find.byKey(endSessionKey),
          prepare: () => tester.ensureVisible(participantsHeader),
        );

        final managerDialog = await _RoomFixture.connected(
          manager: true,
          snapshot: _queueSnapshot(entries: managerEntries),
          participants: _participants(locale),
        );
        final resetLabel = rtl ? SessionUiLabels.resetQueue : 'Reset round';
        await capture(
          tester,
          'dialog_reset_confirm',
          const SessionRoomScreen(sessionId: _sessionId, canStart: true),
          locale,
          brightness,
          [_roomOverride(managerDialog)],
          find.bySemanticsLabel(rtl ? SessionUiLabels.confirm : 'Confirm'),
          prepare: () async {
            await tester.ensureVisible(find.text(resetLabel));
            await tester.pump();
            await tester.tap(find.text(resetLabel));
            await tester.pump(const Duration(milliseconds: 400));
          },
        );
      }
    });
  }
}

Override _roomOverride(_RoomFixture fixture) =>
    sessionRoomControllerProvider(_sessionId).overrideWith((_) => fixture.room);

Future<({String token, String sessionId})> _credentials() async =>
    (token: 'token', sessionId: 'backend-session');

String _managerName(Locale locale) =>
    locale.languageCode == 'ar' ? 'المعلمة مريم' : 'Maryam';
String _studentAName(Locale locale) =>
    locale.languageCode == 'ar' ? 'أحمد' : 'Ahmed';
String _studentBName(Locale locale) =>
    locale.languageCode == 'ar' ? 'فاطمة' : 'Fatimah';

List<SessionParticipant> _participants(Locale locale) => [
      SessionParticipant(
        userId: 'user-manager',
        displayName: _managerName(locale),
        role: CircleRole.teacher,
        isCurrentlyPresent: true,
      ),
      SessionParticipant(
        userId: 'user-student-b',
        displayName: _studentBName(locale),
        role: CircleRole.student,
        isCurrentlyPresent: true,
        handRaisedAt: DateTime.utc(2026, 9, 1, 12),
      ),
      SessionParticipant(
        userId: 'user-student-a',
        displayName: _studentAName(locale),
        role: CircleRole.student,
        isCurrentlyPresent: true,
      ),
    ];

CircleResponse _circle(Locale locale) => CircleResponse(
      id: _circleId,
      name: locale.languageCode == 'ar' ? 'حلقة الإتقان' : 'Mastery Circle',
      description: locale.languageCode == 'ar'
          ? 'تلاوة وحفظ'
          : 'Recitation and memorization',
      inviteCode: 'HLQ-7X2K',
      inviteLink: 'https://halaqaty.app/join/HLQ-7X2K',
      maxCapacity: 20,
      language: locale.languageCode,
      createdAt: DateTime.utc(2026, 1, 1),
    );

SessionModel _sessionTile(String id, String status, int participantCount) =>
    SessionModel(
      id: id,
      circleId: _circleId,
      status: status,
      mediaMode: 'audio',
      participantCount: participantCount,
      isLocked: false,
    );

Map<String, dynamic> _entryJson(
  String id,
  String studentId,
  String studentName,
  int position,
  String status,
) =>
    {
      'id': id,
      'student_id': studentId,
      'student_name': studentName,
      'position': position,
      'status': status,
      'grade': null,
      'grade_notes': null,
      'version': 1,
    };

QueueState _queueSnapshot({
  String? selectedEntryId,
  bool gradingRequired = false,
  required List<Map<String, dynamic>> entries,
}) =>
    QueueState.fromJson({
      'session_id': _sessionId,
      'round_id': 'round-1',
      'round_number': 1,
      'round_type': 'revision',
      'lifecycle': 'active',
      'surah_id': 2,
      'from_ayah': 1,
      'to_ayah': 5,
      'grading_required': gradingRequired,
      'selected_entry_id': selectedEntryId,
      'version': 1,
      'policy': {
        'population': 'present_at_activation',
        'unfinished_finalization': 'mark_unfinished_skipped',
        'opt_out': 'approval_required',
        'grade_visibility': 'managers_and_student',
        'grade_correction': 'audited_any_time',
        'version': 1,
      },
      'preorder': const [],
      'entries': entries,
    });

class _RoomFixture {
  const _RoomFixture(
      {required this.room, required this.queue, required this.api});

  final SessionRoomController room;
  final QueueController? queue;
  final _VisualSessionApi api;

  static Future<_RoomFixture> connected({
    required QueueState snapshot,
    bool manager = false,
    String? currentUserId,
    List<SessionParticipant> participants = const [],
    Map<String, dynamic>? optOutEntryJson,
  }) async {
    final queueApi =
        _VisualQueueApi(snapshot, optOutEntryJson: optOutEntryJson);
    final queue = QueueController(
      queueApi,
      _credentials,
      realtime: _VisualRealtime(),
      isManager: manager,
    );
    final api =
        _VisualSessionApi(participantsList: participants, isModerator: manager);
    final room = SessionRoomController(
      api,
      _credentials,
      _VisualMedia(),
      realtime: _VisualRealtime(),
      isModerator: manager,
      queue: queue,
      currentUserId: currentUserId,
    );
    await room.join(_sessionId);
    if (room.state.status != SessionRoomStatus.connected) {
      throw StateError('Fixture room failed to connect: ${room.state.status}');
    }
    return _RoomFixture(room: room, queue: queue, api: api);
  }
}

class _VisualSessionApi extends SessionApiClient {
  _VisualSessionApi({
    this.participantsList = const [],
    this.isModerator = false,
    this.terminal = false,
  }) : super(Dio());

  final List<SessionParticipant> participantsList;
  final bool isModerator;
  final bool terminal;

  /// When true the next join/start never completes, holding the room in its
  /// reconnecting state for the capture.
  bool hangJoin = false;

  @override
  Future<SessionConnection> join({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) =>
      _connect();

  @override
  Future<SessionConnection> start({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) =>
      _connect();

  Future<SessionConnection> _connect() {
    if (hangJoin) return Completer<SessionConnection>().future;
    if (terminal) {
      return Future.error(StateError('session access ended: 403'));
    }
    return Future.value(SessionConnection(
      session: const SessionModel(
        id: _sessionId,
        circleId: _circleId,
        status: 'active',
        mediaMode: 'audio',
        participantCount: 3,
        isLocked: false,
      ),
      mediaConnection: MediaConnection(
        endpoint: 'wss://media.example',
        credential: 'credential',
        expiresAt: DateTime.utc(2026, 9, 1),
      ),
      isModerator: isModerator,
    ));
  }

  @override
  Future<List<SessionParticipant>> participants({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      participantsList;

  @override
  Future<SessionModel> end({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      const SessionModel(
        id: _sessionId,
        circleId: _circleId,
        status: 'ended',
        mediaMode: 'audio',
        participantCount: 0,
        isLocked: false,
      );
}

class _VisualQueueApi extends QueueApiClient {
  _VisualQueueApi(this._snapshot, {this.optOutEntryJson}) : super(Dio());

  final QueueState _snapshot;
  final Map<String, dynamic>? optOutEntryJson;

  @override
  Future<QueueState> getQueue({
    required String token,
    required String sessionId,
    required String liveSessionId,
  }) async =>
      _snapshot;

  @override
  Future<OptOutResult> optOut({
    required String token,
    required String sessionId,
    required String liveSessionId,
    String? idempotencyKey,
  }) async {
    final entryJson = optOutEntryJson!;
    return OptOutResult(
      request: OptOutRequest(
        id: 'optout-1',
        queueEntryId: entryJson['id'] as String,
        status: 'pending',
        requestedAt: DateTime.utc(2026, 9, 1),
      ),
      entry: QueueEntry.fromJson(entryJson),
    );
  }
}

class _VisualMedia implements MediaSession {
  @override
  Future<void> connect(MediaConnection connection) async {}

  @override
  Future<void> disconnect() async {}

  @override
  Future<void> setMicrophoneEnabled(bool enabled) async {}
}

class _VisualRealtime implements RealtimeSessionClient {
  // An open stream: a completed stream would trip the room's reconnect path.
  final StreamController<RealtimeSessionEvent> _events =
      StreamController<RealtimeSessionEvent>.broadcast();

  @override
  Stream<RealtimeSessionEvent> sessionEvents(
    String liveSessionId, {
    required String token,
    required String backendSessionId,
  }) =>
      _events.stream;

  @override
  Future<void> raiseHand(String liveSessionId) async {}

  @override
  Future<void> lowerHand(String liveSessionId) async {}

  @override
  Future<void> dispose() => _events.close();
}

class _FakeCircleSessions extends CircleSessionsController {
  _FakeCircleSessions(List<CircleSessionsState> states)
      : _states = List.of(states),
        super(
          SessionApiClient(Dio()),
          _credentials,
          circleId: _circleId,
        );

  final List<CircleSessionsState> _states;

  @override
  Future<void> load() async {
    if (_states.isNotEmpty) {
      state = _states.removeAt(0);
    }
  }
}
