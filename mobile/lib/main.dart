import 'package:firebase_core/firebase_core.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:halaqaty_mobile/app/app_locale_controller.dart';
import 'package:halaqaty_mobile/app/router.dart';
import 'package:halaqaty_mobile/core/theme/halaqaty_theme.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';

import 'firebase_options.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await Firebase.initializeApp(
    options: DefaultFirebaseOptions.currentPlatform,
  );
  runApp(const ProviderScope(child: MyApp()));
}

class MyApp extends ConsumerStatefulWidget {
  const MyApp({super.key});

  @override
  ConsumerState<MyApp> createState() => _MyAppState();
}

class _MyAppState extends ConsumerState<MyApp> {
  GoRouter? _router;
  AuthStatus? _routerStatus;

  @override
  Widget build(BuildContext context) {
    final authStatus =
        ref.watch(authControllerProvider.select((s) => s.status));
    final locale = ref.watch(appLocaleControllerProvider);
    // Logout/session loss drops the locale back to the platform fallback.
    ref.listen(authControllerProvider.select((s) => s.status), (_, next) {
      if (next == AuthStatus.unauthenticated) {
        ref
            .read(appLocaleControllerProvider.notifier)
            .restorePlatformFallback();
      }
    });
    // Rebuild the router only on auth-status change (login/logout clears the
    // protected stack); locale changes must not reset navigation.
    if (_router == null || _routerStatus != authStatus) {
      _routerStatus = authStatus;
      _router = buildHalaqatyRouter(authStatus);
    }
    return MaterialApp.router(
      title: 'Halaqaty',
      theme: halaqatyLightTheme(),
      darkTheme: halaqatyDarkTheme(),
      // Explicit root Directionality: the default delegates support English
      // only, and F-019 adds no localization framework.
      builder: (context, child) => Directionality(
        textDirection:
            locale.languageCode == 'ar' ? TextDirection.rtl : TextDirection.ltr,
        child: child!,
      ),
      routerConfig: _router!,
    );
  }
}
