import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/app/app_locale_controller.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/features/auth/presentation/auth_screens.dart';
import 'package:halaqaty_mobile/features/profile/application/profile_controller.dart';
import 'package:halaqaty_mobile/features/profile/data/profile_api_client.dart';

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

/// Profile screen: the existing F-001 profile form grouped into profile
/// details, preferences, and account sections.
///
/// Approved future-intended settings (appearance, notifications, privacy,
/// support) stay visible but invoke only the shared under-implementation
/// notice (FR-032/FR-033); avatar upload and account deletion stay omitted.
class ProfileScreen extends ConsumerStatefulWidget {
  const ProfileScreen({super.key, this.onSaved});

  final VoidCallback? onSaved;

  @override
  ConsumerState<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends ConsumerState<ProfileScreen> {
  final _formKey = GlobalKey<FormState>();
  final _fullNameController = TextEditingController();
  final _displayNameController = TextEditingController();
  final _bioController = TextEditingController();
  final _countryController = TextEditingController();
  final _avatarUrlController = TextEditingController();
  final _phoneController = TextEditingController();

  String _selectedLanguage = 'ar';
  bool _didSeedFromProfile = false;
  bool _saveSucceeded = false;

  @override
  void initState() {
    super.initState();
    Future.microtask(
        () => ref.read(profileControllerProvider.notifier).loadProfile());
  }

  @override
  void dispose() {
    _fullNameController.dispose();
    _displayNameController.dispose();
    _bioController.dispose();
    _countryController.dispose();
    _avatarUrlController.dispose();
    _phoneController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(profileControllerProvider);
    final isRtl = Directionality.of(context) == TextDirection.rtl;

    // A loaded profile replaces the locale with its preferred language
    // (FR-029).
    ref.listen(
      profileControllerProvider.select((s) => s.profile?.preferredLanguage),
      (previous, next) {
        if (next != null && next != previous) {
          ref
              .read(appLocaleControllerProvider.notifier)
              .applyProfileLanguage(next);
        }
      },
    );

    if (state.profile != null && !_didSeedFromProfile) {
      _seedFromProfile(state.profile!);
    }

    return Scaffold(
      appBar: AppBar(title: Text(isRtl ? 'الملف الشخصي' : 'Profile')),
      body: SafeArea(
        child: state.isLoading && state.profile == null
            ? const HalaqatyLoading()
            : state.profile == null && state.errorMessage != null
                ? _LoadError(
                    message: state.errorMessage!,
                    onRetry: () => ref
                        .read(profileControllerProvider.notifier)
                        .loadProfile(),
                  )
                : SingleChildScrollView(
                    padding: const EdgeInsets.all(24),
                    child: Form(
                      key: _formKey,
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          SectionHeader(
                            title: isRtl
                                ? 'تفاصيل الملف الشخصي'
                                : 'Profile details',
                          ),
                          TextFormField(
                            key: const Key('profileFullNameField'),
                            controller: _fullNameController,
                            textInputAction: TextInputAction.next,
                            textDirection:
                                isRtl ? TextDirection.rtl : TextDirection.ltr,
                            decoration: InputDecoration(
                              labelText: isRtl ? 'الاسم الكامل' : 'Full Name',
                              errorText: state.fieldErrors['full_name'],
                            ),
                            validator: (value) {
                              final v = value?.trim() ?? '';
                              if (v.isEmpty) {
                                return isRtl
                                    ? 'الاسم الكامل مطلوب'
                                    : 'Full name is required';
                              }
                              if (v.length < 2) {
                                return isRtl
                                    ? 'الاسم الكامل يجب أن يكون حرفين على الأقل'
                                    : 'Full name must be at least 2 characters';
                              }
                              return null;
                            },
                          ),
                          const SizedBox(height: 16),
                          TextFormField(
                            key: const Key('profileDisplayNameField'),
                            controller: _displayNameController,
                            textInputAction: TextInputAction.next,
                            textDirection:
                                isRtl ? TextDirection.rtl : TextDirection.ltr,
                            decoration: InputDecoration(
                              labelText:
                                  isRtl ? 'الاسم المعروض' : 'Display Name',
                              errorText: state.fieldErrors['display_name'],
                            ),
                          ),
                          const SizedBox(height: 16),
                          TextFormField(
                            key: const Key('profileBioField'),
                            controller: _bioController,
                            maxLines: 3,
                            textDirection:
                                isRtl ? TextDirection.rtl : TextDirection.ltr,
                            decoration: InputDecoration(
                              labelText: isRtl ? 'نبذة' : 'Bio',
                              hintText: isRtl ? 'اختياري' : 'Optional',
                              errorText: state.fieldErrors['bio'],
                            ),
                          ),
                          const SizedBox(height: 16),
                          TextFormField(
                            key: const Key('profileCountryField'),
                            controller: _countryController,
                            textInputAction: TextInputAction.next,
                            decoration: InputDecoration(
                              labelText: isRtl ? 'الدولة' : 'Country',
                              hintText: isRtl ? 'مثل: EG' : 'e.g. EG',
                              errorText: state.fieldErrors['country'],
                            ),
                            validator: (value) {
                              final v = value?.trim() ?? '';
                              if (v.isEmpty) {
                                return isRtl
                                    ? 'الدولة مطلوبة'
                                    : 'Country is required';
                              }
                              if (v.length != 2) {
                                return isRtl
                                    ? 'رمز الدولة يجب أن يكون حرفين'
                                    : 'Country code must be 2 letters';
                              }
                              return null;
                            },
                          ),
                          const SizedBox(height: 16),
                          TextFormField(
                            key: const Key('profileAvatarUrlField'),
                            controller: _avatarUrlController,
                            keyboardType: TextInputType.url,
                            textInputAction: TextInputAction.next,
                            decoration: InputDecoration(
                              labelText: isRtl ? 'رابط الصورة' : 'Avatar URL',
                              hintText: isRtl ? 'اختياري' : 'Optional',
                              errorText: state.fieldErrors['avatar_url'],
                            ),
                          ),
                          const SizedBox(height: 16),
                          TextFormField(
                            key: const Key('profilePhoneField'),
                            controller: _phoneController,
                            keyboardType: TextInputType.phone,
                            textInputAction: TextInputAction.done,
                            decoration: InputDecoration(
                              labelText: isRtl ? 'الهاتف' : 'Phone',
                              hintText: isRtl ? 'اختياري' : 'Optional',
                              errorText: state.fieldErrors['phone'],
                            ),
                          ),
                          const SizedBox(height: 32),
                          SectionHeader(
                            title: isRtl ? 'التفضيلات' : 'Preferences',
                          ),
                          DropdownButtonFormField<String>(
                            key: const Key('profileLanguageDropdown'),
                            initialValue: _selectedLanguage,
                            decoration: InputDecoration(
                              labelText: isRtl
                                  ? 'اللغة المفضلة'
                                  : 'Preferred Language',
                              errorText:
                                  state.fieldErrors['preferred_language'],
                            ),
                            items: _supportedLanguages
                                .map(
                                  (l) => DropdownMenuItem(
                                    value: l.code,
                                    child: Text(isRtl ? l.labelAr : l.labelEn),
                                  ),
                                )
                                .toList(),
                            onChanged: (value) {
                              if (value != null) {
                                setState(() => _selectedLanguage = value);
                              }
                            },
                          ),
                          const SizedBox(height: 8),
                          _NoticeTile(
                            icon: Icons.palette_outlined,
                            label: isRtl ? 'المظهر' : 'Appearance',
                          ),
                          _NoticeTile(
                            icon: Icons.notifications_outlined,
                            label: isRtl ? 'الإشعارات' : 'Notifications',
                          ),
                          const SizedBox(height: 24),
                          FilledButton(
                            key: const Key('profileSaveButton'),
                            onPressed: state.isSaving ? null : _submit,
                            child: state.isSaving
                                ? SizedBox(
                                    height: 20,
                                    width: 20,
                                    child: CircularProgressIndicator(
                                      strokeWidth: 2,
                                      color: Theme.of(context)
                                          .colorScheme
                                          .onPrimary,
                                    ),
                                  )
                                : Text(isRtl ? 'حفظ' : 'Save'),
                          ),
                          if (_saveSucceeded) ...[
                            const SizedBox(height: 12),
                            _SaveSuccessBanner(
                                message: isRtl
                                    ? 'تم تحديث الملف الشخصي'
                                    : 'Profile updated'),
                          ],
                          if (state.errorMessage != null &&
                              state.fieldErrors.isEmpty) ...[
                            const SizedBox(height: 12),
                            _InlineError(message: state.errorMessage!),
                          ],
                          const SizedBox(height: 32),
                          SectionHeader(
                            title:
                                isRtl ? 'الحساب والدعم' : 'Account & support',
                          ),
                          _NoticeTile(
                            icon: Icons.lock_outline,
                            label: isRtl
                                ? 'الخصوصية والأمان'
                                : 'Privacy & security',
                          ),
                          _NoticeTile(
                            icon: Icons.help_outline,
                            label: isRtl ? 'المساعدة والدعم' : 'Help & support',
                          ),
                          const SizedBox(height: 24),
                          const LogoutButton(),
                          const SizedBox(height: 24),
                        ],
                      ),
                    ),
                  ),
      ),
    );
  }

  void _seedFromProfile(ProfileUser profile) {
    _fullNameController.text = profile.fullName ?? '';
    _displayNameController.text = profile.displayName;
    _bioController.text = profile.bio ?? '';
    _countryController.text = profile.country ?? '';
    _avatarUrlController.text = profile.avatarUrl ?? '';
    _phoneController.text = profile.phone ?? '';
    _selectedLanguage = profile.preferredLanguage;
    _didSeedFromProfile = true;
  }

  Future<void> _submit() async {
    setState(() => _saveSucceeded = false);
    if (!_formKey.currentState!.validate()) {
      return;
    }

    final success =
        await ref.read(profileControllerProvider.notifier).updateProfile(
              request: UpdateProfileRequest(
                fullName: _fullNameController.text.trim(),
                displayName: _displayNameController.text.trim(),
                bio: _bioController.text.trim(),
                country: _countryController.text.trim().toUpperCase(),
                preferredLanguage: _selectedLanguage,
                avatarUrl: _avatarUrlController.text.trim(),
                phone: _phoneController.text.trim(),
              ),
            );

    if (!mounted || !success) {
      return;
    }
    // A successful save updates the app locale and direction.
    ref
        .read(appLocaleControllerProvider.notifier)
        .applyProfileLanguage(_selectedLanguage);
    // Consequential success stays visible in context (FR-008) — retained
    // until the next save attempt, not a transient snackbar.
    setState(() => _saveSucceeded = true);
    widget.onSaved?.call();
  }
}

