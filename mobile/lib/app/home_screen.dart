import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_discovery_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_detail_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_join_screen.dart';
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
    final auth = ref.watch(authControllerProvider);
    final state = ref.watch(circleDiscoveryControllerProvider);
    final calendarState = ref.watch(calendarControllerProvider);
    final calendar = ref.read(calendarControllerProvider.notifier);
    final nextSession = _nextSession(calendarState.items);
    final isRtl = Directionality.of(context) == TextDirection.rtl;
    final scheme = Theme.of(context).colorScheme;
    final displayName = auth.user?.displayName?.trim();
    final avatarInitial = displayName == null || displayName.isEmpty
        ? (isRtl ? '؟' : '?')
        : String.fromCharCode(displayName.runes.first).toUpperCase();
    return Scaffold(
      appBar: AppBar(
        centerTitle: false,
        title: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            DecoratedBox(
              decoration: BoxDecoration(
                color: scheme.brightness == Brightness.dark
                    ? scheme.primaryContainer
                    : Color.alphaBlend(
                        scheme.primary.withValues(alpha: 0.1),
                        scheme.surface,
                      ),
                borderRadius: BorderRadius.circular(11),
              ),
              child: const SizedBox(
                width: 32,
                height: 32,
                child: Center(child: HalaqatyLogo(size: 20)),
              ),
            ),
            const SizedBox(width: 8),
            Text(isRtl ? 'حِلْقَتي' : 'Halaqaty'),
          ],
        ),
        actions: [
          IconButton(
            tooltip: isRtl ? 'التقويم' : 'Calendar',
            onPressed: () => context.push('/calendar'),
            icon: const Icon(Icons.calendar_month_outlined),
          ),
          IconButton(
            key: const Key('homeProfileAction'),
            tooltip: isRtl ? 'حسابي' : 'Profile',
            onPressed: () => context.go('/profile'),
            icon: CircleAvatar(
              radius: 16,
              backgroundColor: scheme.brightness == Brightness.dark
                  ? scheme.primaryContainer
                  : Color.alphaBlend(
                      scheme.primary.withValues(alpha: 0.1),
                      scheme.surface,
                    ),
              foregroundColor: scheme.brightness == Brightness.dark
                  ? scheme.onPrimaryContainer
                  : scheme.primary,
              child: Text(avatarInitial),
            ),
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
              if (state.myCirclesFailure != null)
                CircleLoadError(
                  failure: state.myCirclesFailure!,
                  onRetry: () => ref
                      .read(circleDiscoveryControllerProvider.notifier)
                      .loadMyCircles(),
                ),
              if (state.myCirclesFailure != null) const SizedBox(height: 16),
              Row(
                children: [
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          displayName?.isNotEmpty == true
                              ? (isRtl
                                  ? 'أهلاً، $displayName'
                                  : 'Hello, $displayName')
                              : (isRtl ? 'أهلاً بك' : 'Welcome'),
                          key: const Key('homeGreeting'),
                          style: Theme.of(context)
                              .textTheme
                              .headlineSmall
                              ?.copyWith(color: scheme.primary),
                        ),
                        const SizedBox(height: 4),
                        Text(
                          isRtl
                              ? 'لنواصل رحلة الحفظ اليوم'
                              : 'Let’s continue your memorization journey today',
                          style: Theme.of(context)
                              .textTheme
                              .bodyMedium
                              ?.copyWith(color: scheme.onSurfaceVariant),
                        ),
                      ],
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
                title: isRtl ? 'وصول سريع' : 'Quick access',
              ),
              Row(
                children: [
                  Expanded(
                    child: _QuickActionCard(
                      key: const Key('homeQuickDiscover'),
                      icon: Icons.search,
                      label: isRtl ? 'اكتشاف حلقة' : 'Discover a circle',
                      onTap: () => context.go('/circles'),
                    ),
                  ),
                  const SizedBox(width: 8),
                  Expanded(
                    child: _QuickActionCard(
                      key: const Key('homeQuickInvite'),
                      icon: Icons.link,
                      label: isRtl ? 'رابط الدعوة' : 'Invite link',
                      onTap: () => Navigator.of(context).push(
                        MaterialPageRoute<void>(
                          builder: (_) => const CircleJoinScreen(),
                        ),
                      ),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 24),
              SectionHeader(
                title: isRtl ? 'حلقاتي' : 'My circles',
                action: TextButton(
                  onPressed: () => context.go('/circles'),
                  child: Text(isRtl ? 'عرض الكل' : 'See all'),
                ),
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
              ...state.myCircles.indexed.map(
                (entry) => _CircleCard(
                  circle: entry.$2,
                  isLast: entry.$1 == state.myCircles.length - 1,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _QuickActionCard extends StatelessWidget {
  const _QuickActionCard({
    super.key,
    required this.icon,
    required this.label,
    required this.onTap,
  });

  final IconData icon;
  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Card(
      margin: EdgeInsets.zero,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(12),
        child: Padding(
          padding: const EdgeInsets.all(10),
          child: Row(
            children: [
              DecoratedBox(
                decoration: BoxDecoration(
                  color: scheme.brightness == Brightness.dark
                      ? scheme.primaryContainer
                      : Color.alphaBlend(
                          scheme.primary.withValues(alpha: 0.1),
                          scheme.surface,
                        ),
                  borderRadius: BorderRadius.circular(11),
                ),
                child: SizedBox(
                  width: 36,
                  height: 36,
                  child: Icon(
                    icon,
                    color: scheme.brightness == Brightness.dark
                        ? scheme.onPrimaryContainer
                        : scheme.primary,
                  ),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  label,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: Theme.of(context).textTheme.labelLarge,
                ),
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
    final isLight = scheme.brightness == Brightness.light;
    final heroText = isLight ? scheme.onPrimary : scheme.onSurface;
    final heroMuted = isLight ? scheme.onPrimary : scheme.onSurfaceVariant;
    final start = calendarTimeInZone(item.startsAt, timezone);
    final when = rtl
        ? '${start.day} ${_arabicMonths[start.month - 1]} · ${_clock(start)}'
        : '${_englishMonths[start.month - 1]} ${start.day} · ${_clock(start)}';
    return Card(
      key: const Key('homeNextSession'),
      margin: EdgeInsets.zero,
      color: isLight ? scheme.primary : scheme.surface,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(19),
        side: isLight
            ? BorderSide.none
            : BorderSide(color: scheme.outlineVariant),
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
                  ?.copyWith(color: heroMuted),
            ),
            const SizedBox(height: 4),
            Text(
              item.circleName,
              style: Theme.of(context)
                  .textTheme
                  .titleLarge
                  ?.copyWith(color: heroText),
            ),
            if (item.title.isNotEmpty) ...[
              const SizedBox(height: 2),
              Text(
                item.title,
                style: Theme.of(context)
                    .textTheme
                    .bodyMedium
                    ?.copyWith(color: heroMuted),
              ),
            ],
            const SizedBox(height: 4),
            Text(
              when,
              style: Theme.of(context)
                  .textTheme
                  .bodySmall
                  ?.copyWith(color: heroMuted),
            ),
            const SizedBox(height: 12),
            FilledButton(
              style: FilledButton.styleFrom(
                backgroundColor:
                    isLight ? scheme.secondaryContainer : scheme.primary,
                foregroundColor:
                    isLight ? scheme.onSecondaryContainer : scheme.onPrimary,
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
  const _CircleCard({required this.circle, required this.isLast});

  final CircleSummary circle;
  final bool isLast;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Semantics(
      button: true,
      label: circle.name,
      child: Container(
        decoration: BoxDecoration(
          border: isLast
              ? null
              : Border(
                  bottom: BorderSide(color: scheme.outlineVariant),
                ),
        ),
        child: Material(
          color: Colors.transparent,
          child: InkWell(
            borderRadius: BorderRadius.circular(12),
            onTap: () => Navigator.of(context).push(
              MaterialPageRoute<void>(
                builder: (_) => CircleDetailScreen(circleId: circle.id),
              ),
            ),
            child: Padding(
              padding: const EdgeInsets.symmetric(vertical: 8),
              child: ListTile(
                key: Key('homeCircle-${circle.id}'),
                contentPadding: const EdgeInsets.symmetric(horizontal: 8),
                leading: DecoratedBox(
                  decoration: BoxDecoration(
                    color: scheme.brightness == Brightness.dark
                        ? scheme.primaryContainer
                        : Color.alphaBlend(
                            scheme.primary.withValues(alpha: 0.1),
                            scheme.surface,
                          ),
                    borderRadius: BorderRadius.circular(11),
                  ),
                  child: SizedBox(
                    width: 36,
                    height: 36,
                    child: Center(
                      child: Text(
                        key: Key('homeCircleInitial-${circle.id}'),
                        circle.name.isEmpty
                            ? '•'
                            : String.fromCharCode(circle.name.runes.first),
                        style:
                            Theme.of(context).textTheme.titleMedium?.copyWith(
                                  color: scheme.brightness == Brightness.dark
                                      ? scheme.onPrimaryContainer
                                      : scheme.primary,
                                ),
                      ),
                    ),
                  ),
                ),
                title: CircleNameText(name: circle.name),
                subtitle:
                    circle.description == null || circle.description!.isEmpty
                        ? null
                        : Text(
                            circle.description!,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                          ),
                // Material mirrors this direction-aware icon once for RTL.
                trailing: const Icon(Icons.chevron_right),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
