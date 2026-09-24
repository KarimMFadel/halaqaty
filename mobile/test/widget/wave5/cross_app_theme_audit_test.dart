import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/core/theme/halaqaty_theme.dart';

/// Wave 5 cross-app audit (T044): theme-level contrast, bundled typography,
/// 48dp button enforcement, snackbar announcements, and reduced motion.
/// The regression assertions preserve the T045 findings after their fixes.

/// WCAG 2.x relative luminance of an sRGB color.
double _relativeLuminance(Color color) {
  double channel(double value) => value <= 0.04045
      ? value / 12.92
      : math.pow((value + 0.055) / 1.055, 2.4).toDouble();
  return 0.2126 * channel(color.r) +
      0.7152 * channel(color.g) +
      0.0722 * channel(color.b);
}

double _contrastRatio(Color foreground, Color background) {
  final fg = _relativeLuminance(foreground);
  final bg = _relativeLuminance(background);
  return (math.max(fg, bg) + 0.05) / (math.min(fg, bg) + 0.05);
}

/// Harness whose only actions surface the two shared snackbars; no
/// navigation, controllers, or APIs exist behind it.
Future<void> _pumpSnackBarHarness(
  WidgetTester tester, {
  TextDirection direction = TextDirection.ltr,
}) {
  return tester.pumpWidget(
    MaterialApp(
      theme: halaqatyLightTheme(),
      darkTheme: halaqatyDarkTheme(),
      home: Directionality(
        textDirection: direction,
        child: Scaffold(
          body: Builder(
            builder: (context) => Column(
              children: [
                FilledButton(
                  key: const Key('triggerError'),
                  onPressed: () => showHalaqatyError(context, 'Action failed'),
                  child: const Text('Trigger error'),
                ),
                FilledButton(
                  key: const Key('triggerNotice'),
                  onPressed: () =>
                      showHalaqatyUnderImplementationNotice(context),
                  child: const Text('Trigger notice'),
                ),
              ],
            ),
          ),
        ),
      ),
    ),
  );
}

