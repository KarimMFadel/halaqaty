import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/sessions/application/circle_sessions_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/session_api_client.dart';

void main() {
  test('initial state is loading with an empty list', () {
    final controller = _controller(_FakeSessionApi());
    addTearDown(controller.dispose);

    expect(controller.state.status, CircleSessionsStatus.loading);
    expect(controller.state.sessions, isEmpty);
    expect(controller.state.failure, isNull);
  });

  test('load lists circle sessions with injected credentials and circle id',
      () async {
    final api = _FakeSessionApi()..listResult = [_session('session-1')];
    final controller = _controller(api);
    addTearDown(controller.dispose);

    await controller.load();

    expect(controller.state.status, CircleSessionsStatus.ready);
    expect(controller.state.sessions.single.id, 'session-1');
    expect(api.listCalls, 1);
    expect(api.lastCircleId, 'circle-1');
    expect(api.lastToken, 'firebase-token');
    expect(api.lastSessionId, 'backend-session');
  });

  test('load failure keeps the last loaded list (degraded, not empty)',
      () async {
    final api = _FakeSessionApi()..listResult = [_session('session-1')];
    final controller = _controller(api);
    addTearDown(controller.dispose);
    await controller.load();

    api.listError = DioException(
      requestOptions: RequestOptions(path: '/circles/circle-1/sessions'),
      type: DioExceptionType.connectionError,
    );
    await controller.load();

    expect(controller.state.status, CircleSessionsStatus.error);
    expect(controller.state.failure, CircleSessionsFailure.network);
    expect(controller.state.sessions.single.id, 'session-1');
  });

  test('a 403 load failure is reported as a permission failure', () async {
    final api = _FakeSessionApi()
      ..listError = DioException(
        requestOptions: RequestOptions(path: '/circles/circle-1/sessions'),
        response: Response<Map<String, dynamic>>(
          requestOptions: RequestOptions(path: '/circles/circle-1/sessions'),
          statusCode: 403,
        ),
      );
    final controller = _controller(api);
    addTearDown(controller.dispose);

    await controller.load();

    expect(controller.state.status, CircleSessionsStatus.error);
    expect(controller.state.failure, CircleSessionsFailure.permission);
  });

  test('retry after a failure reloads successfully', () async {
    final api = _FakeSessionApi()..listError = StateError('temporary');
    final controller = _controller(api);
    addTearDown(controller.dispose);
    await controller.load();
    expect(controller.state.status, CircleSessionsStatus.error);

    api.listError = null;
    api.listResult = [_session('session-2')];
    await controller.load();

    expect(controller.state.status, CircleSessionsStatus.ready);
    expect(controller.state.sessions.single.id, 'session-2');
    expect(api.listCalls, 2);
  });

  test('create posts to the circle and prepends the created session', () async {
    final api = _FakeSessionApi()
      ..createResult = _session('session-created', status: 'scheduled');
    final controller = _controller(api);
    addTearDown(controller.dispose);

    final created = await controller.create();

    expect(created?.id, 'session-created');
    expect(api.createCalls, 1);
    expect(api.lastCircleId, 'circle-1');
    expect(controller.state.sessions.first.id, 'session-created');
    expect(controller.state.isCreating, isFalse);
    expect(controller.state.failure, isNull);
  });

  test('create failure keeps the list and exposes a failure', () async {
    final api = _FakeSessionApi()
      ..listResult = [_session('session-1')]
      ..createError = DioException(
        requestOptions: RequestOptions(path: '/circles/circle-1/sessions'),
        response: Response<Map<String, dynamic>>(
          requestOptions: RequestOptions(path: '/circles/circle-1/sessions'),
          statusCode: 403,
        ),
      );
    final controller = _controller(api);
    addTearDown(controller.dispose);
    await controller.load();

    final created = await controller.create();

    expect(created, isNull);
    expect(controller.state.isCreating, isFalse);
    expect(controller.state.status, CircleSessionsStatus.ready);
    expect(controller.state.failure, CircleSessionsFailure.permission);
    expect(controller.state.sessions.single.id, 'session-1');
  });

  test('create failure on an empty list keeps the empty state, not load-error',
      () async {
    final api = _FakeSessionApi()
      ..createError = DioException(
        requestOptions: RequestOptions(path: '/circles/circle-1/sessions'),
        type: DioExceptionType.connectionError,
      );
    final controller = _controller(api);
    addTearDown(controller.dispose);
    await controller.load();

    final created = await controller.create();

    expect(created, isNull);
    expect(controller.state.isCreating, isFalse);
    expect(controller.state.status, CircleSessionsStatus.ready);
    expect(controller.state.failure, CircleSessionsFailure.network);
    expect(controller.state.sessions, isEmpty);
  });
}

CircleSessionsController _controller(_FakeSessionApi api) =>
    CircleSessionsController(
      api,
      () async => (token: 'firebase-token', sessionId: 'backend-session'),
      circleId: 'circle-1',
    );

SessionModel _session(String id, {String status = 'active'}) => SessionModel(
      id: id,
      circleId: 'circle-1',
      status: status,
      mediaMode: 'audio',
      participantCount: 0,
      isLocked: false,
    );

class _FakeSessionApi extends SessionApiClient {
  _FakeSessionApi() : super(Dio());

  List<SessionModel> listResult = const [];
  Object? listError;
  SessionModel? createResult;
  Object? createError;
  int listCalls = 0;
  int createCalls = 0;
  String? lastCircleId;
  String? lastToken;
  String? lastSessionId;

  @override
  Future<List<SessionModel>> list({
    required String token,
    required String sessionId,
    required String circleId,
  }) async {
    listCalls++;
    lastToken = token;
    lastSessionId = sessionId;
    lastCircleId = circleId;
    final error = listError;
    if (error != null) throw error;
    return listResult;
  }

  @override
  Future<SessionModel> create({
    required String token,
    required String sessionId,
    required String circleId,
  }) async {
    createCalls++;
    lastToken = token;
    lastSessionId = sessionId;
    lastCircleId = circleId;
    final error = createError;
    if (error != null) throw error;
    return createResult!;
  }
}
