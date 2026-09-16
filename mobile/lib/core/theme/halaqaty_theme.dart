import 'package:flutter/material.dart';

/// Halaqaty brand color tokens from `docs/engineering/design/DESIGN.md`.
///
/// These are the only color literals allowed in the app; screens must read
/// colors from `Theme.of(context).colorScheme`.
abstract final class HalaqatyColors {
  /// Primary — Islamic Green.
  static const Color primary = Color(0xFF1B7E3C);

  /// Primary light variant (hover, dark-mode primary).
  static const Color primaryLight = Color(0xFF4CB368);

  /// Primary dark variant (focus, dark-mode container).
  static const Color primaryDark = Color(0xFF0F5627);

  /// Secondary — Quranic Gold (accent only, never large fills).
  static const Color secondary = Color(0xFFD4A574);

  /// Secondary light variant.
  static const Color secondaryLight = Color(0xFFE8C8A0);

  /// Secondary dark variant.
  static const Color secondaryDark = Color(0xFF8B6F47);

  // Semantic (light, dark) per DESIGN.md.
  static const Color successLight = Color(0xFF2E7D32);
  static const Color successDark = Color(0xFF66BB6A);
  static const Color warningLight = Color(0xFFF57C00);
  static const Color warningDark = Color(0xFFFFB74D);
  static const Color errorLight = Color(0xFFC62828);
  static const Color errorDark = Color(0xFFEF5350);
  static const Color infoLight = Color(0xFF0277BD);
  static const Color infoDark = Color(0xFF29B6F6);

  // Neutrals (light, dark) per DESIGN.md.
  static const Color surfaceLight = Color(0xFFFFFFFF);
  static const Color surfaceDark = Color(0xFF121212);
  static const Color surfaceVariantLight = Color(0xFFF5F5F5);
  static const Color surfaceVariantDark = Color(0xFF2C2C2C);
  static const Color backgroundLight = Color(0xFFFAFAFA);
  static const Color backgroundDark = Color(0xFF0A0A0A);
  static const Color neutralLight = Color(0xFF616161);
  static const Color neutralDark = Color(0xFFBDBDBD);
  static const Color outlineLight = Color(0xFF79747E);
  static const Color outlineDark = Color(0xFF928F96);
  static const Color outlineVariantLight = Color(0xFFCAC7D0);
  static const Color outlineVariantDark = Color(0xFF49454E);

  static const Color onSurfaceLight = Color(0xFF1C1C1E);
  static const Color onSurfaceDark = Color(0xFFEDEDED);
}

/// Font family names declared in `pubspec.yaml`.
abstract final class HalaqatyFonts {
  /// Latin headings/body.
  static const String primary = 'Poppins';

  /// Arabic UI text — used as glyph fallback for Arabic script.
  static const String arabic = 'Cairo';

  /// Fallback chain so Arabic glyphs inside primary-font text render in Cairo.
  static const List<String> fallback = [arabic];
}

/// Builds the Halaqaty light theme (reference theme).
ThemeData halaqatyLightTheme() => _buildTheme(_lightScheme);

/// Builds the Halaqaty dark theme (per DESIGN.md dark column).
ThemeData halaqatyDarkTheme() => _buildTheme(_darkScheme);

final ColorScheme _lightScheme = ColorScheme.light(
  primary: HalaqatyColors.primary,
  onPrimary: Colors.white,
  primaryContainer: HalaqatyColors.primaryLight,
  onPrimaryContainer: HalaqatyColors.primaryDark,
  secondary: HalaqatyColors.secondary,
  onSecondary: Colors.white,
  secondaryContainer: HalaqatyColors.secondaryLight,
  onSecondaryContainer: HalaqatyColors.secondaryDark,
  error: HalaqatyColors.errorLight,
  surface: HalaqatyColors.surfaceLight,
  onSurface: HalaqatyColors.onSurfaceLight,
  surfaceContainerHighest: HalaqatyColors.surfaceVariantLight,
  onSurfaceVariant: HalaqatyColors.neutralLight,
  outline: HalaqatyColors.outlineLight,
  outlineVariant: HalaqatyColors.outlineVariantLight,
);

final ColorScheme _darkScheme = ColorScheme.dark(
  primary: HalaqatyColors.primaryLight,
  onPrimary: HalaqatyColors.primaryDark,
  primaryContainer: HalaqatyColors.primaryDark,
  onPrimaryContainer: HalaqatyColors.primaryLight,
  secondary: HalaqatyColors.secondary,
  onSecondary: HalaqatyColors.secondaryDark,
  secondaryContainer: HalaqatyColors.secondaryDark,
  onSecondaryContainer: HalaqatyColors.secondaryLight,
  error: HalaqatyColors.errorDark,
  surface: HalaqatyColors.surfaceDark,
  onSurface: HalaqatyColors.onSurfaceDark,
  surfaceContainerHighest: HalaqatyColors.surfaceVariantDark,
  onSurfaceVariant: HalaqatyColors.neutralDark,
  outline: HalaqatyColors.outlineDark,
  outlineVariant: HalaqatyColors.outlineVariantDark,
);

ThemeData _buildTheme(ColorScheme scheme) {
  final isLight = scheme.brightness == Brightness.light;
  return ThemeData(
    useMaterial3: true,
    colorScheme: scheme,
    fontFamily: HalaqatyFonts.primary,
    fontFamilyFallback: HalaqatyFonts.fallback,
    scaffoldBackgroundColor: isLight
        ? HalaqatyColors.backgroundLight
        : HalaqatyColors.backgroundDark,
    appBarTheme: AppBarTheme(
      backgroundColor: isLight
          ? HalaqatyColors.backgroundLight
          : HalaqatyColors.backgroundDark,
      foregroundColor: scheme.onSurface,
      centerTitle: true,
    ),
    inputDecorationTheme: InputDecorationTheme(
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(8),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(8),
        borderSide: BorderSide(color: scheme.primary, width: 2),
      ),
    ),
    cardTheme: CardThemeData(
      elevation: 1,
      color: scheme.surface,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        minimumSize: const Size(64, 48),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        minimumSize: const Size(64, 48),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      ),
    ),
    navigationBarTheme: NavigationBarThemeData(
      backgroundColor: scheme.surface,
      indicatorColor: scheme.secondaryContainer,
      labelTextStyle: const WidgetStatePropertyAll(
        TextStyle(fontSize: 12, fontWeight: FontWeight.w600),
      ),
    ),
  );
}
