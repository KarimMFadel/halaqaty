import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/presentation/auth_screens.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_discovery_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/create_circle_screen.dart';
import 'package:halaqaty_mobile/features/profile/presentation/profile_screen.dart';

class ImplementedFeaturesRoot extends ConsumerWidget {
  const ImplementedFeaturesRoot({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return switch (ref.watch(authControllerProvider).status) {
      AuthStatus.unknown => const Center(
          child: CircularProgressIndicator(key: Key('authInitializing')),
        ),
      AuthStatus.unauthenticated => const AuthWelcomeScreen(),
      AuthStatus.authenticated => const _AuthenticatedFeaturesNavigator(),
    };
  }
}

class _AuthenticatedFeaturesNavigator extends StatelessWidget {
  const _AuthenticatedFeaturesNavigator();

  @override
  Widget build(BuildContext context) {
    return Navigator(
      onGenerateRoute: (_) => MaterialPageRoute<void>(
        builder: (_) => const ImplementedFeaturesScreen(),
      ),
    );
  }
}

class AuthWelcomeScreen extends StatelessWidget {
  const AuthWelcomeScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final isRtl = Directionality.of(context) == TextDirection.rtl;

    return Scaffold(
      appBar: AppBar(title: const Text('Halaqaty')),
      body: SafeArea(
        child: Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(
                  'Halaqaty',
                  textAlign: TextAlign.center,
                  style: Theme.of(context).textTheme.headlineMedium,
                ),
                const SizedBox(height: 24),
                ElevatedButton(
                  key: const Key('openLogin'),
                  onPressed: () {
                    Navigator.of(context).push(
                      MaterialPageRoute<void>(
                        builder: (_) => LoginScreen(
                          onSuccess: () => Navigator.of(context).pop(),
                        ),
                      ),
                    );
                  },
                  child: Text(isRtl ? 'تسجيل الدخول' : 'Sign in'),
                ),
                const SizedBox(height: 12),
                OutlinedButton(
                  key: const Key('openRegister'),
                  onPressed: () {
                    Navigator.of(context).push(
                      MaterialPageRoute<void>(
                        builder: (_) => RegisterScreen(
                          onSuccess: () => Navigator.of(context).pop(),
                        ),
                      ),
                    );
                  },
                  child: Text(isRtl ? 'إنشاء حساب' : 'Register'),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class ImplementedFeaturesScreen extends StatelessWidget {
  const ImplementedFeaturesScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final isRtl = Directionality.of(context) == TextDirection.rtl;
    final chevron = isRtl ? Icons.chevron_left : Icons.chevron_right;

    return Scaffold(
      appBar: AppBar(
        title: Text(isRtl ? 'الميزات المتاحة' : 'Implemented features'),
      ),
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              ListTile(
                key: const Key('openProfile'),
                title: Text(isRtl ? 'الملف الشخصي' : 'Profile'),
                trailing: Icon(chevron),
                onTap: () {
                  Navigator.of(context).push(
                    MaterialPageRoute<void>(
                      builder: (_) => const ProfileScreen(),
                    ),
                  );
                },
              ),
              ListTile(
                key: const Key('openCircleDiscovery'),
                title: Text(isRtl ? 'اكتشاف الحلقات' : 'Discover circles'),
                trailing: Icon(chevron),
                onTap: () {
                  Navigator.of(context).push(
                    MaterialPageRoute<void>(
                      builder: (_) => const CircleDiscoveryScreen(),
                    ),
                  );
                },
              ),
              ListTile(
                key: const Key('openCreateCircle'),
                title: Text(isRtl ? 'إنشاء حلقة' : 'Create circle'),
                trailing: Icon(chevron),
                onTap: () {
                  Navigator.of(context).push(
                    MaterialPageRoute<void>(
                      builder: (_) => const CreateCircleScreen(),
                    ),
                  );
                },
              ),
              const Spacer(),
              const LogoutButton(),
            ],
          ),
        ),
      ),
    );
  }
}
