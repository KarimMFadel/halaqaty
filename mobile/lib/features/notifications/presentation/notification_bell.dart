import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:halaqaty_mobile/features/notifications/application/notification_history_controller.dart';

/// Shell AppBar action opening the notification center.
///
/// M3 error badge (notification-count convention); the RTL tooltip copy is
/// Arabic-first. Hidden badge when the count is unknown (`null`, e.g. no
/// session): never shows a zero badge from a stale/local guess.
class NotificationBell extends ConsumerWidget {
  const NotificationBell({super.key});

  /// Exact-count badge caps at the two-character display convention.
  static String badgeLabel(int count) => count > 99 ? '99+' : '$count';

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(notificationHistoryControllerProvider);
    final isRtl = Directionality.of(context) == TextDirection.rtl;
    final count = state.unreadCount;
    return IconButton(
      key: const Key('homeNotificationsAction'),
      tooltip: isRtl ? 'الإشعارات' : 'Notifications',
      onPressed: () => context.push('/notifications'),
      icon: Badge(
        isLabelVisible: count != null && count > 0,
        label: count == null
            ? null
            : Text(badgeLabel(count),
                textDirection: TextDirection.ltr,
                semanticsLabel: isRtl
                    ? '$count إشعارات غير مقروءة'
                    : '$count unread notifications'),
        child: const Icon(Icons.notifications_none_outlined),
      ),
    );
  }
}
