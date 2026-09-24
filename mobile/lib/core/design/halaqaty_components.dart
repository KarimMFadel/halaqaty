import 'package:flutter/material.dart';
import 'package:flutter_svg/flutter_svg.dart';

/// Path of the primary brand logo asset.
const String halaqatyLogoAsset = 'assets/brand/logo.svg';

/// Shows an action failure as a floating M3 SnackBar.
///
/// Soft error colors (not raw red inline text), dismissible, announced to
/// screen readers by the SnackBar itself. Field-level validation errors stay
/// inline in their forms; this is for action failures (join, load, send...).
void showHalaqatyError(BuildContext context, String message) {
  final scheme = Theme.of(context).colorScheme;
  final isRtl = Directionality.of(context) == TextDirection.rtl;
  ScaffoldMessenger.of(context)
    ..hideCurrentSnackBar()
    ..showSnackBar(
      SnackBar(
        key: const Key('halaqatyErrorSnackBar'),
        content: Text(
          message,
          style: TextStyle(color: scheme.onErrorContainer),
        ),
        backgroundColor: scheme.errorContainer,
        behavior: SnackBarBehavior.floating,
        duration: const Duration(seconds: 4),
        action: SnackBarAction(
          textColor: scheme.onErrorContainer,
          label: isRtl ? 'إغلاق' : 'Dismiss',
          // Remove immediately so a queued/replaced snackbar cannot win the
          // hide animation and make the dismiss tap appear ineffective.
          onPressed: ScaffoldMessenger.of(context).removeCurrentSnackBar,
        ),
      ),
    );
}

/// Shows the shared under-implementation notice (FR-032/FR-033).
/// Presentation-only: no navigation, controller calls, or state mutation.
void showHalaqatyUnderImplementationNotice(BuildContext context) {
  final isRtl = Directionality.of(context) == TextDirection.rtl;
  ScaffoldMessenger.of(context)
    ..hideCurrentSnackBar()
    ..showSnackBar(
      SnackBar(
        key: const Key('halaqatyUnderImplementationSnackBar'),
        content: Text(
          isRtl
              ? 'هذه الميزة قيد التنفيذ وغير متاحة حالياً.'
              : 'This feature is under implementation and is not available yet.',
        ),
        behavior: SnackBarBehavior.floating,
        duration: const Duration(seconds: 4),
        action: SnackBarAction(
          label: isRtl ? 'إغلاق' : 'Dismiss',
          onPressed: ScaffoldMessenger.of(context).removeCurrentSnackBar,
        ),
      ),
    );
}

/// Branded loading surface for waits over 300ms (FR-007), with a localized
/// semantics label instead of a bare spinner.
class HalaqatyLoading extends StatelessWidget {
  const HalaqatyLoading({super.key});

  @override
  Widget build(BuildContext context) {
    final isRtl = Directionality.of(context) == TextDirection.rtl;
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          const HalaqatyLogo(size: 72, monochrome: true),
          const SizedBox(height: 24),
          CircularProgressIndicator(
            semanticsLabel: isRtl ? 'جارٍ التحميل' : 'Loading',
          ),
        ],
      ),
    );
  }
}

/// The Halaqaty logo (Rub el Hizb).
class HalaqatyLogo extends StatelessWidget {
  const HalaqatyLogo({super.key, this.size = 64, this.monochrome = false});

  /// Logical size (width and height) of the logo.
  final double size;

  /// Whether to use the monochrome variant (for empty states/tinting).
  final bool monochrome;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: 'Halaqaty',
      image: true,
      child: SvgPicture.asset(
        monochrome ? 'assets/brand/logo_monochrome.svg' : halaqatyLogoAsset,
        width: size,
        height: size,
        fit: BoxFit.contain,
        // The monochrome asset is single-currentColor; tint it from the
        // scheme so it stays legible in both themes.
        colorFilter: monochrome
            ? ColorFilter.mode(
                Theme.of(context).colorScheme.onSurfaceVariant,
                BlendMode.srcIn,
              )
            : null,
      ),
    );
  }
}

/// Section title with optional trailing action, per DESIGN.md type scale.
class SectionHeader extends StatelessWidget {
  const SectionHeader({super.key, required this.title, this.action});

  /// Section title text (already localized by the caller).
  final String title;

  /// Optional trailing action (e.g. "See all").
  final Widget? action;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Row(
        children: [
          Expanded(
            child: Text(
              title,
              style: Theme.of(context).textTheme.titleLarge,
            ),
          ),
          if (action != null) action!,
        ],
      ),
    );
  }
}

/// Branded empty state: monochrome logo mark, title, and a hint.
class EmptyStateCard extends StatelessWidget {
  const EmptyStateCard({
    super.key,
    required this.title,
    required this.hint,
  });

  /// What is empty (e.g. "No circles yet").
  final String title;

  /// What to do next (e.g. "Discover public circles").
  final String hint;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            HalaqatyLogo(size: 72, monochrome: true),
            const SizedBox(height: 16),
            Text(
              title,
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 8),
            Text(
              hint,
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                    color: Theme.of(context).colorScheme.onSurfaceVariant,
                  ),
            ),
          ],
        ),
      ),
    );
  }
}
