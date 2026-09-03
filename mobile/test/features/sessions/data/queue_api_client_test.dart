import 'dart:convert';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';

const _token = 'firebase-token';
const _backendSessionId = 'backend-session';
const _liveSessionId = '11111111-1111-1111-1111-111111111111';
const _entryId = '33333333-3333-3333-3333-333333333333';

void main() {
  group('QueueState', () {
    test('decodes a manager queue snapshot with policy entries and preorder',
        () {
      final state = QueueState.fromJson(_queueStateJson(
        preorder: [
          {
            'student_id': '44444444-4444-4444-4444-444444444444',
            'student_name': 'أحمد',
            'position': 1,
          },
        ],
      ));

      expect(state.sessionId, _liveSessionId);
      expect(state.roundId, '22222222-2222-2222-2222-222222222222');
      expect(state.roundNumber, 2);
      expect(state.lifecycle, 'active');
      expect(state.selectedEntryId, _entryId);
      expect(state.policy.population, 'present_at_activation');
      expect(state.policy.gradeVisibility, 'managers_and_student');
      expect(state.policy.version, 4);
      expect(state.entries.single.studentName, 'مريم');
      expect(state.entries.single.status, 'waiting');
      expect(state.entries.single.gradeNotes, isNull);
      expect(state.preorder.single.studentName, 'أحمد');
      expect(state.preorder.single.position, 1);
    });

    test(
        'getQueue preserves the empty preorder projection sent to non-managers',
        () async {
      final requests = <RequestOptions>[];
      final client = QueueApiClient(
        Dio()
          ..httpClientAdapter = _QueueAdapter(
            requests,
            [_QueuedResponse.ok(_queueStateJson())],
          ),
      );

      final state = await client.getQueue(
        token: _token,
        sessionId: _backendSessionId,
        liveSessionId: _liveSessionId,
      );

      expect(state.preorder, isEmpty);
      expect(requests.single.path, '/sessions/$_liveSessionId/queue');
      expect(requests.single.method, 'GET');
      expect(requests.single.headers['Authorization'], 'Bearer $_token');
      expect(
        requests.single.headers['X-Halaqaty-Session-ID'],
        _backendSessionId,
      );
    });
  });

  group('QueueApiClient', () {
    test('sends atomic completion and audited correction payloads', () async {
      final requests = <RequestOptions>[];
      final client = QueueApiClient(
        Dio()
          ..httpClientAdapter = _QueueAdapter(
            requests,
            [
              _QueuedResponse.ok(_queueStateJson()),
              _QueuedResponse.ok({
                'id': _entryId,
                'student_id': '55555555-5555-5555-5555-555555555555',
                'student_name': 'مريم',
                'position': 1,
                'status': 'completed',
                'grade': 'good',
                'grade_notes': null,
                'version': 4,
              }),
            ],
          ),
      );

      await client.completeEntry(
        token: _token,
        sessionId: _backendSessionId,
        liveSessionId: _liveSessionId,
        entryId: _entryId,
        expectedEntryVersion: 3,
        grade: 'excellent',
        notes: 'Strong recitation',
        idempotencyKey: 'complete-retry',
      );
      await client.correctEntry(
        token: _token,
        sessionId: _backendSessionId,
        liveSessionId: _liveSessionId,
        entryId: _entryId,
        expectedEntryVersion: 4,
        grade: 'good',
        includeNotes: true,
        idempotencyKey: 'correct-retry',
      );

      expect(_request(requests, 0), {
        'path': '/sessions/$_liveSessionId/queue/entries/$_entryId/status',
        'method': 'PUT',
        'data': {
          'status': 'completed',
          'expected_entry_version': 3,
          'grade': 'excellent',
          'notes': 'Strong recitation',
        },
        'idempotencyKey': 'complete-retry',
      });
      expect(_request(requests, 1), {
        'path': '/sessions/$_liveSessionId/queue/entries/$_entryId/grade',
        'method': 'POST',
        'data': {
          'expected_entry_version': 4,
          'grade': 'good',
          'notes': null,
        },
        'idempotencyKey': 'correct-retry',
      });
    });

    test('manager commands send documented payloads and idempotency keys',
        () async {
      final requests = <RequestOptions>[];
      final client = QueueApiClient(
        Dio()
          ..httpClientAdapter = _QueueAdapter(
            requests,
            [
              _QueuedResponse.ok(_queueStateJson()),
              _QueuedResponse.ok(_queueStateJson()),
              _QueuedResponse.ok(_queueStateJson()),
              _QueuedResponse.ok(_queueStateJson()),
              _QueuedResponse.ok(_queueStateJson()),
              _QueuedResponse.ok(_queueStateJson()),
              _QueuedResponse.ok(_queuePolicyJson()),
            ],
          ),
      );

      await client.prepareRound(
        token: _token,
        sessionId: _backendSessionId,
        liveSessionId: _liveSessionId,
        roundType: 'revision',
        surahId: 2,
        fromAyah: 1,
        toAyah: 5,
        gradingRequired: true,
        studentOrder: const ['student-1', 'student-2'],
        idempotencyKey: 'prepare-retry',
      );
      await client.advance(
        token: _token,
        sessionId: _backendSessionId,
        liveSessionId: _liveSessionId,
        expectedVersion: 4,
        idempotencyKey: 'advance-retry',
      );
      await client.reorder(
        token: _token,
        sessionId: _backendSessionId,
        liveSessionId: _liveSessionId,
        orderedIds: const ['student-2', 'student-1'],
        expectedVersion: 4,
        idempotencyKey: 'reorder-retry',
      );
      await client.moveEntry(
        token: _token,
        sessionId: _backendSessionId,
        liveSessionId: _liveSessionId,
        entryId: _entryId,
        newPosition: 2,
        expectedVersion: 4,
        idempotencyKey: 'move-retry',
      );
      await client.updateEntryStatus(
        token: _token,
        sessionId: _backendSessionId,
        liveSessionId: _liveSessionId,
        entryId: _entryId,
        status: 'reciting',
        expectedEntryVersion: 3,
        idempotencyKey: 'status-retry',
      );
      await client.reset(
        token: _token,
        sessionId: _backendSessionId,
        liveSessionId: _liveSessionId,
        roundType: 'new_memorization',
        surahId: 1,
        fromAyah: 1,
        toAyah: 7,
        gradingRequired: false,
        expectedVersion: 4,
        idempotencyKey: 'reset-retry',
      );
      await client.updatePolicy(
        token: _token,
        sessionId: _backendSessionId,
        liveSessionId: _liveSessionId,
        expectedVersion: 4,
        population: 'all_active_students',
        idempotencyKey: 'policy-retry',
      );

      expect(_request(requests, 0), {
        'path': '/sessions/$_liveSessionId/queue/rounds',
        'method': 'POST',
        'data': {
          'round_type': 'revision',
          'surah_id': 2,
          'from_ayah': 1,
          'to_ayah': 5,
          'grading_required': true,
          'student_order': ['student-1', 'student-2'],
        },
        'idempotencyKey': 'prepare-retry',
      });
      expect(_request(requests, 1), {
        'path': '/sessions/$_liveSessionId/queue/advance',
        'method': 'POST',
        'data': {'expected_version': 4},
        'idempotencyKey': 'advance-retry',
      });
      expect(_request(requests, 2), {
        'path': '/sessions/$_liveSessionId/queue/order',
        'method': 'PUT',
        'data': {
          'ordered_ids': ['student-2', 'student-1'],
          'expected_version': 4,
        },
        'idempotencyKey': 'reorder-retry',
      });
      expect(_request(requests, 3), {
        'path': '/sessions/$_liveSessionId/queue/entries/$_entryId/move',
        'method': 'POST',
        'data': {'new_position': 2, 'expected_version': 4},
        'idempotencyKey': 'move-retry',
      });
      expect(_request(requests, 4), {
        'path': '/sessions/$_liveSessionId/queue/entries/$_entryId/status',
        'method': 'PUT',
        'data': {'status': 'reciting', 'expected_entry_version': 3},
        'idempotencyKey': 'status-retry',
      });
      expect(_request(requests, 5), {
        'path': '/sessions/$_liveSessionId/queue/reset',
        'method': 'POST',
        'data': {
          'round_type': 'new_memorization',
          'surah_id': 1,
          'from_ayah': 1,
          'to_ayah': 7,
          'grading_required': false,
          'expected_version': 4,
        },
        'idempotencyKey': 'reset-retry',
      });
      expect(_request(requests, 6), {
        'path': '/sessions/$_liveSessionId/queue/policy',
        'method': 'PATCH',
        'data': {'expected_version': 4, 'population': 'all_active_students'},
        'idempotencyKey': 'policy-retry',
      });
    });

    test('maps standard 409, 422, and 503 error envelopes', () async {
      final cases = <({
        int statusCode,
        String code,
        String message,
        Map<String, String>? fields,
        Future<void> Function(QueueApiClient) invoke,
      })>[
        (
          statusCode: 409,
          code: 'ERR_QUEUE_CONFLICT',
          message: 'Queue version is stale.',
          fields: null,
          invoke: (client) async {
            await client.advance(
              token: _token,
              sessionId: _backendSessionId,
              liveSessionId: _liveSessionId,
              expectedVersion: 4,
              idempotencyKey: 'conflict-retry',
            );
          },
        ),
        (
          statusCode: 422,
          code: 'ERR_VALIDATION_FAILED',
          message: 'The Ayah range is invalid.',
          fields: const {'to_ayah': 'must not exceed the Surah length'},
          invoke: (client) async {
            await client.prepareRound(
              token: _token,
              sessionId: _backendSessionId,
              liveSessionId: _liveSessionId,
              roundType: 'revision',
              surahId: 1,
              fromAyah: 1,
              toAyah: 8,
              gradingRequired: false,
              idempotencyKey: 'validation-retry',
            );
          },
        ),
        (
          statusCode: 503,
          code: 'ERR_QUEUE_UNAVAILABLE',
          message: 'Queue processing is temporarily unavailable.',
          fields: null,
          invoke: (client) async {
            await client.updateEntryStatus(
              token: _token,
              sessionId: _backendSessionId,
              liveSessionId: _liveSessionId,
              entryId: _entryId,
              status: 'skipped',
              expectedEntryVersion: 3,
              idempotencyKey: 'unavailable-retry',
            );
          },
        ),
      ];

      for (final testCase in cases) {
        final client = QueueApiClient(
          Dio()
            ..httpClientAdapter = _QueueAdapter(
              <RequestOptions>[],
              [
                _QueuedResponse.error(
                  testCase.statusCode,
                  _errorEnvelope(
                    code: testCase.code,
                    message: testCase.message,
                    fields: testCase.fields,
                  ),
                ),
              ],
            ),
        );

        try {
          await testCase.invoke(client);
          fail('Expected QueueApiException for ${testCase.statusCode}.');
        } on QueueApiException catch (error) {
          expect(error.statusCode, testCase.statusCode);
          expect(error.code, testCase.code);
          expect(error.message, testCase.message);
          expect(error.fields, testCase.fields);
        }
      }
    });
  });
}

