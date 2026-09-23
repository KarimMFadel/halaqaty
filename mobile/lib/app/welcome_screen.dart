import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';

/// Branded welcome screen shown while unauthenticated.
///
/// Replaces the former `AuthWelcomeScreen` dev bridge; keeps the
/// `openLogin`/`openRegister` keys used by widget tests.
class WelcomeScreen extends StatelessWidget {
  const WelcomeScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final isRtl = Directionality.of(context) == TextDirection.rtl;
    final scheme = Theme.of(context).colorScheme;

    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                const SizedBox(height: 48),
                const HalaqatyLogo(size: 112),
                const SizedBox(height: 24),
                Text(
                  isRtl ? 'حلقاتي' : 'Halaqaty',
                  style: Theme.of(context).textTheme.displaySmall?.copyWith(
                        fontWeight: FontWeight.w700,
                        color: scheme.primary,
                      ),
                ),
                const SizedBox(height: 8),
                Text(
                  isRtl
                      ? 'منصة حلقات تحفيظ القرآن'
                      : 'Quran memorization circles platform',
                  textAlign: TextAlign.center,
                  style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                        color: scheme.onSurfaceVariant,
                      ),
                ),
                const SizedBox(height: 48),
                SizedBox(
                  width: double.infinity,
                  child: FilledButton(
                    key: const Key('openLogin'),
                    onPressed: () => context.push('/login'),
                    child: Padding(
                      padding: const EdgeInsets.symmetric(vertical: 12),
                      child: Text(isRtl ? 'تسجيل الدخول' : 'Sign in'),
                    ),
                  ),
                ),
                const SizedBox(height: 12),
                SizedBox(
                  width: double.infinity,
                  child: OutlinedButton(
                    key: const Key('openRegister'),
                    onPressed: () => context.push('/register'),
                    child: Padding(
                      padding: const EdgeInsets.symmetric(vertical: 12),
                      child: Text(isRtl ? 'إنشاء حساب' : 'Register'),
                    ),
                  ),
                ),
                const SizedBox(height: 48),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
