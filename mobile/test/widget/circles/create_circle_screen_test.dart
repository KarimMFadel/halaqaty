import 'package:dio/dio.dart';
import 'package:firebase_auth/firebase_auth.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/create_circle_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/create_circle_screen.dart';

class _StubCircleApiClient extends CircleApiClient {
  _StubCircleApiClient({
    this.searchResults = const [],
    this.createdCircle,
  }) : super(Dio());

  bool submitCalled = false;
  CreateCircleRequest? submittedRequest;
  final List<CircleUser> searchResults;
  CircleResponse? createdCircle;
  DioException? createError;

  @override
  Future<CircleResponse> createCircle({
    required String firebaseIdToken,
    required String sessionId,
    required CreateCircleRequest request,
  }) async {
    submitCalled = true;
    submittedRequest = request;
    if (createError case final error?) throw error;
    return createdCircle ??
        CircleResponse(
          id: 'circle-1',
          name: request.name,
          inviteCode: 'HLQ-7X2K',
          inviteLink: 'https://halaqaty.app/join/HLQ-7X2K',
          createdAt: DateTime.utc(2026),
        );
  }

  @override
  Future<List<CircleUser>> searchUsers({
    required String firebaseIdToken,
    required String sessionId,
    required String query,
  }) async => searchResults;
}

CreateCircleController _controller(
  _StubCircleApiClient apiClient, {
  Future<String?> Function()? loadFirebaseIdToken,
}) {
  return CreateCircleController(
    apiClient: apiClient,
    loadFirebaseIdToken: loadFirebaseIdToken ?? () async => 'firebase-token',
    readAuthState: () => const AuthState(sessionId: 'session-id'),
    logout: () async {},
  );
}

Widget _buildScreen(CreateCircleController controller) {
  return ProviderScope(
    overrides: [createCircleControllerProvider.overrideWith((_) => controller)],
    child: MaterialApp(
      home: Directionality(
        textDirection: TextDirection.rtl,
        child: const CreateCircleScreen(),
      ),
    ),
  );
}

