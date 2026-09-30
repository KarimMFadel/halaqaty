import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_detail_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_name_text.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_load_error.dart';
import 'package:halaqaty_mobile/features/scheduling/application/calendar_controller.dart';
import 'package:halaqaty_mobile/features/scheduling/data/calendar_api_client.dart';

/// Home tab: branded overview of the user's circles and quick actions.
///
/// Sessions stay reachable from circle detail (their owning flow); this tab
/// only surfaces what existing APIs can derive client-side.
class HomeScreen extends ConsumerStatefulWidget {
  const HomeScreen({super.key});

  @override
  ConsumerState<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends ConsumerState<HomeScreen> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) async {
      if (mounted) {
        await ref
            .read(circleDiscoveryControllerProvider.notifier)
            .loadMyCircles();
        if (mounted) {
          await ref.read(calendarControllerProvider.notifier).load();
        }
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(circleDiscoveryControllerProvider);
    final calendarState = ref.watch(calendarControllerProvider);
    final calendar = ref.read(calendarControllerProvider.notifier);
    final nextSession = _nextSession(calendarState.items);
    final isRtl = Directionality.of(context) == TextDirection.rtl;
    final scheme = Theme.of(context).colorScheme;
    return Scaffold(
      appBar: AppBar(
        title: Text(isRtl ? 'الرئيسية' : 'Home'),
        actions: [
          IconButton(
            tooltip: isRtl ? 'التقويم' : 'Calendar',
            onPressed: () => context.push('/calendar'),
            icon: const Icon(Icons.calendar_month_outlined),
          ),
        ],
      ),
      body: SafeArea(
        child: RefreshIndicator(
          onRefresh: () => ref
              .read(circleDiscoveryControllerProvider.notifier)
              .loadMyCircles(),
          child: ListView(
            padding: const EdgeInsets.all(16),
            children: [
              // Recoverable failure keeps loaded circles visible under a
              // retryable error card instead of clearing safe context.
              if (state.failure != null)
                CircleLoadError(
                  failure: state.failure!,
                  onRetry: () => ref
                      .read(circleDiscoveryControllerProvider.notifier)
                      .loadMyCircles(),
                ),
              if (state.failure != null) const SizedBox(height: 16),
              Row(
                children: [
                  const HalaqatyLogo(size: 40),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Text(
                      isRtl ? 'مرحباً بك' : 'Welcome',
                      style: Theme.of(context)
                          .textTheme
                          .headlineSmall
                          ?.copyWith(color: scheme.primary),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 24),
              if (nextSession != null)
                _NextSessionCard(
                  item: nextSession,
                  timezone: calendar.timezone,
                ),
              if (nextSession != null) const SizedBox(height: 24),
              SectionHeader(
                title: isRtl ? 'حلقاتي' : 'My circles',
              ),
              if (state.isLoading && state.myCircles.isEmpty)
                const HalaqatyLoading(key: Key('homeLoading')),
              if (!state.isLoading && state.myCircles.isEmpty)
                EmptyStateCard(
                  key: const Key('homeNoCircles'),
                  title: isRtl ? 'لا توجد حلقات بعد' : 'No circles yet',
                  hint: isRtl
                      ? 'اكتشف الحلقات العامة أو انضم برمز دعوة'
                      : 'Discover public circles or join with an invite code',
                ),
              ...state.myCircles.map(
                (circle) => _CircleCard(circle: circle),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

CalendarItem? _nextSession(List<CalendarItem> items) {
  final now = DateTime.now().toUtc();
  final upcoming = items
      .where((item) => item.state == 'scheduled' && item.startsAt.isAfter(now))
      .toList()
    ..sort((a, b) => a.startsAt.compareTo(b.startsAt));
  return upcoming.isEmpty ? null : upcoming.first;
}

class _NextSessionCard extends StatelessWidget {
  const _NextSessionCard({required this.item, required this.timezone});

  final CalendarItem item;
  final String timezone;

  @override
  Widget build(BuildContext context) {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final scheme = Theme.of(context).colorScheme;
    final start = calendarTimeInZone(item.startsAt, timezone);
    final when = rtl
        ? '${start.day} ${_arabicMonths[start.month - 1]} · ${_clock(start)}'
        : '${_englishMonths[start.month - 1]} ${start.day} · ${_clock(start)}';
    return Card(
      key: const Key('homeNextSession'),
      margin: EdgeInsets.zero,
      color: scheme.surface,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(19),
        side: BorderSide(color: scheme.outlineVariant),
      ),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              rtl ? 'الجلسة القادمة' : 'Next session',
              style: Theme.of(context)
                  .textTheme
                  .labelLarge
                  ?.copyWith(color: scheme.onSurfaceVariant),
            ),
            const SizedBox(height: 4),
            Text(
              item.circleName,
              style: Theme.of(context)
                  .textTheme
                  .titleLarge
                  ?.copyWith(color: scheme.onSurface),
            ),
            if (item.title.isNotEmpty) ...[
              const SizedBox(height: 2),
              Text(
                item.title,
                style: Theme.of(context)
                    .textTheme
                    .bodyMedium
                    ?.copyWith(color: scheme.onSurfaceVariant),
              ),
            ],
            const SizedBox(height: 4),
            Text(
              when,
              style: Theme.of(context)
                  .textTheme
                  .bodySmall
                  ?.copyWith(color: scheme.onSurfaceVariant),
            ),
            const SizedBox(height: 12),
            FilledButton(
              style: FilledButton.styleFrom(
                backgroundColor: scheme.primary,
                foregroundColor: scheme.onPrimary,
              ),
              onPressed: () => context.push('/calendar'),
              child: Text(rtl ? 'عرض التقويم' : 'View calendar'),
            ),
          ],
        ),
      ),
    );
  }

  String _clock(DateTime date) =>
      '${date.hour.toString().padLeft(2, '0')}:${date.minute.toString().padLeft(2, '0')}';
}

const _englishMonths = [
  'January',
  'February',
  'March',
  'April',
  'May',
  'June',
  'July',
  'August',
  'September',
  'October',
  'November',
  'December',
];

const _arabicMonths = [
  'يناير',
  'فبراير',
  'مارس',
  'أبريل',
  'مايو',
  'يونيو',
  'يوليو',
  'أغسطس',
  'سبتمبر',
  'أكتوبر',
  'نوفمبر',
  'ديسمبر',
];

class _CircleCard extends StatelessWidget {
  const _CircleCard({required this.circle});

  final CircleSummary circle;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      button: true,
      label: circle.name,
      child: Card(
        margin: const EdgeInsets.only(bottom: 12),
        child: ListTile(
          key: Key('homeCircle-${circle.id}'),
          contentPadding:
              const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          leading: Icon(
            Icons.auto_stories,
            color: Theme.of(context).colorScheme.primary,
          ),
          title: CircleNameText(name: circle.name),
          subtitle: circle.description == null || circle.description!.isEmpty
              ? null
              : Text(
                  circle.description!,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
          // Material mirrors this direction-aware icon once for RTL; picking
          // chevron_left manually would double-mirror it (FR-010).
          trailing: const Icon(Icons.chevron_right),
          onTap: () => Navigator.of(context).push(
            MaterialPageRoute<void>(
              builder: (_) => CircleDetailScreen(circleId: circle.id),
            ),
          ),
        ),
      ),
    );
  }
}
