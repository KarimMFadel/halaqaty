import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/features/scheduling/application/calendar_controller.dart';
import 'package:halaqaty_mobile/features/scheduling/data/calendar_api_client.dart';
import 'package:halaqaty_mobile/features/scheduling/presentation/overlap_warning.dart';

class CalendarScreen extends ConsumerStatefulWidget {
  const CalendarScreen({super.key});

  @override
  ConsumerState<CalendarScreen> createState() => _CalendarScreenState();
}

class _CalendarScreenState extends ConsumerState<CalendarScreen> {
  @override
  void initState() {
    super.initState();
    Future.microtask(
        () => ref.read(calendarControllerProvider.notifier).load());
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(calendarControllerProvider);
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final controller = ref.read(calendarControllerProvider.notifier);
    final month = state.month;
    final title = month == null ? '' : _monthLabel(month, rtl);

    return Scaffold(
      appBar: AppBar(
        title: Text(rtl ? 'التقويم' : 'Calendar'),
        actions: [
          IconButton(
            tooltip: rtl ? 'الشهر السابق' : 'Previous month',
            onPressed: state.status == CalendarStatus.loading
                ? null
                : controller.previousMonth,
            icon: const Icon(Icons.chevron_left),
          ),
          Center(
            child: Text(title, key: const Key('calendarMonthTitle')),
          ),
          IconButton(
            tooltip: rtl ? 'الشهر التالي' : 'Next month',
            onPressed: state.status == CalendarStatus.loading
                ? null
                : controller.nextMonth,
            icon: const Icon(Icons.chevron_right),
          ),
        ],
      ),
      body: SafeArea(child: _body(state, rtl, controller.timezone)),
    );
  }

  Widget _body(CalendarState state, bool rtl, String timezone) {
    if (state.status == CalendarStatus.loading && state.items.isEmpty) {
      return const HalaqatyLoading();
    }
    if (state.status == CalendarStatus.error && state.items.isEmpty) {
      return _LoadError(rtl: rtl, onRetry: _reload);
    }
    return ListView(
      key: const Key('calendarItems'),
      padding: const EdgeInsets.all(16),
      children: [
        CalendarOverlapWarnings(
          warnings: state.warnings,
          rtl: rtl,
          timezone: timezone,
        ),
        if (state.status == CalendarStatus.error)
          _OfflineBanner(rtl: rtl, onRetry: _reload),
        ...state.items.map((item) => _CalendarItemCard(
              item: item,
              rtl: rtl,
              timezone: timezone,
            )),
        if (state.items.isEmpty)
          EmptyStateCard(
            key: const Key('calendarEmpty'),
            title: rtl ? 'لا توجد جلسات هذا الشهر' : 'No sessions this month',
            hint: rtl ? 'ستظهر الجلسات هنا' : 'Your sessions will appear here',
          ),
      ],
    );
  }

  Future<void> _reload() =>
      ref.read(calendarControllerProvider.notifier).load();
}

class _CalendarItemCard extends StatelessWidget {
  const _CalendarItemCard({
    required this.item,
    required this.rtl,
    required this.timezone,
  });

  final CalendarItem item;
  final bool rtl;
  final String timezone;

  @override
  Widget build(BuildContext context) {
    final start = calendarTimeInZone(item.startsAt, timezone);
    final end = calendarTimeInZone(item.endsAt, timezone);
    final status = _stateLabel(item.state, rtl);
    final circle =
        rtl ? 'الحلقة: ${item.circleName}' : 'Circle: ${item.circleName}';
    return Card(
      key: Key('calendarItem-${item.occurrenceKey}'),
      margin: const EdgeInsets.only(bottom: 12),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 8),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Semantics(
              container: true,
              explicitChildNodes: true,
              label: circle,
              child: ListTile(
                leading: const Icon(Icons.groups_outlined),
                title: Text(item.circleName),
                subtitle: Text(
                  '${_dateLabel(start, rtl)} · ${_time(start)}–${_time(end)}',
                ),
                trailing: Text(status),
              ),
            ),
            Padding(
              padding: const EdgeInsetsDirectional.only(start: 72, end: 16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(item.title),
                  const SizedBox(height: 4),
                  Text(rtl
                      ? 'مخطط حسب ${item.planningTimezone}'
                      : 'Planned in ${item.planningTimezone}'),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  String _time(DateTime value) =>
      '${value.hour.toString().padLeft(2, '0')}:${value.minute.toString().padLeft(2, '0')}';

  String _dateLabel(DateTime value, bool rtl) => rtl
      ? '${value.day}/${value.month}/${value.year}'
      : '${value.month.toString().padLeft(2, '0')}/${value.day.toString().padLeft(2, '0')}/${value.year}';

  String _stateLabel(String value, bool rtl) => switch (value) {
        'active' => rtl ? 'مباشرة' : 'Live',
        'completed' => rtl ? 'مكتملة' : 'Completed',
        'cancelled' => rtl ? 'ملغاة' : 'Cancelled',
        _ => rtl ? 'مجدولة' : 'Scheduled',
      };
}

class _LoadError extends StatelessWidget {
  const _LoadError({required this.rtl, required this.onRetry});

  final bool rtl;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) => Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 24),
              child: Text(rtl
                  ? 'تعذر تحميل التقويم. تحقق من اتصالك وحاول مرة أخرى.'
                  : 'Could not load the calendar. Check your connection and try again.'),
            ),
            const SizedBox(height: 12),
            OutlinedButton.icon(
              onPressed: onRetry,
              icon: const Icon(Icons.refresh),
              label: Text(rtl ? 'حاول مرة أخرى' : 'Try again'),
            ),
          ],
        ),
      );
}

class _OfflineBanner extends StatelessWidget {
  const _OfflineBanner({required this.rtl, required this.onRetry});

  final bool rtl;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) => Card(
        child: ListTile(
          leading: const Icon(Icons.cloud_off_outlined),
          title: Text(
              rtl ? 'غير متصل — بيانات قديمة' : 'Offline — showing saved data'),
          trailing: IconButton(
            tooltip: rtl ? 'تحديث' : 'Refresh',
            onPressed: onRetry,
            icon: const Icon(Icons.refresh),
          ),
        ),
      );
}

String _monthLabel(DateTime month, bool rtl) {
  const english = [
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
    'December'
  ];
  const arabic = [
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
    'ديسمبر'
  ];
  return '${(rtl ? arabic : english)[month.month - 1]} ${month.year}';
}
