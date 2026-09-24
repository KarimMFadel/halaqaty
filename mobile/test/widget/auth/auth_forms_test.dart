import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/presentation/auth_screens.dart';

// ---------------------------------------------------------------------------
// Stub StateNotifier — avoids Firebase/Dio in widget tests
// ---------------------------------------------------------------------------

/// A minimal [StateNotifier] that stands in for [AuthController] in widget
/// tests. It records calls and transitions state deterministically — no
/// network or platform-plugin calls are made.
class _StubAuthNotifier extends StateNotifier<AuthState>
    implements AuthController {
  _StubAuthNotifier({this.submitError, this.hangSubmit = false})
      : super(const AuthState(status: AuthStatus.unauthenticated));

  /// When set, register/signIn finish unauthenticated with this message.
  final String? submitError;

  /// When true, register/signIn hold `isLoading` forever (loading state).
  final bool hangSubmit;

  bool registerCalled = false;
  bool signInCalled = false;
  String? lastDisplayName;
  String? lastPreferredLanguage;
  String? lastEmail;
  String? lastPassword;

  @override
  Future<void> register({
    required String email,
    required String password,
    required String displayName,
    required String preferredLanguage,
  }) async {
    registerCalled = true;
    lastDisplayName = displayName;
    lastPreferredLanguage = preferredLanguage;
    lastEmail = email;
    lastPassword = password;
    await _applySubmit();
  }

  @override
  Future<void> signIn({
    required String email,
    required String password,
  }) async {
    signInCalled = true;
    lastEmail = email;
    lastPassword = password;
    await _applySubmit();
  }

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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

Widget _buildRegisterScreen(
  _StubAuthNotifier stub, {
  TextDirection direction = TextDirection.ltr,
}) {
  return ProviderScope(
    overrides: [
      authControllerProvider.overrideWith((_) => stub),
    ],
    child: MaterialApp(
      builder: (context, child) =>
          Directionality(textDirection: direction, child: child!),
      home: const RegisterScreen(),
    ),
  );
}

Widget _buildLoginScreen(
  _StubAuthNotifier stub, {
  TextDirection direction = TextDirection.ltr,
}) {
  return ProviderScope(
    overrides: [
      authControllerProvider.overrideWith((_) => stub),
    ],
    child: MaterialApp(
      builder: (context, child) =>
          Directionality(textDirection: direction, child: child!),
      home: const LoginScreen(),
    ),
  );
}

TextField _field(WidgetTester tester, Key key) => tester.widget<TextField>(
      find.descendant(of: find.byKey(key), matching: find.byType(TextField)),
    );

// ---------------------------------------------------------------------------
// Tests: RegisterScreen form validation
// ---------------------------------------------------------------------------

void main() {
  group('RegisterScreen — form validation', () {
    testWidgets('empty display_name shows required-field error',
        (WidgetTester tester) async {
      final stub = _StubAuthNotifier();
      await tester.pumpWidget(_buildRegisterScreen(stub));

      // Leave display-name blank; fill other fields so they do not trigger
      // their own errors and obscure the assertion.
      await tester.enterText(find.byKey(const Key('emailField')), 'a@b.com');
      await tester.enterText(
          find.byKey(const Key('passwordField')), 'password123');

      await tester.tap(find.byKey(const Key('submitButton')));
      await tester.pump();

      expect(find.text('Display name is required'), findsOneWidget);
      expect(stub.registerCalled, isFalse);
    });

    testWidgets('display_name with 1 character shows minimum-length error',
        (WidgetTester tester) async {
      final stub = _StubAuthNotifier();
      await tester.pumpWidget(_buildRegisterScreen(stub));

      await tester.enterText(find.byKey(const Key('displayNameField')), 'X');
      await tester.enterText(find.byKey(const Key('emailField')), 'a@b.com');
      await tester.enterText(
          find.byKey(const Key('passwordField')), 'password123');

      await tester.tap(find.byKey(const Key('submitButton')));
      await tester.pump();

      expect(
        find.text('Display name must be at least 2 characters'),
        findsOneWidget,
      );
      expect(stub.registerCalled, isFalse);
    });

    testWidgets(
        'valid display_name does not show any display-name validation error',
        (WidgetTester tester) async {
      final stub = _StubAuthNotifier();
      await tester.pumpWidget(_buildRegisterScreen(stub));

      await tester.enterText(
          find.byKey(const Key('displayNameField')), 'Ahmad');
      await tester.enterText(find.byKey(const Key('emailField')), 'a@b.com');
      await tester.enterText(
          find.byKey(const Key('passwordField')), 'password123');

      // Validate the form without triggering the async Firebase submit — tap
      // and pump a single frame so synchronous form-validation runs but async
      // network/platform calls have not yet executed.
      await tester.tap(find.byKey(const Key('submitButton')));
      await tester.pump();

      expect(find.text('Display name is required'), findsNothing);
      expect(
        find.text('Display name must be at least 2 characters'),
        findsNothing,
      );
    });
  });

  group('RegisterScreen — US5 hierarchy, targets, and direction', () {
    testWidgets('exposes every preserved field and the brand mark',
        (WidgetTester tester) async {
      await tester.pumpWidget(_buildRegisterScreen(_StubAuthNotifier()));

      for (final key in [
        const Key('displayNameField'),
        const Key('emailField'),
        const Key('passwordField'),
        const Key('languageDropdown'),
        const Key('submitButton'),
      ]) {
        expect(find.byKey(key), findsOneWidget);
      }
      expect(find.byType(HalaqatyLogo), findsOneWidget);
    });

    testWidgets('submit is the single dominant filled action and meets 48dp',
        (WidgetTester tester) async {
      await tester.pumpWidget(_buildRegisterScreen(_StubAuthNotifier()));

      expect(find.byType(FilledButton), findsOneWidget);
      expect(
        tester.widget(find.byKey(const Key('submitButton'))),
        isA<FilledButton>(),
      );
      expect(
        tester.getSize(find.byKey(const Key('submitButton'))).height,
        greaterThanOrEqualTo(48),
      );
    });

    testWidgets('submit is disabled with progress while registering',
        (WidgetTester tester) async {
      final stub = _StubAuthNotifier(hangSubmit: true);
      await tester.pumpWidget(_buildRegisterScreen(stub));

      await tester.enterText(
          find.byKey(const Key('displayNameField')), 'Ahmad');
      await tester.enterText(find.byKey(const Key('emailField')), 'a@b.com');
      await tester.enterText(
          find.byKey(const Key('passwordField')), 'password123');
      await tester.tap(find.byKey(const Key('submitButton')));
      await tester.pump();

      final submit = tester.widget<ButtonStyleButton>(
        find.byKey(const Key('submitButton')),
      );
      expect(submit.enabled, isFalse);
      expect(find.byType(CircularProgressIndicator), findsOneWidget);
      expect(stub.registerCalled, isTrue);
    });

    testWidgets('keyboard flow follows the visual field order',
        (WidgetTester tester) async {
      await tester.pumpWidget(_buildRegisterScreen(_StubAuthNotifier()));

      expect(
        _field(tester, const Key('displayNameField')).textInputAction,
        TextInputAction.next,
      );
      expect(
        _field(tester, const Key('emailField')).textInputAction,
        TextInputAction.next,
      );
      expect(
        _field(tester, const Key('passwordField')).textInputAction,
        TextInputAction.done,
      );
    });

    testWidgets('password visibility toggle has a localized accessible label',
        (WidgetTester tester) async {
      await tester.pumpWidget(_buildRegisterScreen(_StubAuthNotifier()));

      final toggleFinder = find.descendant(
        of: find.byKey(const Key('passwordField')),
        matching: find.byType(IconButton),
      );
      final toggle = tester.widget<IconButton>(toggleFinder);
      expect(toggle.tooltip, isNotNull);
      expect(toggle.tooltip, isNotEmpty);
      final semantics = tester.getSemantics(toggleFinder);
      expect(
          semantics.tooltip.isNotEmpty || semantics.label.isNotEmpty, isTrue);
    });

    testWidgets('submit button exposes its label to screen readers',
        (WidgetTester tester) async {
      await tester.pumpWidget(_buildRegisterScreen(_StubAuthNotifier()));

      final semantics =
          tester.getSemantics(find.byKey(const Key('submitButton')));
      expect(semantics.label, 'Register');
    });

    testWidgets(
        'auth failure shows the branded error snackbar and keeps '
        'entered input', (WidgetTester tester) async {
      final stub = _StubAuthNotifier(submitError: 'Email is already in use');
      await tester.pumpWidget(_buildRegisterScreen(stub));

      await tester.enterText(
          find.byKey(const Key('displayNameField')), 'Ahmad');
      await tester.enterText(find.byKey(const Key('emailField')), 'a@b.com');
      await tester.enterText(
          find.byKey(const Key('passwordField')), 'password123');
      await tester.tap(find.byKey(const Key('submitButton')));
      await tester.pump();

      expect(
        find.byKey(const Key('halaqatyErrorSnackBar')),
        findsOneWidget,
      );
      expect(find.text('Email is already in use'), findsOneWidget);
      // Non-secret input survives the failed attempt (spec state matrix).
      expect(
          _field(tester, const Key('emailField')).controller?.text, 'a@b.com');
      expect(_field(tester, const Key('displayNameField')).controller?.text,
          'Ahmad');
    });

    testWidgets('Arabic RTL renders localized labels and validation copy',
        (WidgetTester tester) async {
      final stub = _StubAuthNotifier();
      await tester.pumpWidget(
        _buildRegisterScreen(stub, direction: TextDirection.rtl),
      );

      expect(find.text('الاسم المعروض'), findsOneWidget);
      expect(find.text('البريد الإلكتروني'), findsOneWidget);
      expect(find.text('كلمة المرور'), findsOneWidget);
      expect(find.text('اللغة المفضلة'), findsOneWidget);
      expect(find.text('إنشاء حساب'), findsWidgets);

      await tester.tap(find.byKey(const Key('submitButton')));
      await tester.pump();

      expect(find.text('الاسم المعروض مطلوب'), findsOneWidget);
      expect(find.text('البريد مطلوب'), findsOneWidget);
      expect(find.text('كلمة المرور مطلوبة'), findsOneWidget);
      expect(stub.registerCalled, isFalse);
    });

    testWidgets('registers with the selected preferred language',
        (WidgetTester tester) async {
      final stub = _StubAuthNotifier();
      await tester.pumpWidget(
        _buildRegisterScreen(stub, direction: TextDirection.rtl),
      );

      await tester.tap(find.byKey(const Key('languageDropdown')));
      await tester.pumpAndSettle();
      await tester.tap(find.text('الإنجليزية').last);
      await tester.pumpAndSettle();

      await tester.enterText(
          find.byKey(const Key('displayNameField')), 'Ahmad');
      await tester.enterText(find.byKey(const Key('emailField')), 'a@b.com');
      await tester.enterText(
          find.byKey(const Key('passwordField')), 'password123');
      await tester.tap(find.byKey(const Key('submitButton')));
      await tester.pump();

      expect(stub.registerCalled, isTrue);
      expect(stub.lastPreferredLanguage, 'en');
    });
  });

  group('LoginScreen — US5 hierarchy, targets, and direction', () {
    testWidgets('exposes every preserved field and the brand mark',
        (WidgetTester tester) async {
      await tester.pumpWidget(_buildLoginScreen(_StubAuthNotifier()));

      expect(find.byKey(const Key('emailField')), findsOneWidget);
      expect(find.byKey(const Key('passwordField')), findsOneWidget);
      expect(find.byKey(const Key('submitButton')), findsOneWidget);
      expect(find.byType(HalaqatyLogo), findsOneWidget);
    });

    testWidgets('submit is the single dominant filled action and meets 48dp',
        (WidgetTester tester) async {
      await tester.pumpWidget(_buildLoginScreen(_StubAuthNotifier()));

      expect(find.byType(FilledButton), findsOneWidget);
      expect(
        tester.widget(find.byKey(const Key('submitButton'))),
        isA<FilledButton>(),
      );
      expect(
        tester.getSize(find.byKey(const Key('submitButton'))).height,
        greaterThanOrEqualTo(48),
      );
    });

    testWidgets('empty fields show inline validation and block sign-in',
        (WidgetTester tester) async {
      final stub = _StubAuthNotifier();
      await tester.pumpWidget(_buildLoginScreen(stub));

      await tester.tap(find.byKey(const Key('submitButton')));
      await tester.pump();

      expect(find.text('Email is required'), findsOneWidget);
      expect(find.text('Password is required'), findsOneWidget);
      expect(stub.signInCalled, isFalse);
    });

    testWidgets('valid submit signs in once with a trimmed email',
        (WidgetTester tester) async {
      final stub = _StubAuthNotifier();
      await tester.pumpWidget(_buildLoginScreen(stub));

      await tester.enterText(
          find.byKey(const Key('emailField')), '  a@b.com  ');
      await tester.enterText(find.byKey(const Key('passwordField')), 'secret1');
      await tester.tap(find.byKey(const Key('submitButton')));
      await tester.pump();

      expect(stub.signInCalled, isTrue);
      expect(stub.lastEmail, 'a@b.com');
      expect(stub.lastPassword, 'secret1');
    });

    testWidgets('submit is disabled with progress while signing in',
        (WidgetTester tester) async {
      final stub = _StubAuthNotifier(hangSubmit: true);
      await tester.pumpWidget(_buildLoginScreen(stub));

      await tester.enterText(find.byKey(const Key('emailField')), 'a@b.com');
      await tester.enterText(find.byKey(const Key('passwordField')), 'secret1');
      await tester.tap(find.byKey(const Key('submitButton')));
      await tester.pump();

      final submit = tester.widget<ButtonStyleButton>(
        find.byKey(const Key('submitButton')),
      );
      expect(submit.enabled, isFalse);
      expect(find.byType(CircularProgressIndicator), findsOneWidget);
      expect(stub.signInCalled, isTrue);
    });

    testWidgets('keyboard flow follows the visual field order',
        (WidgetTester tester) async {
      await tester.pumpWidget(_buildLoginScreen(_StubAuthNotifier()));

      expect(
        _field(tester, const Key('emailField')).textInputAction,
        TextInputAction.next,
      );
      expect(
        _field(tester, const Key('passwordField')).textInputAction,
        TextInputAction.done,
      );
    });

    testWidgets('password visibility toggle has a localized accessible label',
        (WidgetTester tester) async {
      await tester.pumpWidget(_buildLoginScreen(_StubAuthNotifier()));

      final toggleFinder = find.descendant(
        of: find.byKey(const Key('passwordField')),
        matching: find.byType(IconButton),
      );
      final toggle = tester.widget<IconButton>(toggleFinder);
      expect(toggle.tooltip, isNotNull);
      expect(toggle.tooltip, isNotEmpty);
      final semantics = tester.getSemantics(toggleFinder);
      expect(
          semantics.tooltip.isNotEmpty || semantics.label.isNotEmpty, isTrue);
    });

    testWidgets('submit button exposes its label to screen readers',
        (WidgetTester tester) async {
      await tester.pumpWidget(_buildLoginScreen(_StubAuthNotifier()));

      final semantics =
          tester.getSemantics(find.byKey(const Key('submitButton')));
      expect(semantics.label, 'Sign In');
    });

    testWidgets(
        'auth failure shows the branded error snackbar and keeps '
        'the entered email', (WidgetTester tester) async {
      final stub = _StubAuthNotifier(submitError: 'Invalid email or password');
      await tester.pumpWidget(_buildLoginScreen(stub));

      await tester.enterText(find.byKey(const Key('emailField')), 'a@b.com');
      await tester.enterText(find.byKey(const Key('passwordField')), 'secret1');
      await tester.tap(find.byKey(const Key('submitButton')));
      await tester.pump();

      expect(
        find.byKey(const Key('halaqatyErrorSnackBar')),
        findsOneWidget,
      );
      expect(find.text('Invalid email or password'), findsOneWidget);
      expect(
          _field(tester, const Key('emailField')).controller?.text, 'a@b.com');
    });

    testWidgets('Arabic RTL renders localized labels and validation copy',
        (WidgetTester tester) async {
      final stub = _StubAuthNotifier();
      await tester.pumpWidget(
        _buildLoginScreen(stub, direction: TextDirection.rtl),
      );

      expect(find.text('البريد الإلكتروني'), findsOneWidget);
      expect(find.text('كلمة المرور'), findsOneWidget);
      expect(find.text('دخول'), findsOneWidget);

      await tester.tap(find.byKey(const Key('submitButton')));
      await tester.pump();

      expect(find.text('البريد مطلوب'), findsOneWidget);
      expect(find.text('كلمة المرور مطلوبة'), findsOneWidget);
      expect(stub.signInCalled, isFalse);
    });
  });
}
