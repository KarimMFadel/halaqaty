import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';

/// Halaqaty brand color tokens from `docs/engineering/design/DESIGN.md`.
///
/// These are the only color literals allowed in the app; screens must read
/// colors from `Theme.of(context).colorScheme`.
abstract final class HalaqatyColors {
  /// Primary — Islamic Green.
  static const Color primary = Color(0xFF1B7E3C);

  /// Primary light variant for light-mode containers.
  static const Color primaryLight = Color(0xFF4CB368);

  /// Primary dark variant for light-mode text and focus states.
  static const Color primaryDark = Color(0xFF0F5627);

  /// Garden dusk dark-mode primary and its readable foreground.
  static const Color darkPrimary = Color(0xFFB5C9AE);
  static const Color darkOnPrimary = Color(0xFF20291F);
  static const Color darkPrimaryContainer = Color(0xFF354638);
  static const Color darkOnPrimaryContainer = Color(0xFFD9E5D4);

  /// Secondary — Quranic Gold (accent only, never large fills).
  static const Color secondary = Color(0xFFD4A574);

  /// Secondary light variant.
  static const Color secondaryLight = Color(0xFFE8C8A0);

  // Token amendment 2026-09-22 (UI review, F-019 Wave 2): the
  // secondaryContainer/onSecondaryContainer pairs measured 2.96:1 in both
  // schemes and dark onPrimary 3.34:1 — below the 4.5:1 text-on-fill floor.
  // The light scheme keeps its contrast-safe gold roles. Garden dusk below
  // defines separate muted roles for dark appearance.

  /// Text/icons on the light secondary container (#E8C8A0) — 8.16:1.
  static const Color onSecondaryContainerLight = Color(0xFF3F2E1B);

  /// Garden dusk gold roles for the dark appearance.
  static const Color darkSecondary = Color(0xFFD6C08A);
  static const Color darkOnSecondary = Color(0xFF302A1F);
  static const Color darkSecondaryContainer = Color(0xFF493F2C);
  static const Color darkOnSecondaryContainer = Color(0xFFE8D8B1);

  // Semantic (light, dark) per DESIGN.md.
  static const Color successLight = Color(0xFF2E7D32);
  static const Color successDark = Color(0xFFA3C58F);
  static const Color warningLight = Color(0xFFF57C00);
  static const Color warningDark = Color(0xFFD6B27B);
  static const Color errorLight = Color(0xFFC62828);
  static const Color errorDark = Color(0xFFD97970);
  static const Color infoLight = Color(0xFF0277BD);
  static const Color infoDark = Color(0xFF91B9C2);

  // Neutrals (light, dark) per DESIGN.md.
  static const Color surfaceLight = Color(0xFFFFFFFF);
  static const Color surfaceDark = Color(0xFF202722);
  static const Color surfaceVariantLight = Color(0xFFF5F5F5);
  static const Color surfaceVariantDark = Color(0xFF28312B);
  static const Color backgroundLight = Color(0xFFF8F6F0);
  static const Color backgroundDark = Color(0xFF151816);
  static const Color neutralLight = Color(0xFF616161);
  static const Color neutralDark = Color(0xFFB3BCB2);
  static const Color outlineLight = Color(0xFF79747E);
  static const Color outlineDark = Color(0xFF7C877D);
  static const Color outlineVariantLight = Color(0xFFCAC7D0);
  static const Color outlineVariantDark = Color(0xFF465047);

  static const Color onSurfaceLight = Color(0xFF1C1C1E);
  static const Color onSurfaceDark = Color(0xFFE6E9E3);
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
  onSecondaryContainer: HalaqatyColors.onSecondaryContainerLight,
  error: HalaqatyColors.errorLight,
  surface: HalaqatyColors.surfaceLight,
  onSurface: HalaqatyColors.onSurfaceLight,
  surfaceContainerHighest: HalaqatyColors.surfaceVariantLight,
  onSurfaceVariant: HalaqatyColors.neutralLight,
  outline: HalaqatyColors.outlineLight,
  outlineVariant: HalaqatyColors.outlineVariantLight,
);

