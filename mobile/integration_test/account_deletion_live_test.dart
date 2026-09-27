import 'package:firebase_auth/firebase_auth.dart';
import 'package:firebase_core/firebase_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/app/app_locale_controller.dart';
import 'package:halaqaty_mobile/core/theme/halaqaty_theme.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/profile/application/profile_controller.dart';
import 'package:halaqaty_mobile/features/profile/presentation/profile_screen.dart';
import 'package:halaqaty_mobile/firebase_options.dart';
import 'package:integration_test/integration_test.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('F-001 deletes a disposable Firebase student', (tester) async {
    const email = String.fromEnvironment('F001_DELETE_EMAIL');
    const password = String.fromEnvironment('F001_DELETE_PASSWORD');
    const register = bool.fromEnvironment('F001_DELETE_REGISTER');
    if (email.isEmpty || password.isEmpty) {
      markTestSkipped('Disposable F001 Firebase test credentials are missing.');
      return;
    }

    await Firebase.initializeApp(
      options: DefaultFirebaseOptions.currentPlatform,
    );
    final container = ProviderContainer(
      overrides: [
        platformLocaleProvider.overrideWithValue(const Locale('en')),
      ],
    );
    addTearDown(container.dispose);

    final auth = container.read(authControllerProvider.notifier);
    if (register) {
      await auth.register(
        email: email,
        password: password,
        displayName: 'Disposable deletion test',
        preferredLanguage: 'en',
      );
    } else {
      await auth.signIn(email: email, password: password);
    }
    expect(
      container.read(authControllerProvider).isAuthenticated,
      isTrue,
      reason:
          'Sign-in failed: ${container.read(authControllerProvider).errorMessage}',
    );

    await tester.pumpWidget(UncontrolledProviderScope(
      container: container,
      child: MaterialApp(
        theme: halaqatyLightTheme(),
        home: const ProfileScreen(),
      ),
    ));
    await tester.pumpAndSettle();

    final deleteAction = find.byKey(const Key('deleteAccountTile'));
    await tester.ensureVisible(deleteAction);
    await tester.pumpAndSettle();
    expect(deleteAction, findsOneWidget);
    await tester.tap(deleteAction);
    await tester.pumpAndSettle();
    expect(find.text('Permanently delete account?'), findsOneWidget);

    await tester.enterText(find.byType(TextField).last, password);
    await tester.tap(find.descendant(
      of: find.byType(AlertDialog),
      matching: find.text('Delete account'),
    ));
    await tester.runAsync(() async {
      final deadline = DateTime.now().add(const Duration(seconds: 30));
      while (container.read(authControllerProvider).status !=
              AuthStatus.unauthenticated &&
          container.read(profileControllerProvider).errorMessage == null &&
          DateTime.now().isBefore(deadline)) {
        await Future<void>.delayed(const Duration(milliseconds: 100));
      }
    });
    await tester.pumpAndSettle();

    expect(container.read(authControllerProvider).status,
        AuthStatus.unauthenticated,
        reason: 'Deletion error: '
            '${container.read(profileControllerProvider).errorMessage}');
    expect(FirebaseAuth.instance.currentUser, isNull);
    expect(
      find.text('Account deleted.'),
      findsOneWidget,
      reason: 'The live Admin credential should complete Firebase cleanup.',
    );
  });
}
