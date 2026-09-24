import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Platform locale as a provider so widget tests can pin it.
final platformLocaleProvider = Provider<Locale>((ref) {
  return WidgetsBinding.instance.platformDispatcher.locale;
});

/// Owns the normalized `ar`/`en` UI locale (FR-029). Registration selection
/// applies immediately; profile load/save replaces it; logout restores the
/// platform fallback. Unsupported or missing values normalize to `ar`.
class AppLocaleController extends StateNotifier<Locale> {
  AppLocaleController({required Locale Function() readPlatformLocale})
      : _readPlatformLocale = readPlatformLocale,
        super(normalize(readPlatformLocale()));

  final Locale Function() _readPlatformLocale;

  /// Normalizes any locale to `ar` or `en`; unsupported/missing → `ar`.
  static Locale normalize(Locale? locale) {
    return switch (locale?.languageCode) {
      'en' => const Locale('en'),
      _ => const Locale('ar'),
    };
  }

  /// Applies the registration language choice immediately.
  void selectLanguage(String languageCode) {
    state = normalize(Locale(languageCode));
  }

  /// Applies a loaded/saved profile `preferred_language` (null-safe).
  void applyProfileLanguage(String? preferredLanguage) {
    state = normalize(
      preferredLanguage == null ? null : Locale(preferredLanguage),
    );
  }

  /// Restores the platform-derived fallback after logout.
  void restorePlatformFallback() {
    state = normalize(_readPlatformLocale());
  }
}

final appLocaleControllerProvider =
    StateNotifierProvider<AppLocaleController, Locale>((ref) {
  final platformLocale = ref.watch(platformLocaleProvider);
  return AppLocaleController(readPlatformLocale: () => platformLocale);
});
