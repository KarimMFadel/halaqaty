import 'package:flutter/material.dart';
import 'package:halaqaty_mobile/features/scheduling/application/calendar_controller.dart';
import 'package:halaqaty_mobile/features/scheduling/data/calendar_api_client.dart';

class CalendarOverlapWarnings extends StatelessWidget {
  const CalendarOverlapWarnings({
    required this.warnings,
    required this.rtl,
    required this.timezone,
    super.key,
  });

  final List<CalendarWarning> warnings;
  final bool rtl;
  final String timezone;

  @override
  Widget build(BuildContext context) {
    if (warnings.isEmpty) return const SizedBox.shrink();
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(children: [
              const Icon(Icons.warning_amber_rounded),
              const SizedBox(width: 8),
              Text(rtl ? 'تنبيه: تداخل في المواعيد' : 'Schedule overlap',
                  style: Theme.of(context).textTheme.titleSmall),
            ]),
            Padding(
              padding: const EdgeInsetsDirectional.only(start: 32),
              child: Text(rtl
                  ? 'للعلم فقط؛ يمكنك المشاركة في أي جلسة.'
                  : 'For your information; you may attend any session.'),
            ),
            for (final warning in warnings) ...[
              const SizedBox(height: 8),
              Text(rtl
                  ? '${warning.firstCircleName} و${warning.secondCircleName}'
                  : '${warning.firstCircleName} overlaps ${warning.secondCircleName}'),
              Text(_interval(context, warning)),
            ],
          ],
        ),
      ),
    );
  }

  String _interval(BuildContext context, CalendarWarning warning) {
    final start = calendarTimeInZone(warning.overlapStartsAt, timezone);
    final end = calendarTimeInZone(warning.overlapEndsAt, timezone);
    final date = MaterialLocalizations.of(context).formatMediumDate(start);
    return '$date · ${_time(start)}–${_time(end)}';
  }

  String _time(DateTime value) =>
      '${value.hour.toString().padLeft(2, '0')}:${value.minute.toString().padLeft(2, '0')}';
}

Future<bool> saveWithOverlapReconfirmation({
  required BuildContext context,
  required bool rtl,
  required String timezone,
  required Future<bool> Function(bool confirmed, List<String> warningIDs) save,
  required List<CalendarWarning> Function() currentWarnings,
}) async {
  var confirmed = false;
  var warningIDs = <String>[];
  while (true) {
    if (await save(confirmed, warningIDs)) return true;
    if (!context.mounted) return false;
    final warnings = currentWarnings();
    if (warnings.isEmpty) return false;
    final accepted = await showDialog<bool>(
      context: context,
      builder: (dialogContext) => AlertDialog(
        title: Text(rtl ? 'تعارض في المواعيد' : 'Schedule conflicts'),
        content: SingleChildScrollView(
          child: CalendarOverlapWarnings(
            warnings: warnings,
            rtl: rtl,
            timezone: timezone,
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(dialogContext, false),
            child: Text(rtl ? 'مراجعة لاحقاً' : 'Review later'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(dialogContext, true),
            child: Text(rtl ? 'حفظ رغم ذلك' : 'Save anyway'),
          ),
        ],
      ),
    );
    if (accepted != true) return false;
    confirmed = true;
    warningIDs = warnings.map((warning) => warning.warningId).toList();
  }
}