void main() {
  group('WCAG AA contrast for key theme roles (FR-012)', () {
    final themes = <String, ThemeData>{
      'light': halaqatyLightTheme(),
      'dark': halaqatyDarkTheme(),
    };

    for (final entry in themes.entries) {
      final theme = entry.value;
      final scheme = theme.colorScheme;
      // (label, foreground, background, minimum ratio): 4.5:1 for normal
      // text, 3:1 for large text and essential graphics (FR-012).
      final pairs =
          <String, (Color foreground, Color background, double minimum)>{
        'onSurface/surface': (scheme.onSurface, scheme.surface, 4.5),
        'onSurface/scaffoldBackground': (
          scheme.onSurface,
          theme.scaffoldBackgroundColor,
          4.5,
        ),
        'onPrimary/primary': (scheme.onPrimary, scheme.primary, 4.5),
        'onSecondaryContainer/secondaryContainer (nav selected state)': (
          scheme.onSecondaryContainer,
          scheme.secondaryContainer,
          4.5,
        ),
        'onErrorContainer/errorContainer': (
          scheme.onErrorContainer,
          scheme.errorContainer,
          4.5,
        ),
        'onSurfaceVariant/surface (secondary text)': (
          scheme.onSurfaceVariant,
          scheme.surface,
          4.5,
        ),
        'error/surface (inline error text)': (
          scheme.error,
          scheme.surface,
          4.5
        ),
        'outline/surface (essential graphics)': (
          scheme.outline,
          scheme.surface,
          3.0,
        ),
      };

      for (final pair in pairs.entries) {
        test(
            '${entry.key} theme ${pair.key} meets '
            '${pair.value.$3}:1', () {
          final ratio = _contrastRatio(pair.value.$1, pair.value.$2);
          expect(
            ratio,
            greaterThanOrEqualTo(pair.value.$3),
            reason: '${entry.key} ${pair.key} measured '
                '${ratio.toStringAsFixed(2)}:1',
          );
        });
      }
    }
  });

  group('bundled typography mapping (spec clarification 2026-09-22)', () {
    final themes = <String, ThemeData>{
      'light': halaqatyLightTheme(),
      'dark': halaqatyDarkTheme(),
    };

    Iterable<MapEntry<String, TextStyle?>> roles(TextTheme textTheme) => {
          'displayLarge': textTheme.displayLarge,
          'displayMedium': textTheme.displayMedium,
          'displaySmall': textTheme.displaySmall,
          'headlineLarge': textTheme.headlineLarge,
          'headlineMedium': textTheme.headlineMedium,
          'headlineSmall': textTheme.headlineSmall,
          'titleLarge': textTheme.titleLarge,
          'titleMedium': textTheme.titleMedium,
          'titleSmall': textTheme.titleSmall,
          'bodyLarge': textTheme.bodyLarge,
          'bodyMedium': textTheme.bodyMedium,
          'bodySmall': textTheme.bodySmall,
          'labelLarge': textTheme.labelLarge,
          'labelMedium': textTheme.labelMedium,
          'labelSmall': textTheme.labelSmall,
        }.entries;

    for (final entry in themes.entries) {
      final textTheme = entry.value.textTheme;

      test('${entry.key} text theme uses only Poppins with Cairo fallback', () {
        for (final role in roles(textTheme)) {
          final style = role.value;
          expect(style, isNotNull, reason: '${role.key} must be set');
          expect(
            style!.fontFamily,
            HalaqatyFonts.primary,
            reason: '${role.key} must use Poppins',
          );
          expect(
            style.fontFamilyFallback,
            HalaqatyFonts.fallback,
            reason: '${role.key} must fall back to Cairo only',
          );
        }
      });

      test('${entry.key} display/headline/body roles request weight 400', () {
        for (final name in [
          'displayLarge',
          'displayMedium',
          'displaySmall',
          'headlineLarge',
          'headlineMedium',
          'headlineSmall',
          'bodyLarge',
          'bodyMedium',
          'bodySmall',
        ]) {
          final style =
              roles(textTheme).firstWhere((role) => role.key == name).value;
          expect(
            style?.fontWeight,
            FontWeight.w400,
            reason: '$name must request 400',
          );
        }
      });

      test('${entry.key} title/label/button roles request weight 600', () {
        for (final name in [
          'titleLarge',
          'titleMedium',
          'titleSmall',
          'labelLarge',
          'labelMedium',
          'labelSmall',
        ]) {
          final style =
              roles(textTheme).firstWhere((role) => role.key == name).value;
          expect(
            style?.fontWeight,
            FontWeight.w600,
            reason: '$name must request 600',
          );
        }
      });

      test('${entry.key} Arabic-serving styles keep height at least 1.5', () {
        for (final role in roles(textTheme)) {
          final height = role.value?.height;
          expect(
            height,
            anyOf(isNull, greaterThanOrEqualTo(1.5)),
            reason: '${role.key} height $height clips Arabic diacritics',
          );
        }
        // The frozen mapping sets heights on every role; a null height here
        // would mean the mapping silently regressed.
        expect(textTheme.bodyLarge?.height, isNotNull);
        expect(textTheme.titleLarge?.height, isNotNull);
      });
    }
  });

  group('theme-enforced 48dp button targets (FR-013)', () {
    final themes = <String, ThemeData>{
      'light': halaqatyLightTheme(),
      'dark': halaqatyDarkTheme(),
    };

    Size? minimumSizeOf(ButtonStyle? style) =>
        style?.minimumSize?.resolve(const <WidgetState>{});

    for (final entry in themes.entries) {
      final theme = entry.value;

      test('${entry.key} FilledButton theme enforces 48x48dp', () {
        final size = minimumSizeOf(theme.filledButtonTheme.style);
        expect(size, isNotNull);
        expect(size!.width, greaterThanOrEqualTo(48));
        expect(size.height, greaterThanOrEqualTo(48));
      });

      test('${entry.key} OutlinedButton theme enforces 48x48dp', () {
        final size = minimumSizeOf(theme.outlinedButtonTheme.style);
        expect(size, isNotNull);
        expect(size!.width, greaterThanOrEqualTo(48));
        expect(size.height, greaterThanOrEqualTo(48));
      });

      test('${entry.key} ElevatedButton theme enforces 48x48dp', () {
        final size = minimumSizeOf(theme.elevatedButtonTheme.style);
        expect(
          size,
          isNotNull,
          reason: 'theme must enforce the 48dp minimum for elevated buttons',
        );
        expect(size!.width, greaterThanOrEqualTo(48));
        expect(size.height, greaterThanOrEqualTo(48));
      });

      test('${entry.key} TextButton theme enforces 48x48dp', () {
        final size = minimumSizeOf(theme.textButtonTheme.style);
        expect(
          size,
          isNotNull,
          reason: 'theme must enforce the 48dp minimum for text buttons',
        );
        expect(size!.width, greaterThanOrEqualTo(48));
        expect(size.height, greaterThanOrEqualTo(48));
      });

      test('${entry.key} IconButton theme enforces 48x48dp', () {
        final style = theme.iconButtonTheme.style;
        final size = minimumSizeOf(style);
        final constraints = style?.fixedSize?.resolve(const <WidgetState>{});
        expect(
          size ?? constraints,
          isNotNull,
          reason: 'theme must enforce the 48dp minimum for icon buttons',
        );
        final enforced = size ?? constraints!;
        expect(enforced.width, greaterThanOrEqualTo(48));
        expect(enforced.height, greaterThanOrEqualTo(48));
      });
    }
  });

  group('dynamic announcements (FR-014)', () {
    /// The snackbar's live-region Semantics is built inside its state, below
    /// the keyed widget, so assert it in the widget subtree.
    Finder liveRegionIn(Finder snackBar) => find.descendant(
          of: snackBar,
          matching: find.byWidgetPredicate(
            (widget) =>
                widget is Semantics && widget.properties.liveRegion == true,
          ),
        );

    testWidgets('error snackbar carries live-region semantics', (tester) async {
      await _pumpSnackBarHarness(tester);

      await tester.tap(find.byKey(const Key('triggerError')));
      await tester.pump();

      final snackBar = find.byKey(const Key('halaqatyErrorSnackBar'));
      expect(snackBar, findsOneWidget);
      expect(
        liveRegionIn(snackBar),
        findsOneWidget,
        reason: 'material dynamic changes must be announced (FR-014)',
      );
    });

    testWidgets('under-implementation snackbar carries live-region semantics',
        (tester) async {
      await _pumpSnackBarHarness(tester);

      await tester.tap(find.byKey(const Key('triggerNotice')));
      await tester.pump();

      final snackBar =
          find.byKey(const Key('halaqatyUnderImplementationSnackBar'));
      expect(snackBar, findsOneWidget);
      expect(
        liveRegionIn(snackBar),
        findsOneWidget,
        reason: 'material dynamic changes must be announced (FR-014)',
      );
    });
  });

  group('shared notice target size (FR-013/FR-033)', () {
    testWidgets('the notice dismiss action meets 48dp', (tester) async {
      await _pumpSnackBarHarness(tester);

      await tester.tap(find.byKey(const Key('triggerNotice')));
      await tester.pumpAndSettle();

      final actionButton = find.ancestor(
        of: find.text('Dismiss'),
        matching: find.byType(TextButton),
      );
      expect(actionButton, findsOneWidget);
      final size = tester.getSize(actionButton);
      expect(
        size.height,
        greaterThanOrEqualTo(48),
        reason: 'the notice action is an interactive target (FR-013)',
      );
      expect(size.width, greaterThanOrEqualTo(48));
    });
  });

  group('reduced motion (FR-016)', () {
    testWidgets(
        'the shared snackbar skips its entrance animation when reduced '
        'motion is requested', (tester) async {
      tester.platformDispatcher.accessibilityFeaturesTestValue =
          const FakeAccessibilityFeatures(disableAnimations: true);
      addTearDown(
        tester.platformDispatcher.clearAccessibilityFeaturesTestValue,
      );

      await _pumpSnackBarHarness(tester);
      expect(
        MediaQuery.disableAnimationsOf(
          tester.element(find.byKey(const Key('triggerNotice'))),
        ),
        isTrue,
      );

      await tester.tap(find.byKey(const Key('triggerNotice')));
      await tester.pump();
      // 100ms in, a 250–400ms entrance animation would still be mid-flight;
      // reduced motion must present the notice immediately.
      await tester.pump(const Duration(milliseconds: 100));

      final fades = find.ancestor(
        of: find.byKey(const Key('halaqatyUnderImplementationSnackBar')),
        matching: find.byType(FadeTransition),
      );
      expect(fades, findsWidgets);
      for (var i = 0; i < fades.evaluate().length; i++) {
        final fade = tester.widget<FadeTransition>(fades.at(i));
        expect(
          fade.opacity.value,
          1.0,
          reason: 'reduced motion must suppress nonessential transitions '
              '(FR-016)',
        );
      }
    });
  });
}