final ColorScheme _darkScheme = ColorScheme.dark(
  primary: HalaqatyColors.darkPrimary,
  onPrimary: HalaqatyColors.darkOnPrimary,
  primaryContainer: HalaqatyColors.darkPrimaryContainer,
  onPrimaryContainer: HalaqatyColors.darkOnPrimaryContainer,
  secondary: HalaqatyColors.darkSecondary,
  onSecondary: HalaqatyColors.darkOnSecondary,
  secondaryContainer: HalaqatyColors.darkSecondaryContainer,
  onSecondaryContainer: HalaqatyColors.darkOnSecondaryContainer,
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

  // Visible keyboard-focus indication (DESIGN.md: Primary Dark #0F5627 is the
  // focus-state token; dark mode uses the lifted primary for contrast). The
  // default 10% state-layer overlay is indistinguishable on filled buttons.
  final focusedSide = WidgetStateProperty.resolveWith<BorderSide?>(
    (states) => states.contains(WidgetState.focused)
        ? BorderSide(
            color: isLight
                ? HalaqatyColors.primaryDark
                : HalaqatyColors.darkPrimary,
            width: 2,
          )
        : null,
  );

  // Bundled-font mapping (spec clarification 2026-09-22): title/label get
  // w600 (Poppins SemiBold / Cairo Bold), the rest w400; height 1.5 keeps
  // Arabic diacritics unclipped. No new font assets, no synthesized weights.
  final baseTypography =
      Typography.material2021(platform: defaultTargetPlatform);
  final baseTextTheme = isLight ? baseTypography.black : baseTypography.white;
  TextStyle? emphasis(TextStyle? style) =>
      style?.copyWith(fontWeight: FontWeight.w600, height: 1.5);
  TextStyle? regular(TextStyle? style) =>
      style?.copyWith(fontWeight: FontWeight.w400, height: 1.5);
  final textTheme = baseTextTheme
      .copyWith(
        displayLarge: regular(baseTextTheme.displayLarge),
        displayMedium: regular(baseTextTheme.displayMedium),
        displaySmall: regular(baseTextTheme.displaySmall),
        headlineLarge: regular(baseTextTheme.headlineLarge),
        headlineMedium: regular(baseTextTheme.headlineMedium),
        headlineSmall: regular(baseTextTheme.headlineSmall),
        titleLarge: emphasis(baseTextTheme.titleLarge),
        titleMedium: emphasis(baseTextTheme.titleMedium),
        titleSmall: emphasis(baseTextTheme.titleSmall),
        bodyLarge: regular(baseTextTheme.bodyLarge),
        bodyMedium: regular(baseTextTheme.bodyMedium),
        bodySmall: regular(baseTextTheme.bodySmall),
        labelLarge: emphasis(baseTextTheme.labelLarge),
        labelMedium: emphasis(baseTextTheme.labelMedium),
        labelSmall: emphasis(baseTextTheme.labelSmall),
        // The material2021 platform bases hard-code 'Roboto'; apply() overrides
        // every role so the bundled Poppins family (Cairo fallback) actually wins.
      )
      .apply(
        fontFamily: HalaqatyFonts.primary,
        fontFamilyFallback: HalaqatyFonts.fallback,
      );

  return ThemeData(
    useMaterial3: true,
    colorScheme: scheme,
    fontFamily: HalaqatyFonts.primary,
    fontFamilyFallback: HalaqatyFonts.fallback,
    textTheme: textTheme,
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
      ).copyWith(side: focusedSide),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        minimumSize: const Size(64, 48),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      ).copyWith(side: focusedSide),
    ),
    elevatedButtonTheme: ElevatedButtonThemeData(
      style: ElevatedButton.styleFrom(
        minimumSize: const Size(64, 48),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      ).copyWith(side: focusedSide),
    ),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(
        minimumSize: const Size(64, 48),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      ).copyWith(side: focusedSide),
    ),
    iconButtonTheme: IconButtonThemeData(
      style: IconButton.styleFrom(minimumSize: const Size(48, 48)),
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
