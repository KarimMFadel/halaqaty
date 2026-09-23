import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/app/app_locale_controller.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/profile/application/profile_controller.dart';
import 'package:halaqaty_mobile/features/profile/data/profile_api_client.dart';
import 'package:halaqaty_mobile/features/profile/presentation/profile_screen.dart';
import '../../helpers/stub_auth_notifier.dart';

class _StubProfileNotifier extends StateNotifier<ProfileState>
    implements ProfileController {
  _StubProfileNotifier({
    this.serverFieldErrors = const {},
    this.loadErrorMessage,
    this.hangLoad = false,
    this.saveErrorMessage,
    String preferredLanguage = 'ar',
  })  : _preferredLanguage = preferredLanguage,
        super(const ProfileState());

  final Map<String, String> serverFieldErrors;

  /// When set, [loadProfile] finishes with this general error and no profile.
  final String? loadErrorMessage;

  /// When true, [loadProfile] holds the loading state forever.
  final bool hangLoad;

  /// When set, [updateProfile] fails with this general (non-field) error.
  final String? saveErrorMessage;

  final String _preferredLanguage;

  bool updateCalled = false;
  int loadCalls = 0;

  @override
  Future<void> loadProfile() async {
    loadCalls++;
    if (hangLoad) {
      state = state.copyWith(isLoading: true);
      return Completer<void>().future;
    }
    final loadError = loadErrorMessage;
    if (loadError != null) {
      state = state.copyWith(isLoading: false, errorMessage: loadError);
      return;
    }
    state = ProfileState(
      profile: _profile(_preferredLanguage),
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
    final saveError = saveErrorMessage;
    if (saveError != null) {
      state = state.copyWith(isSaving: false, errorMessage: saveError);
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

  static ProfileUser _profile(String preferredLanguage) => ProfileUser(
        id: 'user-1',
        firebaseUid: 'firebase-1',
        fullName: 'Ali Mahmoud',
        displayName: 'Ali',
        bio: 'Bio',
        country: 'EG',
        preferredLanguage: preferredLanguage,
        avatarUrl: null,
        phone: null,
        createdAt: DateTime.parse('2026-01-01T00:00:00Z'),
      );
}

/// Records logout so the account action can be asserted without Firebase.
class _RecordingAuthNotifier extends StubAuthNotifier {
  _RecordingAuthNotifier()
      : super(
          initialState: const AuthState(
            status: AuthStatus.authenticated,
            sessionId: 'session-1',
          ),
        );

  bool logoutCalled = false;

  @override
  Future<void> logout() async {
    logoutCalled = true;
  }
}

Widget _buildScreen(_StubProfileNotifier stub, {StubAuthNotifier? auth}) {
  return ProviderScope(
    overrides: [
      profileControllerProvider.overrideWith((_) => stub),
      // ProfileScreen embeds LogoutButton, which reads the auth controller;
      // stub it so no Firebase initialization is required.
      authControllerProvider.overrideWith(
        (_) =>
            auth ??
            StubAuthNotifier(
                initialState: const AuthState(
              status: AuthStatus.authenticated,
              sessionId: 'session-1',
            )),
      ),
    ],
    child: const MaterialApp(home: ProfileScreen()),
  );
}

/// Mirrors `MyApp`: the root direction follows [appLocaleControllerProvider],
/// so a loaded/saved profile language drives RTL/LTR (FR-029).
Widget _buildLocalizedScreen(
  _StubProfileNotifier stub, {
  Locale platformLocale = const Locale('ar'),
}) {
  return ProviderScope(
    overrides: [
      platformLocaleProvider.overrideWithValue(platformLocale),
      profileControllerProvider.overrideWith((_) => stub),
      authControllerProvider.overrideWith(
        (_) => StubAuthNotifier(
            initialState: const AuthState(
          status: AuthStatus.authenticated,
          sessionId: 'session-1',
        )),
      ),
    ],
    child: Consumer(
      builder: (context, ref, _) {
        final locale = ref.watch(appLocaleControllerProvider);
        return MaterialApp(
          builder: (context, child) => Directionality(
            textDirection: locale.languageCode == 'ar'
                ? TextDirection.rtl
                : TextDirection.ltr,
            child: child!,
          ),
          home: const ProfileScreen(),
        );
      },
    ),
  );
}

TextField _field(WidgetTester tester, Key key) => tester.widget<TextField>(
      find.descendant(of: find.byKey(key), matching: find.byType(TextField)),
    );

void main() {
  group('ProfileScreen form behavior', () {
    testWidgets('optional profile fields explain that they are optional',
        (tester) async {
      await tester.pumpWidget(_buildScreen(_StubProfileNotifier()));
      await tester.pump();

      for (final key in [
        const Key('profileBioField'),
        const Key('profileAvatarUrlField'),
        const Key('profilePhoneField'),
      ]) {
        final decorator = tester.widget<InputDecorator>(
          find.descendant(
            of: find.byKey(key),
            matching: find.byType(InputDecorator),
          ),
        );
        expect(decorator.decoration.hintText, 'Optional');
      }
    });

    testWidgets('full_name and country are required', (tester) async {
      final stub = _StubProfileNotifier();
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      await tester.enterText(find.byKey(const Key('profileFullNameField')), '');
      await tester.enterText(find.byKey(const Key('profileCountryField')), '');
      await tester.ensureVisible(find.byKey(const Key('profileSaveButton')));
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
      await tester.enterText(
          find.byKey(const Key('profileCountryField')), 'EG');
      await tester.ensureVisible(find.byKey(const Key('profileSaveButton')));
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

    testWidgets('shows save success feedback after a successful update',
        (tester) async {
      await tester.pumpWidget(_buildScreen(_StubProfileNotifier()));
      await tester.pump();

      await tester.ensureVisible(find.byKey(const Key('profileSaveButton')));
      await tester.tap(find.byKey(const Key('profileSaveButton')));
      await tester.pumpAndSettle();

      expect(find.byKey(const Key('profileSaveSuccess')), findsOneWidget);
      expect(find.text('Profile updated'), findsOneWidget);
    });
  });

  group('ProfileScreen — US5 structure and states', () {
    testWidgets(
        'groups existing fields under profile, preferences, and '
        'account sections', (tester) async {
      await tester.pumpWidget(_buildScreen(_StubProfileNotifier()));
      await tester.pump();

      expect(find.text('Profile details'), findsOneWidget);
      expect(find.text('Preferences'), findsOneWidget);
      expect(find.text('Account & support'), findsOneWidget);
    });

    testWidgets('preserves every existing field key', (tester) async {
      await tester.pumpWidget(_buildScreen(_StubProfileNotifier()));
      await tester.pump();

      for (final key in [
        const Key('profileFullNameField'),
        const Key('profileDisplayNameField'),
        const Key('profileBioField'),
        const Key('profileCountryField'),
        const Key('profileLanguageDropdown'),
        const Key('profileAvatarUrlField'),
        const Key('profilePhoneField'),
        const Key('profileSaveButton'),
        const Key('logoutButton'),
      ]) {
        expect(find.byKey(key), findsOneWidget, reason: 'missing $key');
      }
    });

    testWidgets('shows branded loading while the profile loads',
        (tester) async {
      await tester
          .pumpWidget(_buildScreen(_StubProfileNotifier(hangLoad: true)));
      await tester.pump();

      expect(find.byType(HalaqatyLoading), findsOneWidget);
    });

    testWidgets('load failure explains the error and offers retry',
        (tester) async {
      final stub = _StubProfileNotifier(
        loadErrorMessage: 'We cannot reach the server right now',
      );
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      expect(
        find.text('We cannot reach the server right now'),
        findsOneWidget,
      );
      expect(find.text('Retry'), findsOneWidget);
      expect(stub.loadCalls, 1);

      await tester.tap(find.text('Retry'));
      await tester.pump();

      expect(stub.loadCalls, 2);
    });

    testWidgets('English preferred language renders the form LTR',
        (tester) async {
      // English profile wins over the Arabic platform fallback (FR-029).
      await tester.pumpWidget(_buildLocalizedScreen(
        _StubProfileNotifier(preferredLanguage: 'en'),
      ));
      await tester.pump();

      expect(
        Directionality.of(tester.element(find.byType(ProfileScreen))),
        TextDirection.ltr,
      );
      expect(find.text('Full Name'), findsOneWidget);
    });

    testWidgets('Arabic preferred language renders the form RTL',
        (tester) async {
      await tester.pumpWidget(_buildLocalizedScreen(
        _StubProfileNotifier(preferredLanguage: 'ar'),
      ));
      await tester.pump();

      expect(
        Directionality.of(tester.element(find.byType(ProfileScreen))),
        TextDirection.rtl,
      );
      expect(find.text('الاسم الكامل'), findsOneWidget);
    });

    testWidgets(
        'save success remains visible in context, not only in a '
        'transient snackbar', (tester) async {
      await tester.pumpWidget(_buildScreen(_StubProfileNotifier()));
      await tester.pump();

      await tester.ensureVisible(find.byKey(const Key('profileSaveButton')));
      await tester.tap(find.byKey(const Key('profileSaveButton')));
      await tester.pumpAndSettle();

      expect(find.byKey(const Key('profileSaveSuccess')), findsOneWidget);

      // Well past any snackbar duration (incl. exit animation), the
      // confirmation must remain in context (FR-008).
      await tester.pump(const Duration(seconds: 5));
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('profileSaveSuccess')), findsOneWidget);
    });

    testWidgets('save failure keeps edits and explains the error in context',
        (tester) async {
      final stub = _StubProfileNotifier(
        saveErrorMessage: 'Could not save. Check your connection and retry.',
      );
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      await tester.enterText(
          find.byKey(const Key('profileFullNameField')), 'Ali Hassan');
      await tester.ensureVisible(find.byKey(const Key('profileSaveButton')));
      await tester.tap(find.byKey(const Key('profileSaveButton')));
      await tester.pump();

      expect(stub.updateCalled, isTrue);
      expect(
        find.text('Could not save. Check your connection and retry.'),
        findsOneWidget,
      );
      expect(
        _field(tester, const Key('profileFullNameField')).controller?.text,
        'Ali Hassan',
      );
    });

    testWidgets(
        'save is the single dominant filled action and save/logout '
        'meet 48dp', (tester) async {
      await tester.pumpWidget(_buildScreen(_StubProfileNotifier()));
      await tester.pump();

      expect(find.byType(FilledButton), findsOneWidget);
      expect(
        tester.widget(find.byKey(const Key('profileSaveButton'))),
        isA<FilledButton>(),
      );
      expect(
        tester.getSize(find.byKey(const Key('profileSaveButton'))).height,
        greaterThanOrEqualTo(48),
      );
      expect(
        tester.getSize(find.byKey(const Key('logoutButton'))).height,
        greaterThanOrEqualTo(48),
      );
    });

    testWidgets('keyboard flow follows the profile field order',
        (tester) async {
      await tester.pumpWidget(_buildScreen(_StubProfileNotifier()));
      await tester.pump();

      expect(
        _field(tester, const Key('profileFullNameField')).textInputAction,
        TextInputAction.next,
      );
      expect(
        _field(tester, const Key('profileDisplayNameField')).textInputAction,
        TextInputAction.next,
      );
      expect(
        _field(tester, const Key('profileCountryField')).textInputAction,
        TextInputAction.next,
      );
      expect(
        _field(tester, const Key('profileAvatarUrlField')).textInputAction,
        TextInputAction.next,
      );
      expect(
        _field(tester, const Key('profilePhoneField')).textInputAction,
        TextInputAction.done,
      );
    });

    testWidgets('save button exposes its label to screen readers',
        (tester) async {
      await tester.pumpWidget(_buildScreen(_StubProfileNotifier()));
      await tester.pump();

      final semantics =
          tester.getSemantics(find.byKey(const Key('profileSaveButton')));
      expect(semantics.label, 'Save');
    });

    testWidgets(
        'logout stays in the account section below the save action '
        'and completes on tap', (tester) async {
      final auth = _RecordingAuthNotifier();
      await tester.pumpWidget(_buildScreen(_StubProfileNotifier(), auth: auth));
      await tester.pump();

      final saveY =
          tester.getCenter(find.byKey(const Key('profileSaveButton'))).dy;
      final accountHeaderY =
          tester.getCenter(find.text('Account & support')).dy;
      final logoutY =
          tester.getCenter(find.byKey(const Key('logoutButton'))).dy;
      expect(accountHeaderY, greaterThan(saveY));
      expect(logoutY, greaterThan(accountHeaderY));

      await tester.ensureVisible(find.byKey(const Key('logoutButton')));
      await tester.tap(find.byKey(const Key('logoutButton')));
      await tester.pump();

      expect(auth.logoutCalled, isTrue);
    });

    testWidgets(
        'planned settings actions show only the shared '
        'under-implementation notice', (tester) async {
      final stub = _StubProfileNotifier();
      await tester.pumpWidget(_buildScreen(stub));
      await tester.pump();

      for (final label in [
        'Appearance',
        'Notifications',
        'Help & support',
        'Privacy & security',
      ]) {
        await tester.ensureVisible(find.text(label));
        await tester.tap(find.text(label));
        await tester.pump();

        expect(
          find.byKey(const Key('halaqatyUnderImplementationSnackBar')),
          findsOneWidget,
          reason: '$label must surface the shared notice',
        );
        expect(
          find.text(
            'This feature is under implementation and is not available yet.',
          ),
          findsOneWidget,
        );
        // No navigation, no controller call, no state mutation (FR-032).
        expect(find.byType(ProfileScreen), findsOneWidget);
        expect(stub.updateCalled, isFalse);

        // Let the notice expire before the next action.
        await tester.pump(const Duration(seconds: 5));
        await tester.pump();
      }
    });

    testWidgets('the shared notice uses the exact Arabic copy in RTL',
        (tester) async {
      await tester.pumpWidget(
        _buildLocalizedScreen(_StubProfileNotifier()),
      );
      await tester.pump();

      await tester.ensureVisible(find.text('المظهر'));
      await tester.tap(find.text('المظهر'));
      await tester.pump();

      expect(
        find.byKey(const Key('halaqatyUnderImplementationSnackBar')),
        findsOneWidget,
      );
      expect(
        find.text('هذه الميزة قيد التنفيذ وغير متاحة حالياً.'),
        findsOneWidget,
      );
    });

    testWidgets(
        'omitted avatar-upload and account-deletion actions stay '
        'absent', (tester) async {
      await tester.pumpWidget(_buildScreen(_StubProfileNotifier()));
      await tester.pump();

      expect(find.text('Change photo'), findsNothing);
      expect(find.text('تغيير الصورة'), findsNothing);
      expect(find.text('Delete account'), findsNothing);
      expect(find.text('حذف الحساب'), findsNothing);
    });

    testWidgets('large text on a compact width keeps the form usable',
        (tester) async {
      tester.view.physicalSize = const Size(320, 720);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      await tester.pumpWidget(
        MediaQuery(
          data: const MediaQueryData(textScaler: TextScaler.linear(2)),
          child: _buildScreen(_StubProfileNotifier()),
        ),
      );
      await tester.pump();

      // Overflow throws in the test framework; reaching the save action and
      // completing a save proves the layout survives 200% text at 320dp.
      await tester.ensureVisible(find.byKey(const Key('profileSaveButton')));
      await tester.tap(find.byKey(const Key('profileSaveButton')));
      await tester.pumpAndSettle();

      expect(find.byKey(const Key('profileSaveSuccess')), findsOneWidget);
    });
  });
}
