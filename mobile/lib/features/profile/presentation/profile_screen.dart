import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
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

    if (state.profile != null && !_didSeedFromProfile) {
      _seedFromProfile(state.profile!);
    }

    return Scaffold(
      appBar: AppBar(title: Text(isRtl ? 'الملف الشخصي' : 'Profile')),
      body: SafeArea(
        child: state.isLoading && state.profile == null
            ? const Center(child: CircularProgressIndicator())
            : SingleChildScrollView(
                padding: const EdgeInsets.all(24),
                child: Form(
                  key: _formKey,
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      TextFormField(
                        key: const Key('profileFullNameField'),
                        controller: _fullNameController,
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
                        textDirection:
                            isRtl ? TextDirection.rtl : TextDirection.ltr,
                        decoration: InputDecoration(
                          labelText: isRtl ? 'الاسم المعروض' : 'Display Name',
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
                          errorText: state.fieldErrors['bio'],
                        ),
                      ),
                      const SizedBox(height: 16),
                      TextFormField(
                        key: const Key('profileCountryField'),
                        controller: _countryController,
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
                      DropdownButtonFormField<String>(
                        key: const Key('profileLanguageDropdown'),
                        initialValue: _selectedLanguage,
                        decoration: InputDecoration(
                          labelText:
                              isRtl ? 'اللغة المفضلة' : 'Preferred Language',
                          errorText: state.fieldErrors['preferred_language'],
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
                      const SizedBox(height: 16),
                      TextFormField(
                        key: const Key('profileAvatarUrlField'),
                        controller: _avatarUrlController,
                        keyboardType: TextInputType.url,
                        decoration: InputDecoration(
                          labelText: isRtl ? 'رابط الصورة' : 'Avatar URL',
                          errorText: state.fieldErrors['avatar_url'],
                        ),
                      ),
                      const SizedBox(height: 16),
                      TextFormField(
                        key: const Key('profilePhoneField'),
                        controller: _phoneController,
                        keyboardType: TextInputType.phone,
                        decoration: InputDecoration(
                          labelText: isRtl ? 'الهاتف' : 'Phone',
                          errorText: state.fieldErrors['phone'],
                        ),
                      ),
                      const SizedBox(height: 24),
                      ElevatedButton(
                        key: const Key('profileSaveButton'),
                        onPressed: state.isSaving ? null : _submit,
                        child: state.isSaving
                            ? const SizedBox(
                                height: 20,
                                width: 20,
                                child:
                                    CircularProgressIndicator(strokeWidth: 2),
                              )
                            : Text(isRtl ? 'حفظ' : 'Save'),
                      ),
                      if (state.errorMessage != null &&
                          state.fieldErrors.isEmpty) ...[
                        const SizedBox(height: 12),
                        Text(
                          state.errorMessage!,
                          style: TextStyle(
                              color: Theme.of(context).colorScheme.error),
                        ),
                      ],
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
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(
          Directionality.of(context) == TextDirection.rtl
              ? 'تم تحديث الملف الشخصي'
              : 'Profile updated',
        ),
      ),
    );
    widget.onSaved?.call();
  }
}