Future<void> _tapVisible(WidgetTester tester, Finder finder) async {
  await tester.dragUntilVisible(
    finder,
    find.byType(SingleChildScrollView),
    const Offset(0, -200),
  );
  await tester.pumpAndSettle();
  await tester.tap(finder);
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('requires a circle name before submitting', (tester) async {
    final apiClient = _StubCircleApiClient();
    final controller = _controller(apiClient);
    await tester.pumpWidget(_buildScreen(controller));

    await _tapVisible(
      tester,
      find.byKey(const Key('createCircleSubmitButton')),
    );

    expect(find.text('اسم الحلقة مطلوب'), findsOneWidget);
    expect(apiClient.submitCalled, isFalse);
  });

  testWidgets('renders Arabic labels in RTL and submits valid settings',
      (tester) async {
    final apiClient = _StubCircleApiClient();
    final controller = _controller(apiClient);
    await tester.pumpWidget(_buildScreen(controller));

    await tester.enterText(
      find.byKey(const Key('createCircleNameField')),
      'حلقة الحفظ',
    );
    await tester.ensureVisible(
      find.byKey(const Key('createCircleGenderField')),
    );
    await tester.tap(find.byKey(const Key('createCircleGenderField')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('مختلط').last);
    await tester.pumpAndSettle();
    await _tapVisible(
      tester,
      find.byKey(const Key('createCirclePrivateField')),
    );
    await _tapVisible(
      tester,
      find.byKey(const Key('createCircleSubmitButton')),
    );

    expect(find.text('إنشاء حلقة'), findsOneWidget);
    expect(find.text('مختلط'), findsOneWidget);
    expect(find.text('حلقة خاصة'), findsOneWidget);
    expect(apiClient.submitCalled, isTrue);
    expect(apiClient.submittedRequest?.language, 'ar');
    expect(apiClient.submittedRequest?.genderRestriction, 'mixed');
    expect(apiClient.submittedRequest?.isPrivate, isTrue);
  });

  testWidgets('blocks invalid capacity before submission', (tester) async {
    final apiClient = _StubCircleApiClient();
    final controller = _controller(apiClient);
    await tester.pumpWidget(_buildScreen(controller));

    await tester.enterText(find.byKey(const Key('createCircleNameField')), 'Circle');
    await tester.enterText(
      find.byKey(const Key('createCircleCapacityField')),
      '1',
    );
    await _tapVisible(
      tester,
      find.byKey(const Key('createCircleSubmitButton')),
    );

    expect(find.text('السعة بين 2 و200'), findsOneWidget);
    expect(apiClient.submitCalled, isFalse);
  });

  testWidgets('maps server field errors onto the form', (tester) async {
    final apiClient = _StubCircleApiClient()
      ..createError = DioException(
        requestOptions: RequestOptions(path: '/circles'),
        response: Response<Map<String, dynamic>>(
          requestOptions: RequestOptions(path: '/circles'),
          statusCode: 400,
          data: const {
            'error': {
              'message': 'Validation failed',
              'fields': {'name': 'This name is unavailable'},
            },
          },
        ),
      );
    final controller = _controller(apiClient);
    await tester.pumpWidget(_buildScreen(controller));

    await tester.enterText(
      find.byKey(const Key('createCircleNameField')),
      'Circle',
    );
    await _tapVisible(
      tester,
      find.byKey(const Key('createCircleSubmitButton')),
    );

    expect(find.text('This name is unavailable'), findsOneWidget);
  });

  testWidgets('adds a searched teacher to the create request', (tester) async {
    final apiClient = _StubCircleApiClient(
      searchResults: const [CircleUser(id: 'teacher-1', displayName: 'Aisha')],
    );
    final controller = _controller(apiClient);
    await tester.pumpWidget(_buildScreen(controller));

    await tester.enterText(find.byKey(const Key('createCircleUserSearchField')), 'Ai');
    await tester.pump();
    await tester.tap(find.text('معلّم'));
    await tester.enterText(find.byKey(const Key('createCircleNameField')), 'Circle');
    await _tapVisible(
      tester,
      find.byKey(const Key('createCircleSubmitButton')),
    );

    expect(apiClient.submittedRequest?.teacherUserIds, ['teacher-1']);
  });

  testWidgets('shows the invite link after creation', (tester) async {
    final apiClient = _StubCircleApiClient(
      createdCircle: CircleResponse(
        id: 'circle-1',
        name: 'Circle',
        inviteCode: 'HLQ-7X2K',
        inviteLink: 'https://halaqaty.app/join/HLQ-7X2K',
        createdAt: DateTime.utc(2026),
      ),
    );
    final controller = _controller(apiClient);
    await tester.pumpWidget(_buildScreen(controller));

    await tester.enterText(find.byKey(const Key('createCircleNameField')), 'Circle');
    await _tapVisible(
      tester,
      find.byKey(const Key('createCircleSubmitButton')),
    );

    expect(find.text('تم إنشاء الحلقة'), findsOneWidget);
    expect(find.text('https://halaqaty.app/join/HLQ-7X2K'), findsOneWidget);
    expect(find.text('نسخ رابط الدعوة'), findsOneWidget);

    await tester.tap(find.text('نسخ رابط الدعوة'));
    await tester.pump();
    expect(find.text('تم نسخ رابط الدعوة'), findsOneWidget);
  });

  test('failed retry clears the previous circle result', () async {
    final apiClient = _StubCircleApiClient();
    final controller = _controller(apiClient);

    expect(
      await controller.create(const CreateCircleRequest(name: 'First')),
      isTrue,
    );
    apiClient.createError = DioException(
      requestOptions: RequestOptions(path: '/circles'),
    );

    expect(
      await controller.create(const CreateCircleRequest(name: 'Second')),
      isFalse,
    );
    expect(controller.state.circle, isNull);
  });

  test('user search handles Firebase token refresh failure', () async {
    final controller = _controller(
      _StubCircleApiClient(),
      loadFirebaseIdToken: () async => throw FirebaseAuthException(
        code: 'token-refresh-failed',
        message: 'Token refresh failed',
      ),
    );

    expect(await controller.searchUsers('Ai'), isEmpty);
    expect(controller.state.errorMessage, 'Token refresh failed');
  });
}
