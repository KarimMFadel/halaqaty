import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:halaqaty_mobile/app/app_locale_controller.dart';
import 'package:halaqaty_mobile/app/router.dart';
import 'package:halaqaty_mobile/core/theme/halaqaty_theme.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/notifications/application/notification_fcm_controller.dart';
import 'package:halaqaty_mobile/features/notifications/application/notification_permission_controller.dart';

import 'firebase_options.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await Firebase.initializeApp(
    options: DefaultFirebaseOptions.currentPlatform,
  );
  // F-008 T026: top-level background handler. The server persists history
  // and the authoritative unread count and the payload is positional, so
  // the handler performs no work (see
  // features/notifications/application/notification_fcm_controller.dart).
  FirebaseMessaging.onBackgroundMessage(firebaseMessagingBackgroundHandler);
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
  void initState() {
    super.initState();
    // Subscribes foreground messages and open taps once for the app
    // lifetime; F-008 T026 routing + presentation consume its state below.
    ref.read(notificationFcmControllerProvider.notifier);
  }

  @override
  Widget build(BuildContext context) {
    final authStatus =
        ref.watch(authControllerProvider.select((s) => s.status));
    final locale = ref.watch(appLocaleControllerProvider);
    final isArabic = locale.languageCode == 'ar';
    // Logout/session loss drops the locale back to the platform fallback.
    ref.listen(authControllerProvider.select((s) => s.status), (_, next) {
      if (next == AuthStatus.unauthenticated) {
        ref
            .read(appLocaleControllerProvider.notifier)
            .restorePlatformFallback();
      }
      if (next == AuthStatus.authenticated) {
        // F-008 T026: the authenticated session is the first relevant push
        // moment — evaluate/request permission before token registration
        // matters (login sign-in and restored sessions both fire this).
        ref
            .read(
              notificationPermissionControllerProvider.notifier,
            )
            .ensurePermissionRequested();
      }
    });
    // Rebuild the router only on auth-status change (login/logout clears the
    // protected stack); locale changes must not reset navigation.
    if (_router == null || _routerStatus != authStatus) {
      _routerStatus = authStatus;
      _router = buildHalaqatyRouter(authStatus);
    }
    // F-008 T027 notification-center routing: cold starts and in-app taps
    // land on the notification center (the dedicated surface for
    // notification history/read state); no payload-field routing.
    ref.listen<NotificationOpenIntent?>(
      notificationFcmControllerProvider.select((state) => state.pendingOpen),
      (previous, open) {
        if (open == null || _router == null) return;
        _router!.go('/notifications');
      },
    );
    // F-008 T026: single gentle denied-permission hint (FR-017). The
    // controller gates once-per-install timing and the retry action; this
    // is only the non-blocking display surface (Arabic-first copy).
    ref.listen<bool>(
      notificationPermissionControllerProvider
          .select((state) => state.gentlePromptVisible),
      (_, visible) {
        if (!visible) return;
        final messenger = ScaffoldMessenger.of(context);
        final handle = messenger.showSnackBar(SnackBar(
          behavior: SnackBarBehavior.floating,
          content: Text(isArabic
              ? 'فعّل الإشعارات لتعرف متى تبدأ الجلسات.'
              : 'Enable notifications to know when sessions start.'),
          action: SnackBarAction(
            label: isArabic ? 'تفعيل' : 'Enable',
            onPressed: () => ref
                .read(notificationPermissionControllerProvider.notifier)
                .acknowledgePrompt(retry: true),
          ),
        ));
        handle.closed.then((_) {
          if (ref
              .read(notificationPermissionControllerProvider)
              .gentlePromptVisible) {
            ref
                .read(notificationPermissionControllerProvider.notifier)
                .acknowledgePrompt(retry: false);
          }
        });
      },
    );
    return MaterialApp.router(
      title: 'Halaqaty',
      debugShowCheckedModeBanner: false,
      theme: halaqatyLightTheme(),
      darkTheme: halaqatyDarkTheme(),
      // Explicit root Directionality: the default delegates support English
      // only, and F-019 adds no localization framework.
      builder: (context, child) => Directionality(
        textDirection: isArabic ? TextDirection.rtl : TextDirection.ltr,
        child: child!,
      ),
      routerConfig: _router!,
    );
  }
}
