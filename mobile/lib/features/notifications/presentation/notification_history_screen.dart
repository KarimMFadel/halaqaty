import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/features/notifications/application/notification_history_controller.dart';
import 'package:halaqaty_mobile/features/notifications/domain/notification_models.dart';
import 'package:halaqaty_mobile/features/notifications/presentation/notification_copy.dart';

/// Notification center: newest-first 30-day history with the exact server
/// unread count. Entry, pull-to-refresh, resume, and reconnect reconcile
/// through the controller's `load()` (realtime contract); a failing refresh
/// keeps loaded rows visible (controller-side rule).
class NotificationHistoryScreen extends ConsumerStatefulWidget {
  const NotificationHistoryScreen({super.key});

  @override
  ConsumerState<NotificationHistoryScreen> createState() =>
      _NotificationHistoryScreenState();
}

class _NotificationHistoryScreenState
    extends ConsumerState<NotificationHistoryScreen> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      unawaited(
        ref.read(notificationHistoryControllerProvider.notifier).load(),
      );
    });
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(notificationHistoryControllerProvider);
    final isRtl = Directionality.of(context) == TextDirection.rtl;

    // Empty-state/error copy is Arabic-first screen copy (item rows use
    // the type-keyed resolver below).
    final emptyTitle = isRtl ? 'لا توجد إشعارات بعد' : 'No notifications yet';
    final emptyHint = isRtl
        ? 'ستظهر الإشعارات هنا عند نشاط حلقاتك.'
        : 'Notifications will appear here as your circles act.';
    final retryLabel = isRtl ? 'إعادة المحاولة' : 'Retry';
    final errorHint = isRtl
        ? 'تعذّر تحميل الإشعارات؛ تحقق من الاتصال وحاول مجدداً.'
        : 'Could not load notifications; check your connection and retry.';

    final Widget body;
    if (state.items.isNotEmpty) {
      body = _historyList(state, isRtl);
    } else {
      switch (state.status) {
        case NotificationHistoryStatus.loading:
          body = const HalaqatyLoading(key: Key('notificationHistoryLoading'));
        case NotificationHistoryStatus.error:
          // Keep the empty-frame copy in the error frame but add the safe
          // retry affordance (mapped copy, no raw exception text).
          body = ListView(
            children: [
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 24),
                child: Column(
                  children: [
                    EmptyStateCard(
                      key: const Key('notificationHistoryRetry'),
                      title: emptyTitle,
                      hint: errorHint,
                    ),
                    TextButton(
                      onPressed: () => ref
                          .read(notificationHistoryControllerProvider.notifier)
                          .load(),
                      child: Text(retryLabel),
                    ),
                  ],
                ),
              ),
            ],
          );
        case NotificationHistoryStatus.ready:
        case NotificationHistoryStatus.unavailable:
          body = EmptyStateCard(
            key: const Key('notificationHistoryEmpty'),
            title: emptyTitle,
            hint: emptyHint,
          );
      }
    }

    return Scaffold(
      appBar: AppBar(
        title: Text(isRtl ? 'الإشعارات' : 'Notifications'),
      ),
      body: SafeArea(
        child: RefreshIndicator(
          onRefresh: () =>
              ref.read(notificationHistoryControllerProvider.notifier).load(),
          child: body,
        ),
      ),
    );
  }

  Widget _historyList(NotificationHistoryState state, bool isRtl) {
    final scheme = Theme.of(context).colorScheme;
    final loadingMore = state.status == NotificationHistoryStatus.loading;
    return ListView.builder(
      itemCount: state.items.length + (state.hasMore ? 1 : 0),
      itemBuilder: (context, index) {
        if (index >= state.items.length) {
          return Padding(
            padding: const EdgeInsets.symmetric(vertical: 12),
            child: Center(
              child: TextButton(
                key: const Key('notificationHistoryLoadMore'),
                onPressed: loadingMore
                    ? null
                    : () => ref
                        .read(notificationHistoryControllerProvider.notifier)
                        .loadMore(),
                child: Text(
                  loadingMore
                      ? (isRtl ? 'جارٍ التحميل…' : 'Loading…')
                      : (isRtl
                          ? 'تحميل المزيد من الإشعارات'
                          : 'Load more notifications'),
                ),
              ),
            ),
          );
        }
        final item = state.items[index];
        // Karim copy decision: Arabic-first by type (+source_ref.type);
        // the server title/body are the generic fallbacks only.
        final copy = resolveNotificationCopy(
          type: item.type,
          sourceRefType: item.source.type,
          fallbackTitle: item.title,
          fallbackBody: item.body,
          isArabic: isRtl,
        );
        final date = item.createdAt.toLocal();
        final dateLabel = '${date.day}/${date.month}/${date.year}';
        return ListTile(
          key: ValueKey('notificationItem-${item.id}'),
          leading: item.isRead
              ? const SizedBox.shrink()
              : Container(
                  width: 14,
                  alignment: Alignment.center,
                  child: Icon(
                    Icons.fiber_manual_record,
                    size: 10,
                    color: scheme.secondaryContainer,
                    semanticLabel: isRtl ? 'غير مقروء' : 'Unread',
                  ),
                ),
          title: Text(
            copy.title,
            style: Theme.of(context).textTheme.titleSmall,
          ),
          subtitle: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(copy.body),
              Text(
                dateLabel,
                textDirection: TextDirection.ltr,
                style: Theme.of(context).textTheme.labelSmall?.copyWith(
                      color: scheme.onSurfaceVariant,
                    ),
              ),
            ],
          ),
        );
      },
    );
  }
}
