import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/scheduling/application/calendar_controller.dart';
import 'package:halaqaty_mobile/features/scheduling/data/schedule_api_client.dart';
import 'package:halaqaty_mobile/features/scheduling/presentation/overlap_warning.dart';
import 'package:timezone/timezone.dart' as tz;

class OneOffScreen extends ConsumerStatefulWidget {
  const OneOffScreen({
    super.key,
    required this.circleId,
    required this.circleName,
    required this.initialTimezone,
    this.now,
  });

  final String circleId;
  final String circleName;
  final String initialTimezone;
  final DateTime Function()? now;

  @override
  ConsumerState<OneOffScreen> createState() => _OneOffScreenState();
}

class _OneOffScreenState extends ConsumerState<OneOffScreen> {
  final _formKey = GlobalKey<FormState>();
  late final TextEditingController _date;
  final _title = TextEditingController(text: 'Circle Session');
  final _start = TextEditingController(text: '18:00');
  final _end = TextEditingController(text: '19:00');
  final _duration = TextEditingController(text: '60');
  late final TextEditingController _timezone;

  @override
  void initState() {
    super.initState();
    final instant = (widget.now ?? DateTime.now)();
    final today = _localTime(instant, widget.initialTimezone) ??
        _localTime(instant, 'UTC')!;
    _date = TextEditingController(text: _dateValue(today));
    _timezone = TextEditingController(text: widget.initialTimezone);
  }

