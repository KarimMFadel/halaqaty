import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';

class _UnauthorizedCircleApiClient extends CircleApiClient {
  _UnauthorizedCircleApiClient() : super(Dio());

  @override
  Future<CircleResponse> getCircle({
    required String firebaseIdToken,
    required String sessionId,
    required String circleId,
  }) {
    final request = RequestOptions(path: '/circles/$circleId');
    throw DioException(
      requestOptions: request,
      response: Response<void>(requestOptions: request, statusCode: 401),
    );
  }
}

void main() {
  test('circleDetailProvider: logs out after an expired backend session',
      () async {
    var loggedOut = false;
    final container = ProviderContainer(
      overrides: [
        circleApiClientProvider.overrideWithValue(
          _UnauthorizedCircleApiClient(),
        ),
        circleCredentialsProvider.overrideWith(
          (_) async => (token: 'firebase-token', sessionId: 'session-id'),
        ),
        circleSessionLogoutProvider.overrideWithValue(() async {
          loggedOut = true;
        }),
      ],
    );
    addTearDown(container.dispose);

    await expectLater(
      container.read(circleDetailProvider('circle-1').future),
      throwsA(isA<DioException>()),
    );
    expect(loggedOut, isTrue);
  });
}
