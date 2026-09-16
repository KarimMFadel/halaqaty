import 'package:flutter/material.dart';
import 'package:flutter_svg/flutter_svg.dart';

/// Path of the primary brand logo asset.
const String halaqatyLogoAsset = 'assets/brand/logo.svg';

/// The Halaqaty logo (8-point khatam star with open book).
///
/// The asset is a replaceable placeholder; keep this path stable.
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
