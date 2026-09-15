import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_media_api.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';
import 'package:integration_test/integration_test.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('T064: authorized direct conversation lifecycle', (tester) async {
    final env = Platform.environment;
    final teacher = _Credentials.fromEnv(env, 'TEACHER');
    final student = _Credentials.fromEnv(env, 'STUDENT');
    final supervisor = _Credentials.fromEnv(env, 'SUPERVISOR');
    final operator = _Credentials.fromEnv(env, 'OPERATOR');
    if (teacher == null ||
        student == null ||
        supervisor == null ||
        operator == null) {
      markTestSkipped(
        'T064_* env vars missing; provide Firebase ID tokens, backend sessions, '
        'and user IDs for four isolated teacher, student, supervisor, and '
        'operator fixture accounts.',
      );
      return;
    }
    expect({
      teacher.userId,
      student.userId,
      supervisor.userId,
      operator.userId
    }, hasLength(4), reason: 'T064 requires four distinct fixture accounts');

    final dio = Dio(BaseOptions(
      baseUrl:
          env['T064_API_BASE_URL'] ?? 'http://host.docker.internal:8080/api/v1',
    ));
    addTearDown(dio.close);
    final circles = CircleApiClient(dio);
    final chat = ChatApiClient(dio);
    final media = ChatMediaApiClient(dio);

    final circleA = await _createCircle(circles, teacher, supervisor, 'A');
    final circleB = await _createCircle(circles, teacher, supervisor, 'B');
    addTearDown(() async {
      for (final circle in [circleA, circleB]) {
        try {
          await circles.archiveCircle(
            firebaseIdToken: teacher.token,
            sessionId: teacher.sessionId,
            circleId: circle.id,
          );
        } on DioException {
          // Best-effort cleanup; the assertions are already complete.
        }
      }
    });

    await _join(circles, student, circleA.inviteCode);
    await _join(circles, student, circleB.inviteCode);

    // Both allowed role pairs work in both directions.
    await _send(chat, teacher, student, 'teacher to student');
    await _send(chat, student, teacher, 'student to teacher');
    await _send(chat, supervisor, student, 'supervisor to student');
    await _send(chat, student, supervisor, 'student to supervisor');

    // Every disallowed role pairing is denied without conversation enumeration.
    await _assertAllProhibitedPairs(
      circles,
      chat,
      teacher,
      supervisor,
      operator,
    );
    // A DM upload is bound to the peer, not either qualifying circle.
    final fixture = await _imageFixture();
    addTearDown(() async {
      try {
        await fixture.delete();
      } on FileSystemException {
        // Best-effort cleanup.
      }
    });
    final upload = await media.uploadImage(
      token: teacher.token,
      sessionId: teacher.sessionId,
      filePath: fixture.path,
      dmPeerId: student.userId,
    );
    final mediaMessage = await chat.sendDirectMediaMessage(
      token: teacher.token,
      sessionId: teacher.sessionId,
      userId: student.userId,
      type: ChatMessageType.image,
      uploadId: upload.uploadId!,
      idempotencyKey: 't064-media-${DateTime.now().microsecondsSinceEpoch}',
    );
    // REST identifies the recipient; realtime dm_peer_id is receiver-relative.
    final mediaHistory = await dio.get<Map<String, dynamic>>(
      '/dm/${student.userId}',
      options: Options(
          headers: sessionRequestHeaders(teacher.token, teacher.sessionId)),
    );
    final mediaProjection = (mediaHistory.data!['data'] as List<dynamic>)
        .cast<Map<String, dynamic>>()
        .singleWhere((message) => message['id'] == mediaMessage.id);
    expect(mediaProjection['dm_recipient_id'], student.userId);
    expect(mediaMessage.circleId, isNull);
    expect(mediaMessage.type, ChatMessageType.image);
    expect(mediaProjection['media_url'], isNotEmpty);

    // Losing one qualifying circle preserves the conversation and its media.
    await _remove(circles, teacher, circleA.id, student.userId);
    final survivingHistory = await _list(chat, teacher, student);
    expect(
        survivingHistory.messages.any((m) => m.id == mediaMessage.id), isTrue);

    // Losing the last qualifying circle denies list and send.
    await _remove(circles, teacher, circleB.id, student.userId);
    await _expectDenied(() => _list(chat, teacher, student));
    await _expectDenied(
        () => _send(chat, teacher, student, 'last relationship lost'));

    // Rejoining restores the complete unordered-pair history.
    await _join(circles, student, circleB.inviteCode);
    final restoredHistory = await _list(chat, student, teacher);
    expect(
        restoredHistory.messages.any((m) => m.id == mediaMessage.id), isTrue);
    expect(restoredHistory.messages.length, greaterThanOrEqualTo(2));
  });
}

