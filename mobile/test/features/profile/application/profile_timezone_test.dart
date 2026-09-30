import 'package:dio/dio.dart';
import 'package:firebase_auth/firebase_auth.dart' as firebase;
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/profile/application/profile_controller.dart';
import 'package:halaqaty_mobile/features/profile/data/profile_api_client.dart';

void main() {
  group('ProfileUser timezone parsing', () {
    test('reads the stored timezone when present', () {
      final profile =
          ProfileUser.fromJson(_profileJson(timezone: 'Africa/Cairo'));

      expect(profile.timezone, 'Africa/Cairo');
    });

    test('an old payload without a timezone falls back to UTC', () {
      final profile = ProfileUser.fromJson(_profileJson());

      expect(profile.timezone, 'UTC');
    });

    test('an explicit null timezone falls back to UTC', () {
      final profile = ProfileUser.fromJson(_profileJson()..['timezone'] = null);

      expect(profile.timezone, 'UTC');
    });
  });

  group('BackendUser timezone parsing', () {
    test('reads the stored timezone when present', () {
      final user =
          BackendUser.fromJson(_backendUserJson(timezone: 'Asia/Riyadh'));

      expect(user.timezone, 'Asia/Riyadh');
    });

    test('an old payload without a timezone falls back to UTC', () {
      final user = BackendUser.fromJson(_backendUserJson());

      expect(user.timezone, 'UTC');
    });
  });

  group('UpdateProfileRequest timezone', () {
    test('includes the timezone when set', () {
      const request = UpdateProfileRequest(timezone: 'Africa/Cairo');

      expect(request.toJson()['timezone'], 'Africa/Cairo');
    });

    test('omits the timezone when unset so the stored value is preserved', () {
      const request = UpdateProfileRequest(displayName: 'Ali');

      expect(request.toJson().containsKey('timezone'), isFalse);
    });
  });

  group('ProfileApiClient timezone wire shape', () {
    test('getMe tolerates an old payload without a timezone', () async {
      final dio = _stubDio(body: _profileJson());

      final profile = await ProfileApiClient(dio).getMe(
        firebaseIdToken: 'token',
        sessionId: 'session',
      );

      expect(profile.timezone, 'UTC');
    });

    test('getMe reads the stored timezone', () async {
      final dio = _stubDio(body: _profileJson(timezone: 'Africa/Cairo'));

      final profile = await ProfileApiClient(dio).getMe(
        firebaseIdToken: 'token',
        sessionId: 'session',
      );

      expect(profile.timezone, 'Africa/Cairo');
    });

    test('updateMe sends the timezone key when supplied', () async {
      RequestOptions? sent;
      final dio = _stubDio(
        body: _profileJson(timezone: 'Europe/Berlin'),
        onRequest: (options) => sent = options,
      );

      final profile = await ProfileApiClient(dio).updateMe(
        firebaseIdToken: 'token',
        sessionId: 'session',
        request: const UpdateProfileRequest(timezone: 'Europe/Berlin'),
      );

      expect(sent?.method, 'PUT');
      expect((sent?.data as Map<String, dynamic>)['timezone'], 'Europe/Berlin');
      expect(profile.timezone, 'Europe/Berlin');
    });
  });

  group('ProfileController timezone', () {
    test('updateProfile passes the requested timezone to the API', () async {
      final api = _RecordingProfileApi();
      String? updatedTimezone;
      final controller = ProfileController(
        apiClient: api,
        firebaseAuth: _FakeFirebaseAuth(_FakeFirebaseUser()),
        readAuthState: () => const AuthState(
          status: AuthStatus.authenticated,
          sessionId: 'session-1',
        ),
        logout: () async {},
        onTimezoneSaved: (timezone) => updatedTimezone = timezone,
      );
      addTearDown(controller.dispose);

      final saved = await controller.updateProfile(
        request: const UpdateProfileRequest(timezone: 'Africa/Cairo'),
      );

      expect(saved, isTrue);
      expect(api.lastRequest?.timezone, 'Africa/Cairo');
      expect(controller.state.profile?.timezone, 'Africa/Cairo');
      expect(updatedTimezone, 'Africa/Cairo');
    });
  });
}

Map<String, dynamic> _profileJson({String? timezone}) => {
      'id': 'user-1',
      'firebase_uid': 'firebase-1',
      'full_name': 'Ali Mahmoud',
      'display_name': 'Ali',
      'bio': null,
      'country': 'EG',
      'preferred_language': 'ar',
      'avatar_url': null,
      'phone': null,
      'created_at': '2026-01-01T00:00:00Z',
      if (timezone != null) 'timezone': timezone,
    };

Map<String, dynamic> _backendUserJson({String? timezone}) => {
      'id': 'user-1',
      'firebase_uid': 'firebase-1',
      'display_name': 'Ali',
      'preferred_language': 'ar',
      'created_at': '2026-01-01T00:00:00Z',
      if (timezone != null) 'timezone': timezone,
    };

Dio _stubDio({
  required Map<String, dynamic> body,
  void Function(RequestOptions)? onRequest,
}) {
  final dio = Dio(BaseOptions(baseUrl: 'http://localhost'));
  dio.interceptors.add(
    InterceptorsWrapper(
      onRequest: (options, handler) {
        onRequest?.call(options);
        handler.resolve(
          Response<Map<String, dynamic>>(
            requestOptions: options,
            statusCode: 200,
            data: body,
          ),
        );
      },
    ),
  );
  return dio;
}

class _RecordingProfileApi implements ProfileApiClient {
  UpdateProfileRequest? lastRequest;

  @override
  Future<ProfileUser> updateMe({
    required String firebaseIdToken,
    required String sessionId,
    required UpdateProfileRequest request,
  }) async {
    lastRequest = request;
    return ProfileUser.fromJson(
      _profileJson(timezone: request.timezone ?? 'UTC'),
    );
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _FakeFirebaseUser implements firebase.User {
  @override
  Future<String?> getIdToken([bool forceRefresh = false]) async => 'token';

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

class _FakeFirebaseAuth implements firebase.FirebaseAuth {
  _FakeFirebaseAuth(this.currentUser);

  @override
  final firebase.User? currentUser;

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}
