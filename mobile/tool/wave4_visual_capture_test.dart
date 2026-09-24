import 'dart:async';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/app/app_locale_controller.dart';
import 'package:halaqaty_mobile/app/welcome_screen.dart';
import 'package:halaqaty_mobile/core/theme/halaqaty_theme.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/presentation/auth_screens.dart';
import 'package:halaqaty_mobile/features/profile/application/profile_controller.dart';
import 'package:halaqaty_mobile/features/profile/data/profile_api_client.dart';
import 'package:halaqaty_mobile/features/profile/presentation/profile_screen.dart';

/// Wave 4 (US5) visual evidence harness.
///
/// The emulator is shared with another agent in this wave, so instead of the
/// `flutter drive` recipe used for waves 0–2 this harness runs as a plain
/// widget test and rasterizes each state off a [RepaintBoundary], with the
/// bundled Poppins/Cairo fonts loaded so Arabic renders truthfully.
///
/// Run from `mobile/`:
///   flutter test tool/wave4_visual_capture_test.dart
///
/// PNGs land in
/// `../specs/019-mobile-app-shell-brand/evidence/screenshots/wave4/`
/// at 390dp logical width (780px at pixelRatio 2).
const _outDir =
    '../specs/019-mobile-app-shell-brand/evidence/screenshots/wave4';

