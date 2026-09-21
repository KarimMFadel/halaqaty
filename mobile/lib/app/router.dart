import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:halaqaty_mobile/app/chats_screen.dart';
import 'package:halaqaty_mobile/app/home_screen.dart';
import 'package:halaqaty_mobile/app/welcome_screen.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/presentation/auth_screens.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_discovery_screen.dart';
import 'package:halaqaty_mobile/features/profile/presentation/profile_screen.dart';

final _homeNavigatorKey =
    GlobalKey<NavigatorState>(debugLabel: 'homeNavigator');

/// Builds the app router from the current [AuthStatus].
///
/// The router is rebuilt when the auth status changes (status only), which
/// resets navigation on login/logout — matching the previous root watcher's
/// behavior of clearing the protected stack.
GoRouter buildHalaqatyRouter(AuthStatus status) {
  return GoRouter(
    initialLocation: '/home',
    redirect: (context, state) {
      final location = state.matchedLocation;
      switch (status) {
        case AuthStatus.unknown:
          return location == '/splash' ? null : '/splash';
        case AuthStatus.unauthenticated:
          return location == '/welcome' ||
                  location == '/login' ||
                  location == '/register'
              ? null
              : '/welcome';
        case AuthStatus.authenticated:
          return location == '/splash' ||
                  location == '/welcome' ||
                  location == '/login' ||
                  location == '/register'
              ? '/home'
              : null;
      }
    },
    routes: [
      GoRoute(
        path: '/splash',
        builder: (context, state) => const HalaqatySplash(),
      ),
      GoRoute(
        path: '/welcome',
        builder: (context, state) => const WelcomeScreen(),
      ),
      GoRoute(
        path: '/login',
        builder: (context, state) => const LoginScreen(),
      ),
      GoRoute(
        path: '/register',
        builder: (context, state) => const RegisterScreen(),
      ),
      StatefulShellRoute.indexedStack(
        builder: (context, state, navigationShell) =>
            HalaqatyShell(navigationShell: navigationShell),
        branches: [
          StatefulShellBranch(
            navigatorKey: _homeNavigatorKey,
            routes: [
              GoRoute(
                path: '/home',
                builder: (context, state) => const HomeScreen(),
              ),
            ],
          ),
          StatefulShellBranch(routes: [
            GoRoute(
              path: '/circles',
              builder: (context, state) => const CircleDiscoveryScreen(),
            ),
          ]),
          StatefulShellBranch(routes: [
            GoRoute(
              path: '/chats',
              builder: (context, state) => const ChatsScreen(),
            ),
          ]),
          StatefulShellBranch(routes: [
            GoRoute(
              path: '/profile',
              builder: (context, state) => const ProfileScreen(),
            ),
          ]),
        ],
      ),
    ],
  );
}

/// App shell with the bottom navigation bar (Home, Circles, Chats, Profile).
class HalaqatyShell extends StatelessWidget {
  const HalaqatyShell({super.key, required this.navigationShell});

  final StatefulNavigationShell navigationShell;

  @override
  Widget build(BuildContext context) {
    final isRtl = Directionality.of(context) == TextDirection.rtl;
    return Scaffold(
      body: navigationShell,
      bottomNavigationBar: NavigationBar(
        key: const Key('appNavigationBar'),
        selectedIndex: navigationShell.currentIndex,
        onDestinationSelected: (index) {
          // Home is the app's landing surface. Return to its route root
          // instead of restoring a pushed circle-detail screen.
          if (index == 0) {
            _homeNavigatorKey.currentState?.popUntil((route) => route.isFirst);
            navigationShell.goBranch(0);
            return;
          }
          navigationShell.goBranch(
            index,
            initialLocation: index == navigationShell.currentIndex,
          );
        },
        destinations: [
          NavigationDestination(
            icon: const Icon(Icons.home_outlined),
            selectedIcon: const Icon(Icons.home),
            label: isRtl ? 'الرئيسية' : 'Home',
          ),
          NavigationDestination(
            icon: const Icon(Icons.groups_outlined),
            selectedIcon: const Icon(Icons.groups),
            label: isRtl ? 'الحلقات' : 'Circles',
          ),
          NavigationDestination(
            icon: const Icon(Icons.forum_outlined),
            selectedIcon: const Icon(Icons.forum),
            label: isRtl ? 'المحادثات' : 'Chats',
          ),
          NavigationDestination(
            icon: const Icon(Icons.person_outline),
            selectedIcon: const Icon(Icons.person),
            label: isRtl ? 'حسابي' : 'Profile',
          ),
        ],
      ),
    );
  }
}

/// Branded splash shown while authentication initializes.
class HalaqatySplash extends StatelessWidget {
  const HalaqatySplash({super.key});

  @override
  Widget build(BuildContext context) {
    return const Scaffold(
      body: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            HalaqatyLogo(size: 96),
            SizedBox(height: 32),
            CircularProgressIndicator(key: Key('authInitializing')),
          ],
        ),
      ),
    );
  }
}
