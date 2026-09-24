import 'package:flutter/material.dart';

/// Displays a circle name without allowing fixture or user-provided names to
/// destabilize list and detail layouts.
class CircleNameText extends StatelessWidget {
  const CircleNameText({
    super.key,
    required this.name,
    this.maxLines = 1,
    this.style,
  });

  final String name;
  final int maxLines;
  final TextStyle? style;

  @override
  Widget build(BuildContext context) {
    return Text(
      name,
      style: style,
      maxLines: maxLines,
      overflow: TextOverflow.ellipsis,
    );
  }
}
