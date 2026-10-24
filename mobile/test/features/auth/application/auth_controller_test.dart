import 'dart:async';

import 'package:dio/dio.dart';
import 'package:firebase_auth/firebase_auth.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';

class _FakeUser implements User {
  _FakeUser(this.log, {required this.idToken});

  final List<String> log;
  String idToken;

  @override
  Future<String?> getIdToken([bool forceRefresh = false]) async {
    log.add('getIdToken:$idToken');
    return idToken;
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _FakeUserCredential implements UserCredential {
  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _FakeFirebaseAuth implements FirebaseAuth {
  _FakeFirebaseAuth(this.log, this.currentUser);

  final List<String> log;

  @override
  User? currentUser;

  @override
  Future<UserCredential> signInWithEmailAndPassword({
    required String email,
    required String password,
  }) async {
    log.add('signIn:$email');
    return _FakeUserCredential();
  }

  @override
  Future<void> signOut() async {
    log.add('signOut');
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _FakeFirebaseMessaging implements FirebaseMessaging {
  _FakeFirebaseMessaging(this.log, {String? token}) : currentToken = token;

  final List<String> log;
  final StreamController<String> refreshController =
      StreamController<String>.broadcast();
  String? currentToken;

  void simulateTokenRefresh(String token) {
    currentToken = token;
    refreshController.add(token);
  }

  @override
  Stream<String> get onTokenRefresh => refreshController.stream;

  @override
  Future<String?> getToken({String? vapidKey}) async {
    log.add('getToken:$currentToken');
    return currentToken;
  }

  @override
  Future<void> deleteToken() async {
    log.add('deleteToken');
  }

  Future<void> close() => refreshController.close();

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _FakeSecureStorage implements FlutterSecureStorage {
  _FakeSecureStorage(this.log);

  final List<String> log;
  final Map<String, String> values = {};

  @override
  Future<String?> read({
    required String key,
    IOSOptions? iOptions,
    AndroidOptions? aOptions,
    LinuxOptions? lOptions,
    WebOptions? webOptions,
    MacOsOptions? mOptions,
    WindowsOptions? wOptions,
  }) async =>
      values[key];

  @override
  Future<void> write({
    required String key,
    required String? value,
    IOSOptions? iOptions,
    AndroidOptions? aOptions,
    LinuxOptions? lOptions,
    WebOptions? webOptions,
    MacOsOptions? mOptions,
    WindowsOptions? wOptions,
  }) async {
    if (value == null) {
      values.remove(key);
    } else {
      values[key] = value;
    }
  }

  @override
  Future<void> delete({
    required String key,
    IOSOptions? iOptions,
    AndroidOptions? aOptions,
    LinuxOptions? lOptions,
    WebOptions? webOptions,
    MacOsOptions? mOptions,
    WindowsOptions? wOptions,
  }) async {
    log.add('storageDelete:$key');
    values.remove(key);
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _RegisteredFcmToken {
  const _RegisteredFcmToken({
    required this.firebaseIdToken,
    required this.token,
    required this.platform,
  });

  final String firebaseIdToken;
  final String token;
  final String platform;
}

class _FakeAuthApiClient implements AuthApiClient {
  _FakeAuthApiClient(this.log);

  final List<String> log;
  final List<_RegisteredFcmToken> registrations = [];
  int sessionCounter = 0;

  /// When set, every [registerFcmToken] call throws this error.
  DioException? registerError;

  /// When set, [unregisterFcmToken] throws this error.
  DioException? unregisterError;

  @override
  Future<BackendSessionResponse> createSession({
    required String firebaseIdToken,
    CreateSessionRequest? request,
  }) async {
    sessionCounter++;
    log.add('createSession:$firebaseIdToken');
    return BackendSessionResponse(
      sessionId: 'session-$sessionCounter',
      expiresAt: DateTime(2030),
      user: BackendUser(
        id: 'user-$sessionCounter',
        firebaseUid: 'uid-$sessionCounter',
        preferredLanguage: 'ar',
        createdAt: DateTime(2024),
      ),
    );
  }

  @override
  Future<void> registerFcmToken({
    required String firebaseIdToken,
    required String token,
    required String platform,
  }) async {
    log.add('registerFcmToken:$firebaseIdToken|$token');
    registrations.add(_RegisteredFcmToken(
      firebaseIdToken: firebaseIdToken,
      token: token,
      platform: platform,
    ));
    if (registerError case final error?) {
      throw error;
    }
  }

  @override
  Future<void> unregisterFcmToken({
    required String firebaseIdToken,
    required String token,
  }) async {
    log.add('unregisterFcmToken:$firebaseIdToken|$token');
    if (unregisterError case final error?) {
      throw error;
    }
  }

  @override
  Future<void> logout({
    required String firebaseIdToken,
    required String sessionId,
  }) async {
    log.add('logout:$sessionId');
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _Harness {
  _Harness({String? fcmToken = 'fcm-token-1', String idToken = 'id-token-1'}) {
    user = _FakeUser(log, idToken: idToken);
    messaging = _FakeFirebaseMessaging(log, token: fcmToken);
    auth = _FakeFirebaseAuth(log, user);
    storage = _FakeSecureStorage(log);
    api = _FakeAuthApiClient(log);
    controller = AuthController(
      apiClient: api,
      firebaseAuth: auth,
      firebaseMessaging: messaging,
      secureStorage: storage,
    );
  }

  final List<String> log = [];
  late final _FakeUser user;
  late final _FakeFirebaseAuth auth;
  late final _FakeFirebaseMessaging messaging;
  late final _FakeSecureStorage storage;
  late final _FakeAuthApiClient api;
  late final AuthController controller;

  /// Lets the constructor's `_init()` settle before interacting.
  Future<void> settle() => Future<void>.delayed(Duration.zero);

  Future<void> signIn({String email = 'user@example.com'}) async {
    await controller.signIn(email: email, password: 'password');
  }

  Future<void> dispose() async {
    controller.dispose();
    await messaging.close();
  }
}

DioException _backendError({int statusCode = 401}) => DioException(
      requestOptions: RequestOptions(path: '/auth/fcm-token'),
      response: Response(
        requestOptions: RequestOptions(path: '/auth/fcm-token'),
        statusCode: statusCode,
      ),
    );

void main() {
  test(
    'token refresh re-registers with the Firebase ID token as bearer and the '
    'FCM token as the device token',
    () async {
      final harness = _Harness(fcmToken: 'fcm-token-old');
      addTearDown(harness.dispose);
      await harness.settle();
      await harness.signIn();

      expect(harness.api.registrations, hasLength(1));
      harness.api.registrations.clear();
      harness.log.clear();

      harness.messaging.simulateTokenRefresh('fcm-token-new');
      // Allow the stream listener and the async registration to complete.
      await Future<void>.delayed(Duration.zero);
      await Future<void>.delayed(Duration.zero);

      expect(harness.api.registrations, hasLength(1));
      final registration = harness.api.registrations.single;
      expect(registration.firebaseIdToken, 'id-token-1');
      expect(registration.token, 'fcm-token-new');
      expect(registration.firebaseIdToken, isNot(registration.token));
      expect(registration.platform, 'android');
      expect(harness.log, contains('getIdToken:id-token-1'));
      expect(harness.log, contains('getToken:fcm-token-new'));
    },
  );

  test(
    'failed refresh re-registration is swallowed without touching auth state',
    () async {
      final harness = _Harness();
      addTearDown(harness.dispose);
      await harness.settle();
      await harness.signIn();
      final sessionId = harness.controller.state.sessionId;
      harness.api.registerError = _backendError();

      harness.messaging.simulateTokenRefresh('fcm-token-new');
      await Future<void>.delayed(Duration.zero);
      await Future<void>.delayed(Duration.zero);

      expect(harness.api.registrations, hasLength(2));
      expect(harness.controller.state.status, AuthStatus.authenticated);
      expect(harness.controller.state.sessionId, sessionId);
      expect(harness.controller.state.errorMessage, isNull);
      expect(harness.log, isNot(contains('signOut')));
    },
  );

  test(
    'logout unregisters the current FCM token before deleteToken and local '
    'sign-out',
    () async {
      final harness = _Harness(fcmToken: 'fcm-token-logout');
      addTearDown(harness.dispose);
      await harness.settle();
      await harness.signIn();
      harness.log.clear();

      await harness.controller.logout();

      final unregisterIndex = harness.log.indexOf(
        'unregisterFcmToken:id-token-1|fcm-token-logout',
      );
      final deleteTokenIndex = harness.log.indexOf('deleteToken');
      final signOutIndex = harness.log.indexOf('signOut');
      expect(unregisterIndex, isNonNegative);
      expect(deleteTokenIndex, greaterThan(unregisterIndex));
      expect(signOutIndex, greaterThan(deleteTokenIndex));
      expect(harness.controller.state.status, AuthStatus.unauthenticated);
      expect(harness.controller.state.sessionId, isNull);
      expect(harness.storage.values, isNot(contains('halaqaty_session_id')));
    },
  );

  test(
    'logout backend failure still clears the local session',
    () async {
      final harness = _Harness(fcmToken: 'fcm-token-logout');
      addTearDown(harness.dispose);
      await harness.settle();
      await harness.signIn();
      harness.api.unregisterError = _backendError(statusCode: 500);

      await harness.controller.logout();

      expect(harness.controller.state.status, AuthStatus.unauthenticated);
      expect(harness.controller.state.sessionId, isNull);
      expect(harness.log, contains('signOut'));
      expect(harness.storage.values, isNot(contains('halaqaty_session_id')));
    },
  );

  test(
    'account switch re-registers the device under the new identity token',
    () async {
      final harness = _Harness(fcmToken: 'fcm-token-shared');
      addTearDown(harness.dispose);
      await harness.settle();
      await harness.signIn(email: 'first@example.com');

      expect(harness.api.registrations.single.firebaseIdToken, 'id-token-1');
      expect(harness.api.registrations.single.token, 'fcm-token-shared');

      await harness.controller.logout();
      harness.api.registrations.clear();

      harness.user.idToken = 'id-token-2';
      await harness.signIn(email: 'second@example.com');

      expect(harness.api.registrations, hasLength(1));
      final registration = harness.api.registrations.single;
      expect(registration.firebaseIdToken, 'id-token-2');
      expect(registration.token, 'fcm-token-shared');
      expect(harness.controller.state.sessionId, 'session-2');
      expect(harness.storage.values['halaqaty_session_id'], 'session-2');
    },
  );
}