Future<CircleResponse> _createCircle(
  CircleApiClient circles,
  _Credentials teacher,
  _Credentials supervisor,
  String suffix,
) =>
    circles.createCircle(
      firebaseIdToken: teacher.token,
      sessionId: teacher.sessionId,
      request: CreateCircleRequest(
        name: 'T064-direct-$suffix-${DateTime.now().microsecondsSinceEpoch}',
        language: 'ar',
        maxCapacity: 10,
        backupSupervisorUserId: supervisor.userId,
      ),
    );

Future<void> _join(
  CircleApiClient circles,
  _Credentials user,
  String inviteCode,
) =>
    circles.joinCircleByInvite(
      firebaseIdToken: user.token,
      sessionId: user.sessionId,
      inviteCode: inviteCode,
    );

Future<void> _remove(
  CircleApiClient circles,
  _Credentials teacher,
  String circleId,
  String userId,
) =>
    circles.removeMember(
      firebaseIdToken: teacher.token,
      sessionId: teacher.sessionId,
      circleId: circleId,
      userId: userId,
    );

Future<void> _assignRole(
  CircleApiClient circles,
  _Credentials actor,
  CircleResponse circle,
  String userId,
  CircleRole role,
) =>
    circles.assignMemberRole(
      firebaseIdToken: actor.token,
      sessionId: actor.sessionId,
      circleId: circle.id,
      userId: userId,
      request: AssignCircleRoleRequest(role: role),
    );

Future<void> _assertAllProhibitedPairs(
  CircleApiClient circles,
  ChatApiClient chat,
  _Credentials teacher,
  _Credentials supervisor,
  _Credentials operator,
) async {
  const pairs = [
    (CircleRole.teacher, CircleRole.teacher),
    (CircleRole.teacher, CircleRole.supervisor),
    (CircleRole.supervisor, CircleRole.supervisor),
    (CircleRole.student, CircleRole.student),
  ];
  // Reuse one managed circle so role variants cannot exhaust the five-circle
  // membership budget. The separate creator remains its teacher throughout.
  final circle = await _createCircle(circles, operator, supervisor, 'denied');
  addTearDown(() => circles.archiveCircle(
        firebaseIdToken: operator.token,
        sessionId: operator.sessionId,
        circleId: circle.id,
      ));
  await _join(circles, teacher, circle.inviteCode);
  for (final pair in pairs) {
    // A separate teacher manager sets both target roles, preserving the
    // self-role-change and final-teacher safeguards under test.
    await _assignRole(circles, operator, circle, supervisor.userId, pair.$2);
    await _assignRole(circles, operator, circle, teacher.userId, pair.$1);
    await _expectDenied(() => _list(chat, teacher, supervisor));
    await _expectDenied(
      () => _send(chat, teacher, supervisor, 'must be denied'),
    );
  }
}

Future<ChatMessagePage> _list(
  ChatApiClient chat,
  _Credentials viewer,
  _Credentials peer,
) =>
    chat.listDirectMessages(
      token: viewer.token,
      sessionId: viewer.sessionId,
      userId: peer.userId,
    );

Future<ChatMessage> _send(
  ChatApiClient chat,
  _Credentials sender,
  _Credentials peer,
  String content,
) =>
    chat.sendDirectTextMessage(
      token: sender.token,
      sessionId: sender.sessionId,
      userId: peer.userId,
      content: content,
      idempotencyKey: 't064-${DateTime.now().microsecondsSinceEpoch}',
    );

Future<void> _expectDenied(Future<Object> Function() action) async {
  try {
    await action();
    fail('Expected the direct conversation operation to be denied');
  } on ChatApiException catch (error) {
    expect(error.statusCode, 403);
  }
}

Future<File> _imageFixture() async {
  final file = File(
      '${Directory.systemTemp.path}/t064-${DateTime.now().microsecondsSinceEpoch}.png');
  await file.writeAsBytes(base64Decode(
      'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII='));
  return file;
}

class _Credentials {
  const _Credentials({
    required this.token,
    required this.sessionId,
    required this.userId,
  });

  final String token;
  final String sessionId;
  final String userId;

  static _Credentials? fromEnv(Map<String, String> env, String role) {
    final token = env['T064_${role}_TOKEN'];
    final sessionId = env['T064_${role}_SESSION'];
    final userId = env['T064_${role}_USER_ID'];
    if (token == null || sessionId == null || userId == null) return null;
    return _Credentials(token: token, sessionId: sessionId, userId: userId);
  }
}
