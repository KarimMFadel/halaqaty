import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/presentation/auth_screens.dart';
import 'package:halaqaty_mobile/features/profile/application/profile_controller.dart';
import 'package:halaqaty_mobile/features/profile/data/profile_api_client.dart';
import 'package:halaqaty_mobile/features/profile/presentation/profile_screen.dart';
import 'package:integration_test/integration_test.dart';

class _IntegrationAuthNotifier extends StateNotifier<AuthState>
    implements AuthController {
  _IntegrationAuthNotifier()
      : super(const AuthState(status: AuthStatus.unauthenticated));

  bool signInCalled = false;

  @override
  Future<void> signIn({
    required String email,
    required String password,
  }) async {
    signInCalled = true;
    state = const AuthState(
      status: AuthStatus.authenticated,
      sessionId: 'session-123',
    );
  }

  @override
  Future<void> register({
    required String email,
    required String password,
    required String displayName,
    required String preferredLanguage,
  }) async {}

  @override
  Future<void> logout() async {
    state = const AuthState(status: AuthStatus.unauthenticated);
  }
}

class _IntegrationProfileNotifier extends StateNotifier<ProfileState>
    implements ProfileController {
  _IntegrationProfileNotifier() : super(const ProfileState());

  bool updateCalled = false;
  UpdateProfileRequest? lastRequest;

  @override
  Future<void> loadProfile() async {
    state = ProfileState(
      profile: ProfileUser(
        id: 'user-1',
        firebaseUid: 'firebase-1',
        fullName: 'Ali Mahmoud',
        displayName: 'Ali',
        bio: 'Old bio',
        country: 'EG',
        preferredLanguage: 'ar',
        avatarUrl: '',
        phone: '',
        createdAt: DateTime.parse('2026-01-01T00:00:00Z'),
      ),
    );
  }

  @override
  Future<bool> updateProfile({required UpdateProfileRequest request}) async {
    updateCalled = true;
    lastRequest = request;
    state = state.copyWith(
      profile: ProfileUser(
        id: 'user-1',
        firebaseUid: 'firebase-1',
        fullName: request.fullName,
        displayName: request.displayName ?? 'Ali',
        bio: request.bio,
        country: request.country,
        preferredLanguage: request.preferredLanguage ?? 'ar',
        avatarUrl: request.avatarUrl,
        phone: request.phone,
        createdAt: DateTime.parse('2026-01-01T00:00:00Z'),
      ),
    );
    return true;
  }
}

class _FlowApp extends ConsumerWidget {
  const _FlowApp();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final authState = ref.watch(authControllerProvider);
    if (!authState.isAuthenticated) {
      return const LoginScreen();
    }
    return const ProfileScreen();
  }
}

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('login then view/edit profile flow', (tester) async {
    final authNotifier = _IntegrationAuthNotifier();
    final profileNotifier = _IntegrationProfileNotifier();

    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authControllerProvider.overrideWith((_) => authNotifier),
          profileControllerProvider.overrideWith((_) => profileNotifier),
        ],
        child: const MaterialApp(home: _FlowApp()),
      ),
    );

    await tester.enterText(find.byKey(const Key('emailField')), 'a@b.com');
    await tester.enterText(
        find.byKey(const Key('passwordField')), 'password123');
    await tester.tap(find.byKey(const Key('submitButton')));
    await tester.pumpAndSettle();

    expect(authNotifier.signInCalled, isTrue);
    expect(find.byKey(const Key('profileSaveButton')), findsOneWidget);
    expect(find.text('Ali Mahmoud'), findsOneWidget);

    await tester.enterText(
      find.byKey(const Key('profileFullNameField')),
      'Ahmad Kareem',
    );
    await tester.enterText(find.byKey(const Key('profileCountryField')), 'sa');
    await tester.enterText(
      find.byKey(const Key('profileBioField')),
      'Updated bio',
    );
    await tester.tap(find.byKey(const Key('profileSaveButton')));
    await tester.pumpAndSettle();

    expect(profileNotifier.updateCalled, isTrue);
    expect(profileNotifier.lastRequest?.fullName, 'Ahmad Kareem');
    expect(profileNotifier.lastRequest?.country, 'SA');
    expect(profileNotifier.lastRequest?.bio, 'Updated bio');
  });
}