/// Branded load-error state: explains the failure and retries in place.
class _LoadError extends StatelessWidget {
  const _LoadError({required this.message, required this.onRetry});

  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final isRtl = Directionality.of(context) == TextDirection.rtl;
    final scheme = Theme.of(context).colorScheme;
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.cloud_off_outlined,
                size: 48, color: scheme.onSurfaceVariant),
            const SizedBox(height: 16),
            Text(
              message,
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodyLarge,
            ),
            const SizedBox(height: 16),
            OutlinedButton(
              onPressed: onRetry,
              child: Text(isRtl ? 'إعادة المحاولة' : 'Retry'),
            ),
          ],
        ),
      ),
    );
  }
}

/// Retained in-context save confirmation (key `profileSaveSuccess`).
class _SaveSuccessBanner extends StatelessWidget {
  const _SaveSuccessBanner({required this.message});

  final String message;

  @override
  Widget build(BuildContext context) {
    // secondaryContainer/onSecondaryContainer: ≥4.5:1 in both themes (the
    // Wave 2 token amendment); primaryContainer/onPrimaryContainer measures
    // only ~3.2:1.
    final scheme = Theme.of(context).colorScheme;
    return Semantics(
      container: true,
      liveRegion: true,
      label: message,
      child: Container(
        key: const Key('profileSaveSuccess'),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        decoration: BoxDecoration(
          color: scheme.secondaryContainer,
          borderRadius: BorderRadius.circular(12),
        ),
        child: Row(
          children: [
            Icon(Icons.check_circle, color: scheme.onSecondaryContainer),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                message,
                style: Theme.of(context)
                    .textTheme
                    .bodyMedium
                    ?.copyWith(color: scheme.onSecondaryContainer),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// In-context general save/load error (icon + copy, not color alone).
class _InlineError extends StatelessWidget {
  const _InlineError({required this.message});

  final String message;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Icon(Icons.error_outline, color: scheme.error),
        const SizedBox(width: 8),
        Expanded(
          child: Text(
            message,
            style: Theme.of(context)
                .textTheme
                .bodyMedium
                ?.copyWith(color: scheme.error),
          ),
        ),
      ],
    );
  }
}

/// An approved future-intended action that has no behavior yet: tapping it
/// shows only the shared under-implementation notice (FR-032/FR-033).
class _NoticeTile extends StatelessWidget {
  const _NoticeTile({required this.icon, required this.label});

  final IconData icon;
  final String label;

  @override
  Widget build(BuildContext context) {
    return ListTile(
      contentPadding: EdgeInsets.zero,
      leading: Icon(icon),
      title: Text(label),
      trailing: const Icon(Icons.arrow_forward_ios, size: 16),
      onTap: () => showHalaqatyUnderImplementationNotice(context),
    );
  }
}
