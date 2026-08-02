import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../application/auth_controller.dart';

// ---------------------------------------------------------------------------
// Supported languages
// ---------------------------------------------------------------------------

const _supportedLanguages = [
  _LanguageOption(code: 'ar', labelEn: 'Arabic', labelAr: 'العربية'),
  _LanguageOption(code: 'en', labelEn: 'English', labelAr: 'الإنجليزية'),
];

class _LanguageOption {
  const _LanguageOption({
    required this.code,
    required this.labelEn,
    required this.labelAr,
  });
  final String code;
  final String labelEn;
  final String labelAr;
}

// ---------------------------------------------------------------------------
// RegisterScreen
// ---------------------------------------------------------------------------

/// Registration screen.
///
/// Flow:
/// 1. Collects email, password, display_name, preferred_language.
/// 2. Creates a Firebase account via [createUserWithEmailAndPassword].
/// 3. Delegates to [AuthController.register] to provision the backend user
///    and persist the opaque backend session ID.
class RegisterScreen extends ConsumerStatefulWidget {
  const RegisterScreen({super.key, this.onSuccess});

  /// Called after successful registration and backend session creation.
  final VoidCallback? onSuccess;

  @override
  ConsumerState<RegisterScreen> createState() => _RegisterScreenState();
}

class _RegisterScreenState extends ConsumerState<RegisterScreen> {
  final _formKey = GlobalKey<FormState>();
  final _emailController = TextEditingController();
  final _passwordController = TextEditingController();
  final _displayNameController = TextEditingController();

  String _selectedLanguage = 'ar';
  bool _obscurePassword = true;

  @override
  void dispose() {
    _emailController.dispose();
    _passwordController.dispose();
    _displayNameController.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (!_formKey.currentState!.validate()) return;

    await ref.read(authControllerProvider.notifier).register(
          email: _emailController.text.trim(),
          password: _passwordController.text,
          displayName: _displayNameController.text.trim(),
          preferredLanguage: _selectedLanguage,
        );

    if (!mounted) return;
    final authState = ref.read(authControllerProvider);
    if (authState.isAuthenticated) {
      widget.onSuccess?.call();
    } else if (authState.errorMessage != null) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(authState.errorMessage!)),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final isLoading = ref.watch(
      authControllerProvider.select((s) => s.isLoading),
    );
    final isRtl = Directionality.of(context) == TextDirection.rtl;