void main() {
  setUpAll(() async {
    // Load the bundled fonts so captures render real Poppins/Cairo glyphs
    // instead of the Ahem test font.
    Future<void> load(String family, String path) async {
      final bytes = await File(path).readAsBytes();
      final loader = FontLoader(family)
        ..addFont(Future.value(ByteData.sublistView(bytes)));
      await loader.load();
    }

    await load('Poppins', 'assets/fonts/Poppins-Regular.ttf');
    await load('Poppins', 'assets/fonts/Poppins-SemiBold.ttf');
    await load('Poppins', 'assets/fonts/Poppins-Bold.ttf');
    await load('Cairo', 'assets/fonts/Cairo-Regular.ttf');
    await load('Cairo', 'assets/fonts/Cairo-Bold.ttf');
    // Material icons ship with the framework, not the test environment.
    final flutterRoot = Platform.environment['FLUTTER_ROOT'];
    if (flutterRoot != null) {
      await load('MaterialIcons',
          '$flutterRoot/bin/cache/artifacts/material_fonts/MaterialIcons-Regular.otf');
    }
  });

  for (final locale in const [Locale('ar'), Locale('en')]) {
    testWidgets('wave4_auth_profile_matrix_${locale.languageCode}',
        (tester) async {
      tester.view.physicalSize = const Size(390, 844);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      final boundaryKey = GlobalKey();
      final rtl = locale.languageCode == 'ar';

      Future<void> pumpScreen(
        Widget screen,
        Brightness brightness,
        List<Override> overrides,
      ) async {
        await tester.pumpWidget(const SizedBox.shrink());
        await tester.pump();
        await tester.pumpWidget(
          ProviderScope(
            overrides: [
              platformLocaleProvider.overrideWithValue(locale),
              ...overrides,
            ],
            child: RepaintBoundary(
              key: boundaryKey,
              child: MaterialApp(
                debugShowCheckedModeBanner: false,
                theme: halaqatyLightTheme(),
                darkTheme: halaqatyDarkTheme(),
                themeMode: brightness == Brightness.dark
                    ? ThemeMode.dark
                    : ThemeMode.light,
                builder: (context, child) => Directionality(
                  textDirection: rtl ? TextDirection.rtl : TextDirection.ltr,
                  child: child!,
                ),
                home: screen,
              ),
            ),
          ),
        );
      }

      /// Settles non-loading states, verifies [sentinel], writes the PNG.
      Future<void> capture(
        String state,
        Widget screen,
        Brightness brightness,
        List<Override> overrides,
        Finder sentinel, {
        Future<void> Function()? prepare,
        bool settle = true,
      }) async {
        await pumpScreen(screen, brightness, overrides);
        if (prepare != null) await prepare();
        if (settle) {
          await tester.pumpAndSettle();
        } else {
          for (var i = 0; i < 3; i++) {
            await tester.pump(const Duration(milliseconds: 200));
          }
        }
        if (sentinel.evaluate().isEmpty) {
          throw StateError('Sentinel missing for $state: $sentinel');
        }
        // Rasterization is real engine/async work — it must run outside the
        // fake test zone.
        await tester.runAsync(() async {
          final boundary = boundaryKey.currentContext!.findRenderObject()!
              as RenderRepaintBoundary;
          final image = await boundary.toImage(pixelRatio: 2.0);
          final byteData =
              await image.toByteData(format: ui.ImageByteFormat.png);
          final name = 'wave4_${state}_${locale.languageCode}_'
              '${brightness == Brightness.dark ? 'dark' : 'light'}.png';
          final file = await File('$_outDir/$name').create(recursive: true);
          await file.writeAsBytes(byteData!.buffer.asUint8List());
        });
      }

      _StubAuth authStub({String? submitError, bool hangSubmit = false}) =>
          _StubAuth(submitError: submitError, hangSubmit: hangSubmit);

      List<Override> authOverrides(_StubAuth auth) =>
          [authControllerProvider.overrideWith((_) => auth)];

      List<Override> profileOverrides(_StubProfile profile,
              {_StubAuth? auth}) =>
          [
            profileControllerProvider.overrideWith((_) => profile),
            authControllerProvider
                .overrideWith((_) => auth ?? _StubAuth.authenticated()),
          ];

      for (final brightness in Brightness.values) {
        // --- Welcome -------------------------------------------------------
        await capture(
          'welcome',
          const WelcomeScreen(),
          brightness,
          const [],
          find.byKey(const Key('openLogin')),
        );

        // --- Login ---------------------------------------------------------
        await capture(
          'login_ready',
          const LoginScreen(),
          brightness,
          authOverrides(authStub()),
          find.byKey(const Key('submitButton')),
        );

        await capture(
          'login_validation',
          const LoginScreen(),
          brightness,
          authOverrides(authStub()),
          find.text(rtl ? 'البريد مطلوب' : 'Email is required'),
          prepare: () async {
            await tester.tap(find.byKey(const Key('submitButton')));
            await tester.pump();
          },
        );

        await capture(
          'login_loading',
          const LoginScreen(),
          brightness,
          authOverrides(authStub(hangSubmit: true)),
          find.byType(CircularProgressIndicator),
          settle: false,
          prepare: () async {
            await tester.enterText(
                find.byKey(const Key('emailField')), 'a@b.com');
            await tester.enterText(
                find.byKey(const Key('passwordField')), 'secret1');
            await tester.tap(find.byKey(const Key('submitButton')));
            await tester.pump();
          },
        );

        // --- Register ------------------------------------------------------
        await capture(
          'register_ready',
          const RegisterScreen(),
          brightness,
          authOverrides(authStub()),
          find.byKey(const Key('languageDropdown')),
        );

        await capture(
          'register_validation',
          const RegisterScreen(),
          brightness,
          authOverrides(authStub()),
          find.text(rtl ? 'الاسم المعروض مطلوب' : 'Display name is required'),
          prepare: () async {
            await tester.tap(find.byKey(const Key('submitButton')));
            await tester.pump();
          },
        );

        // --- Profile -------------------------------------------------------
        await capture(
          'profile_loading',
          const ProfileScreen(),
          brightness,
          profileOverrides(_StubProfile(hangLoad: true, locale: locale)),
          find.byType(CircularProgressIndicator),
          settle: false,
        );

        await capture(
          'profile_ready',
          const ProfileScreen(),
          brightness,
          profileOverrides(_StubProfile(locale: locale)),
          find.byKey(const Key('profileSaveButton')),
        );

        await capture(
          'profile_saved',
          const ProfileScreen(),
          brightness,
          profileOverrides(_StubProfile(locale: locale)),
          find.byKey(const Key('profileSaveSuccess')),
          prepare: () async {
            await tester
                .ensureVisible(find.byKey(const Key('profileSaveButton')));
            await tester.pump();
            await tester.tap(find.byKey(const Key('profileSaveButton')));
            await tester.pump();
          },
        );

        await capture(
          'profile_save_error',
          const ProfileScreen(),
          brightness,
          profileOverrides(_StubProfile(
            locale: locale,
            saveErrorMessage: rtl
                ? 'تعذر الحفظ. تحقق من اتصالك وحاول مجدداً.'
                : 'Could not save. Check your connection and retry.',
          )),
          find.text(rtl
              ? 'تعذر الحفظ. تحقق من اتصالك وحاول مجدداً.'
              : 'Could not save. Check your connection and retry.'),
          prepare: () async {
            await tester
                .ensureVisible(find.byKey(const Key('profileSaveButton')));
            await tester.pump();
            await tester.tap(find.byKey(const Key('profileSaveButton')));
            await tester.pump();
          },
        );

        await capture(
          'profile_load_error',
          const ProfileScreen(),
          brightness,
          profileOverrides(_StubProfile(
            locale: locale,
            loadErrorMessage: rtl
                ? 'لا يمكن الوصول إلى الخادم حالياً'
                : 'We cannot reach the server right now',
          )),
          find.text(rtl ? 'إعادة المحاولة' : 'Retry'),
        );

        await capture(
          'profile_notice',
          const ProfileScreen(),
          brightness,
          profileOverrides(_StubProfile(locale: locale)),
          find.byKey(const Key('halaqatyUnderImplementationSnackBar')),
          prepare: () async {
            await tester
                .ensureVisible(find.text(rtl ? 'المظهر' : 'Appearance'));
            await tester.pump();
            await tester.tap(find.text(rtl ? 'المظهر' : 'Appearance'));
            await tester.pump(const Duration(milliseconds: 400));
          },
        );
      }
    });
  }
}

