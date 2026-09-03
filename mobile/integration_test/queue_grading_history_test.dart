import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:integration_test/integration_test.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('T062: complete, correct, reset, and end a graded round',
      (tester) async {
    final env = Platform.environment;
    final teacher = _Credentials.fromEnv(env, 'TEACHER');
    final student = _Credentials.fromEnv(env, 'STUDENT');
    if (teacher == null || student == null) {
      markTestSkipped(
        'T062_* env vars missing; real-backend integration needs '
        'pre-provisioned Firebase tokens and backend sessions.',
      );
      return;
    }

    final dio = Dio(BaseOptions(
      baseUrl:
          env['T062_API_BASE_URL'] ?? 'http://host.docker.internal:8080/api/v1',
    ));
    addTearDown(dio.close);
    final circles = CircleApiClient(dio);
    final sessions = SessionApiClient(dio);
    final queue = QueueApiClient(dio);

    final circle = await circles.createCircle(
      firebaseIdToken: teacher.token,
      sessionId: teacher.sessionId,
      request: const CreateCircleRequest(
        name: 'T062-graded-round',
        language: 'ar',
        maxCapacity: 10,
      ),
    );
    addTearDown(() async {
      try {
        await circles.archiveCircle(
          firebaseIdToken: teacher.token,
          sessionId: teacher.sessionId,
          circleId: circle.id,
        );
      } on DioException {
        // Best-effort cleanup.
      }
    });
    await circles.joinCircleByInvite(
      firebaseIdToken: student.token,
      sessionId: student.sessionId,
      inviteCode: circle.inviteCode,
    );

    final session = await sessions.create(
      token: teacher.token,
      sessionId: teacher.sessionId,
      circleId: circle.id,
    );
    addTearDown(() async {
      try {
        await sessions.end(
          token: teacher.token,
          sessionId: teacher.sessionId,
          liveSessionId: session.id,
        );
      } on DioException {
        // Best-effort cleanup.
      }
    });
    await sessions.start(
      token: teacher.token,
      sessionId: teacher.sessionId,
      liveSessionId: session.id,
    );
    await sessions.join(
      token: teacher.token,
      sessionId: teacher.sessionId,
      liveSessionId: session.id,
    );
    await sessions.join(
      token: student.token,
      sessionId: student.sessionId,
      liveSessionId: session.id,
    );

    var state = await queue.prepareRound(
      token: teacher.token,
      sessionId: teacher.sessionId,
      liveSessionId: session.id,
      roundType: 'revision',
      surahId: 1,
      fromAyah: 1,
      toAyah: 7,
      gradingRequired: true,
    );
    expect(state.entries, isNotEmpty);
    final entry = state.entries.first;

    state = await queue.advance(
      token: teacher.token,
      sessionId: teacher.sessionId,
      liveSessionId: session.id,
      expectedVersion: state.version,
    );
    final selected = state.entries.firstWhere(
      (candidate) => candidate.id == state.selectedEntryId,
      orElse: () => entry,
    );
    state = await queue.updateEntryStatus(
      token: teacher.token,
      sessionId: teacher.sessionId,
      liveSessionId: session.id,
      entryId: selected.id,
      status: 'reciting',
      expectedEntryVersion: selected.version,
    );
    final reciting =
        state.entries.firstWhere((candidate) => candidate.id == selected.id);

    state = await queue.completeEntry(
      token: teacher.token,
      sessionId: teacher.sessionId,
      liveSessionId: session.id,
      entryId: reciting.id,
      expectedEntryVersion: reciting.version,
      grade: 'good',
      notes: 'T062 initial grade',
    );
    final completed =
        state.entries.firstWhere((candidate) => candidate.id == reciting.id);
    expect(completed.status, 'completed');
    expect(completed.grade, 'good');
    expect(completed.gradeNotes, 'T062 initial grade');

    final corrected = await queue.correctEntry(
      token: teacher.token,
      sessionId: teacher.sessionId,
      liveSessionId: session.id,
      entryId: completed.id,
      expectedEntryVersion: completed.version,
      notes: 'T062 corrected note',
      includeNotes: true,
    );
    expect(corrected.grade, 'good');
    expect(corrected.gradeNotes, 'T062 corrected note');

    final reset = await queue.reset(
      token: teacher.token,
      sessionId: teacher.sessionId,
      liveSessionId: session.id,
      roundType: 'revision',
      surahId: 1,
      fromAyah: 1,
      toAyah: 7,
      gradingRequired: true,
      expectedVersion: state.version,
    );
    expect(reset.roundId, isNot(state.roundId));
    expect(reset.entries, isNotEmpty);

    // Forced queue failure: a deliberately stale expectedVersion must be
    // rejected with the contract's 409 version conflict, and the session end
    // below must still succeed. reset.version - 1 (never 0 — versions below 1
    // are rejected as 422 validation) is always stale because a reset round
    // is born at version 1 and bumps to >= 2 on activation.
    Object? forcedQueueFailure;
    try {
      await queue.advance(
        token: teacher.token,
        sessionId: teacher.sessionId,
        liveSessionId: session.id,
        expectedVersion: reset.version - 1,
      );
    } on QueueApiException catch (error) {
      forcedQueueFailure = error;
    }
    // QueueApiClient wraps transport errors as QueueApiException, so the
    // forced failure is asserted on the wrapped status/code contract.
    final conflict = forcedQueueFailure as QueueApiException?;
    expect(
      conflict,
      isNotNull,
      reason: 'a stale expectedVersion must be rejected by the queue',
    );
    expect(
      conflict!.statusCode,
      409,
      reason: 'docs/contracts/openapi.yaml maps a stale expected_version to '
          '409 Conflict',
    );
    expect(conflict.code, 'ERR_QUEUE_VERSION_CONFLICT');

    final ended = await sessions.end(
      token: teacher.token,
      sessionId: teacher.sessionId,
      liveSessionId: session.id,
    );
    expect(ended.status, anyOf('ended', 'completed'));
    final terminal = await queue.getQueue(
      token: teacher.token,
      sessionId: teacher.sessionId,
      liveSessionId: session.id,
    );
    // The contract's read-only terminal surface: the latest round stays
    // readable with lifecycle 'finalized' (never 404) after session end.
    expect(terminal.lifecycle, 'finalized');
  });
}

class _Credentials {
  const _Credentials(this.token, this.sessionId);

  final String token;
  final String sessionId;

  static _Credentials? fromEnv(Map<String, String> env, String role) {
    final token = env['T062_${role}_TOKEN'];
    final sessionId = env['T062_${role}_SESSION'];
    if (token == null || sessionId == null) return null;
    return _Credentials(token, sessionId);
  }
}