    return Scaffold(
      appBar: AppBar(
        title: Text(isRtl ? 'إنشاء حساب' : 'Create Account'),
      ),
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: Form(
            key: _formKey,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                // Display name
                TextFormField(
                  key: const Key('displayNameField'),
                  controller: _displayNameController,
                  textDirection: isRtl ? TextDirection.rtl : TextDirection.ltr,
                  decoration: InputDecoration(
                    labelText: isRtl ? 'الاسم المعروض' : 'Display Name',
                  ),
                  validator: (value) {
                    final v = value?.trim() ?? '';
                    if (v.isEmpty) {
                      return isRtl
                          ? 'الاسم المعروض مطلوب'
                          : 'Display name is required';
                    }
                    if (v.length < 2) {
                      return isRtl
                          ? 'الاسم يجب أن يكون حرفين على الأقل'
                          : 'Display name must be at least 2 characters';
                    }
                    if (v.length > 100) {
                      return isRtl
                          ? 'الاسم يجب أن لا يتجاوز 100 حرف'
                          : 'Display name must be at most 100 characters';
                    }
                    return null;
                  },
                ),
                const SizedBox(height: 16),

                // Email
                TextFormField(
                  key: const Key('emailField'),
                  controller: _emailController,
                  keyboardType: TextInputType.emailAddress,
                  decoration: InputDecoration(
                    labelText: isRtl ? 'البريد الإلكتروني' : 'Email',
                  ),
                  validator: (value) {
                    if (value == null || value.trim().isEmpty) {
                      return isRtl ? 'البريد مطلوب' : 'Email is required';
                    }
                    return null;
                  },
                ),
                const SizedBox(height: 16),

                // Password
                TextFormField(
                  key: const Key('passwordField'),
                  controller: _passwordController,
                  obscureText: _obscurePassword,
                  decoration: InputDecoration(
                    labelText: isRtl ? 'كلمة المرور' : 'Password',
                    suffixIcon: IconButton(
                      icon: Icon(
                        _obscurePassword
                            ? Icons.visibility_off
                            : Icons.visibility,
                      ),
                      onPressed: () => setState(
                        () => _obscurePassword = !_obscurePassword,
                      ),
                    ),
                  ),
                  validator: (value) {
                    if (value == null || value.isEmpty) {
                      return isRtl
                          ? 'كلمة المرور مطلوبة'
                          : 'Password is required';
                    }
                    if (value.length < 6) {
                      return isRtl
                          ? 'كلمة المرور يجب أن تكون 6 أحرف على الأقل'
                          : 'Password must be at least 6 characters';
                    }
                    return null;
                  },
                ),
                const SizedBox(height: 16),

                // Preferred language
                DropdownButtonFormField<String>(
                  key: const Key('languageDropdown'),
                  value: _selectedLanguage,
                  decoration: InputDecoration(
                    labelText: isRtl ? 'اللغة المفضلة' : 'Preferred Language',
                  ),
                  items: _supportedLanguages
                      .map(
                        (l) => DropdownMenuItem(
                          value: l.code,
                          child: Text(isRtl ? l.labelAr : l.labelEn),
                        ),
                      )
                      .toList(),
                  onChanged: (v) {
                    if (v != null) setState(() => _selectedLanguage = v);
                  },
                ),
                const SizedBox(height: 32),

                ElevatedButton(
                  key: const Key('submitButton'),
                  onPressed: isLoading ? null : _submit,
                  child: isLoading
                      ? const SizedBox(
                          height: 20,
                          width: 20,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : Text(isRtl ? 'إنشاء حساب' : 'Register'),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// LoginScreen
// ---------------------------------------------------------------------------

/// Login screen.
///
/// Flow:
/// 1. Signs in via Firebase [signInWithEmailAndPassword].
/// 2. Delegates to [AuthController.signIn] to create a backend session.
class LoginScreen extends ConsumerStatefulWidget {
  const LoginScreen({super.key, this.onSuccess});

  final VoidCallback? onSuccess;

  @override
  ConsumerState<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends ConsumerState<LoginScreen> {
  final _formKey = GlobalKey<FormState>();
  final _emailController = TextEditingController();
  final _passwordController = TextEditingController();
  bool _obscurePassword = true;

  @override
  void dispose() {
    _emailController.dispose();
    _passwordController.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    if (!_formKey.currentState!.validate()) return;

    await ref.read(authControllerProvider.notifier).signIn(
          email: _emailController.text.trim(),
          password: _passwordController.text,
        );

    if (!mounted) return;
    final authState = ref.read(authControllerProvider);
    if (authState.isAuthenticated) {
      widget.onSuccess?.call();
    } else if (authState.errorMessage != null) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(authState.errorMessage!)),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final isLoading = ref.watch(
      authControllerProvider.select((s) => s.isLoading),
    );
    final isRtl = Directionality.of(context) == TextDirection.rtl;

    return Scaffold(
      appBar: AppBar(
        title: Text(isRtl ? 'تسجيل الدخول' : 'Sign In'),
      ),
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: Form(
            key: _formKey,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                TextFormField(
                  key: const Key('emailField'),
                  controller: _emailController,
                  keyboardType: TextInputType.emailAddress,
                  decoration: InputDecoration(
                    labelText: isRtl ? 'البريد الإلكتروني' : 'Email',
                  ),
                  validator: (value) {
                    if (value == null || value.trim().isEmpty) {
                      return isRtl ? 'البريد مطلوب' : 'Email is required';
                    }
                    return null;
                  },
                ),
                const SizedBox(height: 16),
                TextFormField(
                  key: const Key('passwordField'),
                  controller: _passwordController,
                  obscureText: _obscurePassword,
                  decoration: InputDecoration(
                    labelText: isRtl ? 'كلمة المرور' : 'Password',
                    suffixIcon: IconButton(
                      icon: Icon(
                        _obscurePassword
                            ? Icons.visibility_off
                            : Icons.visibility,
                      ),
                      onPressed: () => setState(
                        () => _obscurePassword = !_obscurePassword,
                      ),
                    ),
                  ),
                  validator: (value) {
                    if (value == null || value.isEmpty) {
                      return isRtl
                          ? 'كلمة المرور مطلوبة'
                          : 'Password is required';
                    }
                    return null;
                  },
                ),
                const SizedBox(height: 32),
                ElevatedButton(
                  key: const Key('submitButton'),
                  onPressed: isLoading ? null : _submit,
                  child: isLoading
                      ? const SizedBox(
                          height: 20,
                          width: 20,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : Text(isRtl ? 'دخول' : 'Sign In'),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// LogoutButton
// ---------------------------------------------------------------------------

/// A button widget that triggers [AuthController.logout] when tapped.
///
/// Shows a loading indicator while logout is in progress.
class LogoutButton extends ConsumerWidget {
  const LogoutButton({super.key, this.onLoggedOut});

  final VoidCallback? onLoggedOut;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final isLoading = ref.watch(
      authControllerProvider.select((s) => s.isLoading),
    );
    final isRtl = Directionality.of(context) == TextDirection.rtl;

    return ElevatedButton(
      key: const Key('logoutButton'),
      onPressed: isLoading
          ? null
          : () async {
              await ref.read(authControllerProvider.notifier).logout();
              onLoggedOut?.call();
            },
      child: isLoading
          ? const SizedBox(
              height: 20,
              width: 20,
              child: CircularProgressIndicator(strokeWidth: 2),
            )
          : Text(isRtl ? 'تسجيل الخروج' : 'Logout'),
    );
  }
}