// ---------------------------------------------------------------------------
// Stubs (no Firebase, Dio, or platform plugins)
// ---------------------------------------------------------------------------

class _StubAuth extends StateNotifier<AuthState> implements AuthController {
  _StubAuth({this.submitError, this.hangSubmit = false})
      : super(const AuthState(status: AuthStatus.unauthenticated));

  _StubAuth.authenticated()
      : submitError = null,
        hangSubmit = false,
        super(const AuthState(
          status: AuthStatus.authenticated,
          sessionId: 'session-1',
        ));

  final String? submitError;
  final bool hangSubmit;

  @override
  Future<void> register({
    required String email,
    required String password,
    required String displayName,
    required String preferredLanguage,
  }) async =>
      _applySubmit();

  @override
  Future<void> signIn({
    required String email,
    required String password,
  }) async =>
      _applySubmit();

  Future<void> _applySubmit() async {
    if (hangSubmit) {
      state = state.copyWith(isLoading: true);
      return Completer<void>().future;
    }
    final error = submitError;
    if (error != null) {
      state = AuthState(
        status: AuthStatus.unauthenticated,
        errorMessage: error,
      );
      return;
    }
    state = const AuthState(
      status: AuthStatus.authenticated,
      sessionId: 'stub-session',
    );
  }

  @override
  Future<void> logout() async {
    state = const AuthState(status: AuthStatus.unauthenticated);
  }
}

class _StubProfile extends StateNotifier<ProfileState>
    implements ProfileController {
  _StubProfile({
    this.loadErrorMessage,
    this.hangLoad = false,
    this.saveErrorMessage,
    required Locale locale,
  })  : _language = locale.languageCode,
        super(const ProfileState());

  final String? loadErrorMessage;
  final bool hangLoad;
  final String? saveErrorMessage;
  final String _language;

  @override
  Future<void> loadProfile() async {
    if (hangLoad) {
      state = state.copyWith(isLoading: true);
      return Completer<void>().future;
    }
    final loadError = loadErrorMessage;
    if (loadError != null) {
      state = state.copyWith(isLoading: false, errorMessage: loadError);
      return;
    }
    final ar = _language == 'ar';
    state = ProfileState(
      profile: ProfileUser(
        id: 'user-1',
        firebaseUid: 'firebase-1',
        fullName: ar ? 'علي محمود' : 'Ali Mahmoud',
        displayName: ar ? 'علي' : 'Ali',
        bio: ar ? 'طالب علم' : 'Seeker of knowledge',
        country: 'EG',
        preferredLanguage: _language,
        avatarUrl: null,
        phone: null,
        createdAt: DateTime.parse('2026-01-01T00:00:00Z'),
      ),
    );
  }

  @override
  Future<bool> updateProfile({required UpdateProfileRequest request}) async {
    final saveError = saveErrorMessage;
    if (saveError != null) {
      state = state.copyWith(isSaving: false, errorMessage: saveError);
      return false;
    }
    return true;
  }
}