  @override
  void dispose() {
    _date.dispose();
    _title.dispose();
    _start.dispose();
    _end.dispose();
    _duration.dispose();
    _timezone.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final state = ref.watch(oneOffControllerProvider(widget.circleId));
    return Scaffold(
      appBar: AppBar(title: Text(rtl ? 'جلسة لمرة واحدة' : 'One-off session')),
      body: SafeArea(
        child: Form(
          key: _formKey,
          child: ListView(
            padding: const EdgeInsets.all(16),
            children: [
              Text(widget.circleName,
                  style: Theme.of(context).textTheme.titleLarge),
              const SizedBox(height: 16),
              _field(
                _date,
                rtl ? 'التاريخ (YYYY-MM-DD)' : 'Date (YYYY-MM-DD)',
                key: const Key('oneOffDate'),
                validator: (value) => _validateDate(value, rtl),
              ),
              _field(_title, rtl ? 'عنوان الجلسة' : 'Session title',
                  key: const Key('oneOffTitle')),
              _field(
                _start,
                rtl ? 'وقت البدء (HH:MM)' : 'Start time (HH:MM)',
                key: const Key('oneOffStart'),
                validator: (value) => _validateStartTime(value, rtl),
              ),
              _field(
                _end,
                rtl ? 'وقت الانتهاء (HH:MM)' : 'End time (HH:MM)',
                key: const Key('oneOffEnd'),
                validator: (value) {
                  final endMinutes = _minutes(value);
                  if (endMinutes == null) {
                    return rtl ? 'أدخل وقتاً صالحاً' : 'Enter a valid time';
                  }
                  final startMinutes = _minutes(_start.text);
                  if (startMinutes == null) {
                    return rtl
                        ? 'أدخل وقت بدء صالحاً أولاً'
                        : 'Enter a valid start time first';
                  }
                  final duration = int.tryParse(_duration.text);
                  if (duration == null || duration < 1 || duration > 44640) {
                    return rtl
                        ? 'أدخل مدة من دقيقة إلى ٣١ يوماً'
                        : 'Enter a duration from 1 minute to 31 days';
                  }
                  if ((startMinutes + duration) % (24 * 60) != endMinutes) {
                    return rtl
                        ? 'وقت الانتهاء لا يطابق المدة'
                        : 'End time must match the duration';
                  }
                  return null;
                },
              ),
              _field(
                _duration,
                rtl ? 'المدة بالدقائق' : 'Duration in minutes',
                key: const Key('oneOffDuration'),
                keyboardType: TextInputType.number,
                validator: (value) {
                  final duration = int.tryParse(value ?? '');
                  return duration != null && duration >= 1 && duration <= 44640
                      ? null
                      : (rtl
                          ? 'أدخل مدة من دقيقة إلى ٣١ يوماً'
                          : 'Enter a duration from 1 minute to 31 days');
                },
              ),
              _field(
                _timezone,
                rtl ? 'المنطقة الزمنية' : 'Time zone',
                key: const Key('oneOffTimezone'),
                validator: (value) => _validateTimezone(value, rtl),
              ),
              if (state.failure != null)
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 8),
                  child: Text(_failureCopy(state.failure!, rtl)),
                ),
              const SizedBox(height: 16),
              FilledButton.icon(
                key: const Key('oneOffCreate'),
                onPressed: state.isSaving ? null : () => _submit(rtl),
                icon: state.isSaving
                    ? const SizedBox.square(
                        dimension: 18,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.event_available_outlined),
                label: Text(rtl ? 'إنشاء الجلسة' : 'Create session'),
              ),
            ],
          ),
        ),
      ),
    );
  }

  TextFormField _field(
    TextEditingController controller,
    String label, {
    required Key key,
    String? Function(String?)? validator,
    TextInputType? keyboardType,
  }) =>
      TextFormField(
        key: key,
        controller: controller,
        validator: validator,
        keyboardType: keyboardType,
        decoration: InputDecoration(labelText: label),
      );

  Future<void> _submit(bool rtl) async {
    if (!_formKey.currentState!.validate()) return;
    final plan = SchedulePlanInput(
      mode: 'one_off',
      anchorLocalDate: _date.text.trim(),
      localStartTime: _start.text.trim(),
      localEndTime: _end.text.trim(),
      durationMinutes: int.parse(_duration.text),
      timezone: _timezone.text.trim(),
      title: _title.text.trim().isEmpty ? 'Circle Session' : _title.text.trim(),
    );
    final controller =
        ref.read(oneOffControllerProvider(widget.circleId).notifier);
    final created = await saveWithOverlapReconfirmation(
      context: context,
      rtl: rtl,
      timezone: plan.timezone,
      currentWarnings: () =>
          ref.read(oneOffControllerProvider(widget.circleId)).overlapWarnings,
      save: (confirmed, warningIDs) => controller.create(
        plan,
        confirmOverlaps: confirmed,
        confirmedWarningIDs: warningIDs,
      ),
    );
    if (created && mounted) {
      Navigator.of(context).pop(true);
    }
  }

  String? _validateDate(String? value, bool rtl) {
    if (!_validDate(value)) {
      return rtl ? 'أدخل تاريخاً صالحاً' : 'Enter a valid date';
    }
    final parsed = DateTime.parse(value!);
    final localNow = _planningNow();
    if (localNow == null) return null;
    final plannedDate = DateTime(parsed.year, parsed.month, parsed.day);
    final today = DateTime(localNow.year, localNow.month, localNow.day);
    return plannedDate.isBefore(today)
        ? (rtl
            ? 'تاريخ البدء يجب أن يكون اليوم أو بعده'
            : 'Start date must be today or later')
        : null;
  }

  String? _validateStartTime(String? value, bool rtl) {
    final startMinutes = _minutes(value);
    if (startMinutes == null) {
      return rtl ? 'أدخل وقتاً صالحاً' : 'Enter a valid time';
    }
    final localNow = _planningNow();
    if (localNow == null || !_validDate(_date.text)) return null;
    final selected = DateTime.parse(_date.text);
    if (selected.year != localNow.year ||
        selected.month != localNow.month ||
        selected.day != localNow.day) {
      return null;
    }
    final currentMinutes = localNow.hour * 60 + localNow.minute;
    final startsInPast = startMinutes < currentMinutes ||
        (startMinutes == currentMinutes &&
            (localNow.second > 0 || localNow.millisecond > 0));
    return startsInPast
        ? (rtl
            ? 'وقت البدء يجب أن يكون الآن أو بعده'
            : 'Start time must be now or later')
        : null;
  }

  String? _validateTimezone(String? value, bool rtl) =>
      _localTime((widget.now ?? DateTime.now)(), value?.trim() ?? '') == null
          ? (rtl
              ? 'أدخل منطقة زمنية صالحة من نوع IANA'
              : 'Enter a valid IANA time zone')
          : null;

  tz.TZDateTime? _planningNow() =>
      _localTime((widget.now ?? DateTime.now)(), _timezone.text.trim());

  tz.TZDateTime? _localTime(DateTime instant, String timezone) {
    try {
      return calendarTimeInZone(instant, timezone);
    } on tz.LocationNotFoundException {
      return null;
    }
  }

  bool _validDate(String? value) {
    if (value == null || !RegExp(r'^\d{4}-\d{2}-\d{2}$').hasMatch(value)) {
      return false;
    }
    final parsed = DateTime.tryParse(value);
    return parsed != null && _dateValue(parsed) == value;
  }

  int? _minutes(String? value) {
    final match =
        RegExp(r'^([01]\d|2[0-3]):([0-5]\d)$').firstMatch(value ?? '');
    if (match == null) return null;
    return int.parse(match[1]!) * 60 + int.parse(match[2]!);
  }

  String _dateValue(DateTime value) =>
      '${value.year.toString().padLeft(4, '0')}-'
      '${value.month.toString().padLeft(2, '0')}-'
      '${value.day.toString().padLeft(2, '0')}';

  String _failureCopy(CalendarFailure failure, bool rtl) => switch (failure) {
        CalendarFailure.network => rtl
            ? 'تعذر الاتصال. تحقق من اتصالك ثم أعد المحاولة.'
            : 'Could not connect. Check your connection and try again.',
        CalendarFailure.permission => rtl
            ? 'لم يعد لديك إذن إنشاء جلسة لهذه الحلقة.'
            : 'You no longer have permission to plan a session for this circle.',
        CalendarFailure.conflict => rtl
            ? 'تغيرت التعارضات. راجع التحذيرات وحدّث تأكيدك.'
            : 'The conflicts changed. Review the warnings and confirm again.',
        _ => rtl
            ? 'تعذر إنشاء الجلسة. تحقق من البيانات ثم أعد المحاولة.'
            : 'Could not create the session. Check the details and retry.',
      };
}
