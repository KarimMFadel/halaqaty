import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';

void main() {
  test('session-card discovery reuses the canonical circle sessions path', () async {
    final requests = <RequestOptions>[];
    final dio = Dio()
      ..httpClientAdapter = _DiscoveryAdapter(requests);
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
}

class _DiscoveryAdapter implements HttpClientAdapter {
  _DiscoveryAdapter(this.requests);
  final List<RequestOptions> requests;

  @override
  void close({bool force = false}) {}

  @override
  Future<ResponseBody> fetch(RequestOptions options,
      Stream< List<int> >? requestStream, Future? cancelFuture) async {
    requests.add(options);
    return ResponseBody.fromString(
      '{"data":[{"id":"session-1","circle_id":"circle-1","status":"scheduled","media_mode":"audio_only","participant_count":0,"is_locked":false}]}',
      200,
      headers: <String, List<String>>{
        Headers.contentTypeHeader: <String>['application/json'],
      },
    );
  }
}