Map<String, dynamic> _queueStateJson(
        {List<Map<String, dynamic>> preorder = const []}) =>
    {
      'session_id': _liveSessionId,
      'round_id': '22222222-2222-2222-2222-222222222222',
      'round_number': 2,
      'round_type': 'revision',
      'lifecycle': 'active',
      'surah_id': 2,
      'from_ayah': 1,
      'to_ayah': 5,
      'grading_required': true,
      'selected_entry_id': _entryId,
      'version': 7,
      'policy': _queuePolicyJson(),
      'preorder': preorder,
      'entries': [
        {
          'id': _entryId,
          'student_id': '55555555-5555-5555-5555-555555555555',
          'student_name': 'مريم',
          'position': 2,
          'status': 'waiting',
          'grade': null,
          'grade_notes': null,
          'version': 3,
        },
      ],
    };

Map<String, dynamic> _queuePolicyJson() => {
      'population': 'present_at_activation',
      'unfinished_finalization': 'mark_unfinished_skipped',
      'opt_out': 'approval_required',
      'grade_visibility': 'managers_and_student',
      'grade_correction': 'audited_any_time',
      'version': 4,
    };

Map<String, dynamic> _errorEnvelope({
  required String code,
  required String message,
  Map<String, String>? fields,
}) =>
    {
      'error': {
        'code': code,
        'message': message,
        if (fields != null) 'fields': fields,
      },
    };

Map<String, dynamic> _request(List<RequestOptions> requests, int index) {
  final request = requests[index];
  return {
    'path': request.path,
    'method': request.method,
    'data': request.data,
    'idempotencyKey': request.headers['Idempotency-Key'],
  };
}

class _QueuedResponse {
  const _QueuedResponse(this.statusCode, this.body);

  factory _QueuedResponse.ok(Map<String, dynamic> body) =>
      _QueuedResponse(200, body);

  factory _QueuedResponse.error(int statusCode, Map<String, dynamic> body) =>
      _QueuedResponse(statusCode, body);

  final int statusCode;
  final Map<String, dynamic> body;
}

class _QueueAdapter implements HttpClientAdapter {
  _QueueAdapter(this.requests, this.responses);

  final List<RequestOptions> requests;
  final List<_QueuedResponse> responses;

  @override
  void close({bool force = false}) {}

  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<List<int>>? requestStream,
    Future<dynamic>? cancelFuture,
  ) async {
    requests.add(options);
    final response = responses.removeAt(0);
    return ResponseBody.fromString(
      jsonEncode(response.body),
      response.statusCode,
      headers: <String, List<String>>{
        Headers.contentTypeHeader: <String>['application/json'],
      },
    );
  }
}
