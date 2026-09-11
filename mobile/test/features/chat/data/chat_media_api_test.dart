import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_media_api.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_protocol_constants.dart';

const _token = 'firebase-token';
const _backendSessionId = 'backend-session';
const _circleId = '22222222-2222-2222-2222-222222222222';
const _dmPeerId = '44444444-4444-4444-4444-444444444444';
const _messageId = '11111111-1111-1111-1111-111111111111';

const _uploadJson = {
  'object_key': 'chat/voice/abc.m4a',
  'url': 'https://media.example.com/chat/voice/abc.m4a?sig=1',
  'upload_id': '55555555-5555-5555-5555-555555555555',
  'url_expires_at': '2026-09-15T12:00:00Z',
};

late Directory _tempDir;

void main() {
  setUpAll(() async {
    _tempDir = await Directory.systemTemp.createTemp('chat_media_api_test');
  });
  tearDownAll(() async {
    await _tempDir.delete(recursive: true);
  });

  File writeFile(String name, int bytes) {
    final file = File('${_tempDir.path}/$name');
    file.writeAsBytesSync(List<int>.filled(bytes, 1));
    return file;
  }

  Map<String, String> formFields(FormData form) => {
        for (final entry in form.fields) entry.key: entry.value,
      };

  group('ChatMediaApiClient uploads', () {
    test('uploadVoice posts multipart to /uploads/voice with circle_id',
        () async {
      final requests = <RequestOptions>[];
      final client = ChatMediaApiClient(
        Dio()..httpClientAdapter = _ChatAdapter(requests, [_uploadJson]),
      );
      final file = writeFile('note.m4a', 16);

      final result = await client.uploadVoice(
        token: _token,
        sessionId: _backendSessionId,
        filePath: file.path,
        durationSeconds: 42,
        circleId: _circleId,
      );

      final request = requests.single;
      expect(request.path, '/uploads/voice');
      expect(request.method, 'POST');
      expect(request.headers['Authorization'], 'Bearer $_token');
      expect(request.headers['X-Halaqaty-Session-ID'], _backendSessionId);
      final form = request.data as FormData;
      final fields = formFields(form);
      expect(fields, containsPair('circle_id', _circleId));
      expect(fields.containsKey('dm_peer_id'), isFalse);
      // The backend voice contract requires the client-declared duration as
      // a multipart field; omitting it makes every voice upload 422.
      expect(fields, containsPair('duration_seconds', '42'));
      expect(form.files.single.key, 'file');
      expect(form.files.single.value.filename, 'note.m4a');
      expect(result.objectKey, 'chat/voice/abc.m4a');
      expect(result.url, 'https://media.example.com/chat/voice/abc.m4a?sig=1');
      expect(result.uploadId, '55555555-5555-5555-5555-555555555555');
      expect(result.urlExpiresAt, DateTime.parse('2026-09-15T12:00:00Z'));
    });

    test('uploadImage posts multipart to /uploads/image with dm_peer_id only',
        () async {
      final requests = <RequestOptions>[];
      final client = ChatMediaApiClient(
        Dio()..httpClientAdapter = _ChatAdapter(requests, [_uploadJson]),
      );
      final file = writeFile('mushaf.png', 16);

      await client.uploadImage(
        token: _token,
        sessionId: _backendSessionId,
        filePath: file.path,
        dmPeerId: _dmPeerId,
      );

      final request = requests.single;
      expect(request.path, '/uploads/image');
      expect(request.method, 'POST');
      expect(request.headers['Authorization'], 'Bearer $_token');
      expect(request.headers['X-Halaqaty-Session-ID'], _backendSessionId);
      final form = request.data as FormData;
      final fields = formFields(form);
      expect(fields, containsPair('dm_peer_id', _dmPeerId));
      expect(fields.containsKey('circle_id'), isFalse);
      // duration_seconds is voice-only in the contract.
      expect(fields.containsKey('duration_seconds'), isFalse);
      expect(form.files.single.key, 'file');
      expect(form.files.single.value.filename, 'mushaf.png');
    });

    test('uploadFile posts multipart to /uploads/file with circle_id',
        () async {
      final requests = <RequestOptions>[];
      final client = ChatMediaApiClient(
        Dio()..httpClientAdapter = _ChatAdapter(requests, [_uploadJson]),
      );
      final file = writeFile('exercise.pdf', 16);

      await client.uploadFile(
        token: _token,
        sessionId: _backendSessionId,
        filePath: file.path,
        circleId: _circleId,
      );

      final request = requests.single;
      expect(request.path, '/uploads/file');
      expect(request.method, 'POST');
      expect(request.headers['Authorization'], 'Bearer $_token');
      expect(request.headers['X-Halaqaty-Session-ID'], _backendSessionId);
      final form = request.data as FormData;
      expect(formFields(form), containsPair('circle_id', _circleId));
      expect(formFields(form).containsKey('duration_seconds'), isFalse);
      expect(form.files.single.value.filename, 'exercise.pdf');
    });

    test('forwards upload progress through the onSendProgress callback',
        () async {
      final requests = <RequestOptions>[];
      final client = ChatMediaApiClient(
        Dio()..httpClientAdapter = _ChatAdapter(requests, [_uploadJson]),
      );
      final progress = <(int, int)>[];
      final file = writeFile('note.m4a', 16);

      await client.uploadVoice(
        token: _token,
        sessionId: _backendSessionId,
        filePath: file.path,
        durationSeconds: 1,
        circleId: _circleId,
        onProgress: (count, total) => progress.add((count, total)),
      );

      // The callback must be wired into Dio's onSendProgress; drive the
      // captured hook the way the transport would.
      final hook = requests.single.onSendProgress;
      expect(hook, isNotNull);
      hook!(10, 100);
      hook(100, 100);
      expect(progress, [(10, 100), (100, 100)]);
    });

    test('is codec-neutral: every contract voice container uploads identically',
        () async {
      for (final extension in ['ogg', 'mp3', 'webm', 'm4a']) {
        final requests = <RequestOptions>[];
        final client = ChatMediaApiClient(
          Dio()..httpClientAdapter = _ChatAdapter(requests, [_uploadJson]),
        );
        final file = writeFile('note.$extension', 16);

        final result = await client.uploadVoice(
          token: _token,
          sessionId: _backendSessionId,
          filePath: file.path,
          durationSeconds: 5,
          circleId: _circleId,
        );

        // No client-side extension filtering: each container reaches the
        // endpoint unchanged and parses the same response.
        expect(requests.single.path, '/uploads/voice');
        expect(
          (requests.single.data as FormData).files.single.value.filename,
          'note.$extension',
        );
        expect(result.uploadId, '55555555-5555-5555-5555-555555555555');
      }
    });
  });

  group('ChatMediaApiClient media-url renewal', () {
    test('renewMediaUrl posts to /messages/{id}/media-url and parses access',
        () async {
      final requests = <RequestOptions>[];
      final client = ChatMediaApiClient(
        Dio()
          ..httpClientAdapter = _ChatAdapter(requests, [
            {
              'url': 'https://media.example.com/chat/voice/abc.m4a?sig=2',
              'expires_at': '2026-09-22T12:00:00Z',
            },
          ]),
      );

      final access = await client.renewMediaUrl(
        token: _token,
        sessionId: _backendSessionId,
        messageId: _messageId,
      );

      final request = requests.single;
      expect(request.path, '/messages/$_messageId/media-url');
      expect(request.method, 'POST');
      expect(request.headers['Authorization'], 'Bearer $_token');
      expect(request.headers['X-Halaqaty-Session-ID'], _backendSessionId);
      expect(access.url, 'https://media.example.com/chat/voice/abc.m4a?sig=2');
      expect(access.expiresAt, DateTime.parse('2026-09-22T12:00:00Z'));
    });
  });

  group('ChatMediaApiClient error mapping', () {
    for (final statusCode in [413, 415, 422, 429]) {
      test('maps contract error envelope for $statusCode', () async {
        final client = ChatMediaApiClient(
          Dio()
            ..httpClientAdapter = _ChatAdapter(
              <RequestOptions>[],
              [
                {
                  'error': {
                    'code': 'ERR_UPLOAD_TOO_LARGE',
                    'message': 'Attachment exceeds the size limit.',
                  },
                },
              ],
              statusCode: statusCode,
            ),
        );

        await expectLater(
          client.renewMediaUrl(
            token: _token,
            sessionId: _backendSessionId,
            messageId: _messageId,
          ),
          throwsA(
            isA<ChatApiException>()
                .having((e) => e.statusCode, 'statusCode', statusCode)
                .having((e) => e.code, 'code', 'ERR_UPLOAD_TOO_LARGE')
                .having((e) => e.message, 'message',
                    'Attachment exceeds the size limit.'),
          ),
        );
      });
    }

    test('maps transport failures to ChatApiException without a status code',
        () async {
      final client = ChatMediaApiClient(
        Dio()
          ..httpClientAdapter = _ThrowingAdapter(
            (options) => DioException.connectionError(
              requestOptions: options,
              reason: 'offline',
            ),
          ),
      );

      await expectLater(
        client.renewMediaUrl(
          token: _token,
          sessionId: _backendSessionId,
          messageId: _messageId,
        ),
        throwsA(
          isA<ChatApiException>()
              .having((e) => e.statusCode, 'statusCode', isNull)
              .having((e) => e.code, 'code', ChatApiErrors.requestFailed),
        ),
      );
    });
  });

  group('ChatMediaApiClient client-side limits', () {
    test('rejects voice longer than 300 seconds before any network work',
        () async {
      final requests = <RequestOptions>[];
      final client = ChatMediaApiClient(
        Dio()..httpClientAdapter = _ChatAdapter(requests, [_uploadJson]),
      );
      final file = writeFile('note.m4a', 16);

      await expectLater(
        client.uploadVoice(
          token: _token,
          sessionId: _backendSessionId,
          filePath: file.path,
          durationSeconds: 301,
          circleId: _circleId,
        ),
        throwsA(
          isA<ChatMediaLimitException>()
              .having((e) => e.kind, 'kind', ChatMediaLimitKind.voiceTooLong),
        ),
      );
      expect(requests, isEmpty);
    });

    test('rejects a voice file larger than 20 MB before any network work',
        () async {
      final requests = <RequestOptions>[];
      final client = ChatMediaApiClient(
        Dio()..httpClientAdapter = _ChatAdapter(requests, [_uploadJson]),
      );
      final file = writeFile('big.m4a', 20 * 1024 * 1024 + 1);

      await expectLater(
        client.uploadVoice(
          token: _token,
          sessionId: _backendSessionId,
          filePath: file.path,
          durationSeconds: 10,
          circleId: _circleId,
        ),
        throwsA(
          isA<ChatMediaLimitException>()
              .having((e) => e.kind, 'kind', ChatMediaLimitKind.voiceTooLarge),
        ),
      );
      expect(requests, isEmpty);
    });

    test('rejects an image larger than 5 MB before any network work', () async {
      final requests = <RequestOptions>[];
      final client = ChatMediaApiClient(
        Dio()..httpClientAdapter = _ChatAdapter(requests, [_uploadJson]),
      );
      final file = writeFile('big.png', 5 * 1024 * 1024 + 1);

      await expectLater(
        client.uploadImage(
          token: _token,
          sessionId: _backendSessionId,
          filePath: file.path,
          circleId: _circleId,
        ),
        throwsA(
          isA<ChatMediaLimitException>()
              .having((e) => e.kind, 'kind', ChatMediaLimitKind.imageTooLarge),
        ),
      );
      expect(requests, isEmpty);
    });

    test('rejects a PDF larger than 10 MB before any network work', () async {
      final requests = <RequestOptions>[];
      final client = ChatMediaApiClient(
        Dio()..httpClientAdapter = _ChatAdapter(requests, [_uploadJson]),
      );
      final file = writeFile('big.pdf', 10 * 1024 * 1024 + 1);

      await expectLater(
        client.uploadFile(
          token: _token,
          sessionId: _backendSessionId,
          filePath: file.path,
          circleId: _circleId,
        ),
        throwsA(
          isA<ChatMediaLimitException>()
              .having((e) => e.kind, 'kind', ChatMediaLimitKind.fileTooLarge),
        ),
      );
      expect(requests, isEmpty);
    });

    test('accepts media exactly at the limits', () async {
      final requests = <RequestOptions>[];
      final client = ChatMediaApiClient(
        Dio()
          ..httpClientAdapter =
              _ChatAdapter(requests, [_uploadJson, _uploadJson, _uploadJson]),
      );
      final voice = writeFile('at-limit.m4a', 20 * 1024 * 1024);
      final image = writeFile('at-limit.png', 5 * 1024 * 1024);
      final pdf = writeFile('at-limit.pdf', 10 * 1024 * 1024);

      await client.uploadVoice(
        token: _token,
        sessionId: _backendSessionId,
        filePath: voice.path,
        durationSeconds: 300,
        circleId: _circleId,
      );
      await client.uploadImage(
        token: _token,
        sessionId: _backendSessionId,
        filePath: image.path,
        circleId: _circleId,
      );
      await client.uploadFile(
        token: _token,
        sessionId: _backendSessionId,
        filePath: pdf.path,
        circleId: _circleId,
      );

      expect(requests, hasLength(3));
    });
  });
}

class _ChatAdapter implements HttpClientAdapter {
  _ChatAdapter(this.requests, this.bodies, {this.statusCode = 200});

  final List<RequestOptions> requests;
  final List<Map<String, dynamic>> bodies;
  final int statusCode;

  @override
  void close({bool force = false}) {}

  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<List<int>>? requestStream,
    Future<dynamic>? cancelFuture,
  ) async {
    requests.add(options);
    final body = bodies.removeAt(0);
    return ResponseBody.fromString(
      jsonEncode(body),
      statusCode,
      headers: <String, List<String>>{
        Headers.contentTypeHeader: <String>['application/json'],
      },
    );
  }
}

class _ThrowingAdapter implements HttpClientAdapter {
  _ThrowingAdapter(this._errorFactory);

  final DioException Function(RequestOptions options) _errorFactory;

  @override
  void close({bool force = false}) {}

  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<List<int>>? requestStream,
    Future<dynamic>? cancelFuture,
  ) async {
    throw _errorFactory(options);
  }
}
