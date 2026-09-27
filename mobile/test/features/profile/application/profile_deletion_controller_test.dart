import 'package:dio/dio.dart';
import 'package:firebase_auth/firebase_auth.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/profile/application/profile_controller.dart';
import 'package:halaqaty_mobile/features/profile/data/profile_api_client.dart';

class _ProviderInfo implements UserInfo {
  _ProviderInfo(this.providerId);

  @override
  final String providerId;

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _DeletionUser implements User {
  _DeletionUser(this.events,
      {this.reauthError,
      this.token = 'fresh-token',
      String provider = 'password'})
      : providerData = [_ProviderInfo(provider)];

  final List<String> events;
  final Object? reauthError;
  final String? token;

  @override
  final String? email = 'student@example.com';

  @override
  final List<UserInfo> providerData;

  @override
  Future<UserCredential> reauthenticateWithCredential(
      AuthCredential credential) async {
    events.add('reauthenticate');
    if (reauthError case final error?) throw error;
    return _UserCredential();
  }

  @override
  Future<String?> getIdToken([bool forceRefresh = false]) async {
    events.add(forceRefresh ? 'refresh-token' : 'token');
    return token;
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _UserCredential implements UserCredential {
  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _FirebaseAuth implements FirebaseAuth {
  _FirebaseAuth(this.currentUser);

  @override
  final User? currentUser;

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _DeletionApi implements ProfileApiClient {
  _DeletionApi(this.events, {this.pending = false, this.error});

  final List<String> events;
  final bool pending;
  final DioException? error;
  String? usedToken;
  String? usedSessionId;

  @override
  Future<bool> deleteMe(
      {required String firebaseIdToken, required String sessionId}) async {
    events.add('delete');
    usedToken = firebaseIdToken;
    usedSessionId = sessionId;
    if (error case final failure?) throw failure;
    return pending;
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

ProfileController _controller({
  required ProfileApiClient api,
  required FirebaseAuth firebaseAuth,
  required List<String> events,
  String? sessionId = 'active-session',
}) =>
    ProfileController(
      apiClient: api,
      firebaseAuth: firebaseAuth,
      readAuthState: () => AuthState(
        status: AuthStatus.authenticated,
        sessionId: sessionId,
      ),
      logout: () async => events.add('logout'),
    );

void main() {
  test('delete request carries bearer and matching backend session', () async {
    final dio = Dio(BaseOptions(baseUrl: 'http://localhost'));
    RequestOptions? sent;
    dio.interceptors.add(InterceptorsWrapper(onRequest: (options, handler) {
      sent = options;
      handler.resolve(Response<Map<String, dynamic>>(
        requestOptions: options,
        statusCode: 202,
        data: const {'status': 'pending_identity_removal'},
      ));
    }));

    final pending = await ProfileApiClient(dio).deleteMe(
      firebaseIdToken: 'fresh-token',
      sessionId: 'active-session',
    );
    expect(pending, isTrue);
    expect(sent?.headers['Authorization'], 'Bearer fresh-token');
    expect(sent?.headers['X-Halaqaty-Session-ID'], 'active-session');
  });

  test('reauthenticates and returns the committed result before logout',
      () async {
    final events = <String>[];
    final api = _DeletionApi(events, pending: true);
    final controller = _controller(
      api: api,
      firebaseAuth: _FirebaseAuth(_DeletionUser(events)),
      events: events,
    );
    addTearDown(controller.dispose);

    final result = await controller.deleteAccount(password: 'secret');

    expect(result, AccountDeletionOutcome.pending);
    expect(api.usedToken, 'fresh-token');
    expect(api.usedSessionId, 'active-session');
    expect(events, ['reauthenticate', 'refresh-token', 'delete']);
  });

  test('reauthentication failure never calls deletion or logout', () async {
    final events = <String>[];
    final api = _DeletionApi(events);
    final controller = _controller(
      api: api,
      firebaseAuth: _FirebaseAuth(_DeletionUser(
        events,
        reauthError: FirebaseAuthException(code: 'requires-recent-login'),
      )),
      events: events,
    );
    addTearDown(controller.dispose);

    final result = await controller.deleteAccount(password: 'wrong-password');

    expect(result, isNull);
    expect(events, ['reauthenticate']);
    expect(controller.state.errorMessage, isNotEmpty);
  });

  test('missing fresh token explains failure without deleting', () async {
    final events = <String>[];
    final controller = _controller(
      api: _DeletionApi(events),
      firebaseAuth: _FirebaseAuth(_DeletionUser(events, token: null)),
      events: events,
    );
    addTearDown(controller.dispose);

    expect(await controller.deleteAccount(password: 'secret'), isNull);
    expect(events, ['reauthenticate', 'refresh-token']);
    expect(controller.state.errorMessage, contains('verify your identity'));
  });

  test('missing backend session cannot submit deletion', () async {
    final events = <String>[];
    final controller = _controller(
      api: _DeletionApi(events),
      firebaseAuth: _FirebaseAuth(_DeletionUser(events)),
      events: events,
      sessionId: null,
    );
    addTearDown(controller.dispose);

    expect(await controller.deleteAccount(password: 'secret'), isNull);
    expect(events, ['reauthenticate', 'refresh-token']);
    expect(controller.state.errorMessage, contains('Session is missing'));
  });

  test('server conflict keeps the local session active', () async {
    final events = <String>[];
    final api = _DeletionApi(
      events,
      error: DioException(
        requestOptions: RequestOptions(path: '/auth/me'),
        response: Response(
          requestOptions: RequestOptions(path: '/auth/me'),
          statusCode: 409,
          data: {
            'error': {
              'code': 'ACTIVE_SESSION',
              'message': 'Leave the session first'
            }
          },
        ),
      ),
    );
    final controller = _controller(
      api: api,
      firebaseAuth: _FirebaseAuth(_DeletionUser(events)),
      events: events,
    );
    addTearDown(controller.dispose);

    final result = await controller.deleteAccount(password: 'secret');

    expect(result, isNull);
    expect(controller.state.errorMessage, 'Leave the session first');
    expect(events, ['reauthenticate', 'refresh-token', 'delete']);
  });

  test('non-password identity cannot enter password reauthentication',
      () async {
    final events = <String>[];
    final api = _DeletionApi(events);
    final controller = _controller(
      api: api,
      firebaseAuth:
          _FirebaseAuth(_DeletionUser(events, provider: 'google.com')),
      events: events,
    );
    addTearDown(controller.dispose);

    final result = await controller.deleteAccount(password: 'secret');

    expect(result, isNull);
    expect(events, isEmpty);
    expect(controller.state.errorMessage, contains('reauthentication'));
  });
}
