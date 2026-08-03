import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/profile/application/profile_controller.dart';
import 'package:halaqaty_mobile/features/profile/data/profile_api_client.dart';
import 'package:halaqaty_mobile/features/profile/presentation/profile_screen.dart';

class _StubProfileNotifier extends StateNotifier<ProfileState>
    implements ProfileController {
  _StubProfileNotifier({
    this.serverFieldErrors = const {},
  }) : super(const ProfileState());

  final Map<String, String> serverFieldErrors;
  bool updateCalled = false;

  @override
  Future<void> loadProfile() async {
    state = ProfileState(
      profile: ProfileUser(
        id: 'user-1',
        firebaseUid: 'firebase-1',
        fullName: 'Ali Mahmoud',
        displayName: 'Ali',
        bio: 'Bio',
        country: 'EG',
        preferredLanguage: 'ar',
        avatarUrl: null,
        phone: null,
        createdAt: DateTime.parse('2026-01-01T00:00:00Z'),
      ),
    );
  }

  @override
  Future<bool> updateProfile({required UpdateProfileRequest request}) async {
    updateCalled = true;
    if (serverFieldErrors.isNotEmpty) {
      state = state.copyWith(
        isSaving: false,
        errorMessage: 'validation failed',
        fieldErrors: serverFieldErrors,
      );
      return false;
    }
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

Widget _buildScreen(_StubProfileNotifier stub) {
  return ProviderScope(
    overrides: [profileControllerProvider.overrideWith((_) => stub)],
    child: const MaterialApp(home: ProfileScreen()),
  );
}

void main() {
  group('ProfileScreen form behavior', () {
    testWidgets('full_name and country are required', (tester) async {
      final stub = _StubProfileNotifier();
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      await tester.enterText(find.byKey(const Key('profileFullNameField')), '');
      await tester.enterText(find.byKey(const Key('profileCountryField')), '');
      await tester.tap(find.byKey(const Key('profileSaveButton')));
      await tester.pump();

      expect(find.text('Full name is required'), findsOneWidget);
      expect(find.text('Country is required'), findsOneWidget);
      expect(stub.updateCalled, isFalse);
    });

    testWidgets('maps server error.fields into field-level messages',
        (tester) async {
      final stub = _StubProfileNotifier(
        serverFieldErrors: const {
          'full_name': 'full_name is required for first profile completion',
          'country': 'country must be a 2-letter ISO country code',
        },
      );

      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      await tester.enterText(
          find.byKey(const Key('profileFullNameField')), 'Ali Mahmoud');
      await tester.enterText(find.byKey(const Key('profileCountryField')), 'EG');
      await tester.tap(find.byKey(const Key('profileSaveButton')));
      await tester.pump();

      expect(stub.updateCalled, isTrue);
      expect(
        find.text('full_name is required for first profile completion'),
        findsOneWidget,
      );
      expect(
        find.text('country must be a 2-letter ISO country code'),
        findsOneWidget,
      );
    });
  });
}
