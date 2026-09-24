import 'package:flutter/material.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';

/// Recoverable load state shared by Home and circle discovery.
class CircleLoadError extends StatelessWidget {
  const CircleLoadError({
    super.key,
    required this.failure,
    required this.onRetry,
  });

  final CircleJoinFailure failure;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final network = failure == CircleJoinFailure.network;
    return Card(
      key: const Key('circleLoadError'),
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          children: [
            Icon(Icons.cloud_off, color: Theme.of(context).colorScheme.error),
            const SizedBox(height: 12),
            Text(
              network
                  ? (rtl
                      ? 'لا يمكن الاتصال بالخادم الآن'
                      : 'We cannot reach the server right now')
                  : (rtl ? 'تعذّر تحميل الحلقات' : 'Could not load circles'),
              textAlign: TextAlign.center,
            ),
            if (network) ...[
              const SizedBox(height: 8),
              Text(
                rtl
                    ? 'تحقق من الاتصال ثم حاول مرة أخرى.'
                    : 'Check your connection, then try again.',
                textAlign: TextAlign.center,
              ),
            ],
            const SizedBox(height: 12),
            OutlinedButton.icon(
              key: const Key('circleLoadRetry'),
              onPressed: onRetry,
              icon: const Icon(Icons.refresh),
              label: Text(rtl ? 'إعادة المحاولة' : 'Retry'),
            ),
          ],
        ),
      ),
    );
  }
}
