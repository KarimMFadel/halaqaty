import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/admin/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';

void main() {
  test('dioProvider defaults to the versioned API base URL', () {
    final container = ProviderContainer();
    addTearDown(container.dispose);

    expect(
      container.read(dioProvider).options.baseUrl,
      'http://localhost:8080/api/v1',
    );
  });

  test('CircleApiClient sends the Firebase bearer token', () async {
    final dio = Dio(BaseOptions(baseUrl: 'http://localhost:8080/api/v1'));
    dio.interceptors.add(
      InterceptorsWrapper(
        onRequest: (options, handler) {
          expect(options.method, 'POST');
          expect(
            options.uri.toString(),
            'http://localhost:8080/api/v1/circles',
          );
          expect(options.headers['Authorization'], 'Bearer firebase-token');
          handler.reject(
            DioException(
              requestOptions: options,
              type: DioExceptionType.cancel,
            ),
          );
        },
      ),
    );

    final client = CircleApiClient(dio);

    await expectLater(
      client.createCircle(
        sessionId: 'session-id',
        firebaseIdToken: 'firebase-token',
        request: const CreateCircleRequest(name: 'My circle'),
      ),
      throwsA(isA<DioException>()),
    );
  });
}
