import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/core/validation.dart';
import 'package:halaqaty_mobile/features/scheduling/application/schedule_controller.dart';
import 'package:halaqaty_mobile/features/scheduling/data/schedule_api_client.dart';
import 'package:halaqaty_mobile/features/scheduling/presentation/overlap_warning.dart';

/// F-006 US1 schedule manager: list, create, change/stop series and edit or
/// cancel one unstarted occurrence. Teachers and supervisors mutate; other
/// members read. Archived circles and offline-stale lists are read-only.
class ScheduleEditorScreen extends ConsumerStatefulWidget {
  const ScheduleEditorScreen({
    super.key,
    required this.circleId,
    required this.circleName,
    required this.canManage,
    this.isArchived = false,
    this.initialTimezone = 'UTC',
  });

  final String circleId;
  final String circleName;
  final bool canManage;
  final bool isArchived;

  /// The viewer's stored profile timezone, offered as the planning-zone
  /// default. The plan keeps its own IANA zone either way (FR-006).
  final String initialTimezone;

  @override
  ConsumerState<ScheduleEditorScreen> createState() =>
      _ScheduleEditorScreenState();
}

class _ScheduleEditorScreenState extends ConsumerState<ScheduleEditorScreen> {
  /// While a form/occurrence sheet is open it owns the save-error copy, so
  /// the list-level banner waits for the sheet to close.
  bool _sheetOpen = false;

  ScheduleScope get _scope =>
      (circleId: widget.circleId, canManage: widget.canManage);

  @override
  void initState() {
    super.initState();
    Future.microtask(
      () => ref.read(scheduleControllerProvider(_scope).notifier).load(),
    );
  }

  bool get _canMutate => widget.canManage && !widget.isArchived;

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(scheduleControllerProvider(_scope));
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final degraded =
        state.status == ScheduleListStatus.error && state.schedules.isNotEmpty;

    return Scaffold(
      appBar: AppBar(
        title: Text(rtl ? _Labels.titleAr : _Labels.titleEn),
      ),
      body: SafeArea(
        child: _buildBody(state, rtl, degraded),
      ),
    );
  }

  Widget _buildBody(ScheduleEditorState state, bool rtl, bool degraded) {
    if (state.status == ScheduleListStatus.loading) {
      return const HalaqatyLoading();
    }
    if (state.status == ScheduleListStatus.error && state.schedules.isEmpty) {
      return _LoadFailure(failure: state.failure, rtl: rtl, onRetry: _reload);
    }
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        if (degraded) ...[
          _StaleBanner(rtl: rtl, onRetry: _reload),
          const SizedBox(height: 12),
        ],
        if (state.saveSucceeded)
          Padding(
            padding: const EdgeInsets.only(bottom: 12),
            child: _SaveSuccessBanner(
              message: rtl ? _Labels.saveSuccessAr : _Labels.saveSuccessEn,
            ),
          ),
        // Mutations without an open sheet (stop, occurrence cancel) still
        // surface a safe save error on the list itself.
        if (state.saveError != null && !state.isSaving && !_sheetOpen)
          Padding(
            padding: const EdgeInsets.only(bottom: 12),
            child: _FormErrorText(
              message: _saveErrorCopy(state.saveError!, rtl),
            ),
          ),
        if (state.schedules.isEmpty)
          EmptyStateCard(
            key: const Key('schedulesEmpty'),
            title: rtl ? _Labels.emptyTitleAr : _Labels.emptyTitleEn,
            hint: _canMutate
                ? (rtl
                    ? _Labels.emptyManagerHintAr
                    : _Labels.emptyManagerHintEn)
                : (rtl ? _Labels.emptyViewerHintAr : _Labels.emptyViewerHintEn),
          )
        else
          ...state.schedules.map(
            (entry) => _ScheduleCard(
              entry: entry,
              circleName: widget.circleName,
              rtl: rtl,
              canMutate: _canMutate && !degraded,
              onEdit: () => _openPlanForm(existing: entry),
              onStop: () => _confirmStop(entry),
              onEditOccurrence: () => _openOccurrenceSheet(entry),
            ),
          ),
        if (_canMutate && !degraded) ...[
          const SizedBox(height: 16),
          FilledButton.icon(
            key: const Key('scheduleAddButton'),
            onPressed: () => _openPlanForm(),
            icon: const Icon(Icons.add),
            label: Text(rtl ? _Labels.addAr : _Labels.addEn),
          ),
        ],
      ],
    );
  }

  Future<void> _reload() =>
      ref.read(scheduleControllerProvider(_scope).notifier).load();

  Future<void> _openPlanForm({CircleScheduleEntry? existing}) {
    setState(() => _sheetOpen = true);
    return showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      builder: (_) => Padding(
        padding: EdgeInsets.only(
          bottom: MediaQuery.viewInsetsOf(context).bottom,
        ),
        child: SingleChildScrollView(
          child: _SchedulePlanForm(
            scope: _scope,
            existing: existing,
            initialTimezone: widget.initialTimezone,
          ),
        ),
      ),
    ).whenComplete(() {
      if (mounted) setState(() => _sheetOpen = false);
    });
  }

  Future<void> _confirmStop(CircleScheduleEntry entry) async {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(rtl ? _Labels.stopTitleAr : _Labels.stopTitleEn),
        content: Text(rtl ? _Labels.stopBodyAr : _Labels.stopBodyEn),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(rtl ? _Labels.cancelAr : _Labels.cancelEn),
          ),
          FilledButton(
            key: const Key('scheduleStopConfirm'),
            onPressed: () => Navigator.of(context).pop(true),
            child: Text(rtl ? _Labels.stopConfirmAr : _Labels.stopConfirmEn),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    await ref
        .read(scheduleControllerProvider(_scope).notifier)
        .stopSeries(schedule: entry);
  }

  Future<void> _openOccurrenceSheet(CircleScheduleEntry entry) {
    setState(() => _sheetOpen = true);
    return showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      builder: (_) => Padding(
        padding: EdgeInsets.only(
          bottom: MediaQuery.viewInsetsOf(context).bottom,
        ),
        child: SingleChildScrollView(
          child: _OccurrenceEditSheet(scope: _scope, entry: entry),
        ),
      ),
    ).whenComplete(() {
      if (mounted) setState(() => _sheetOpen = false);
    });
  }
}

class _ScheduleCard extends StatelessWidget {
  const _ScheduleCard({
    required this.entry,
    required this.circleName,
    required this.rtl,
    required this.canMutate,
    required this.onEdit,
    required this.onStop,
    required this.onEditOccurrence,
  });

  final CircleScheduleEntry entry;
  final String circleName;
  final bool rtl;
  final bool canMutate;
  final VoidCallback onEdit;
  final VoidCallback onStop;
  final VoidCallback onEditOccurrence;

  @override
  Widget build(BuildContext context) {
    final plan = entry.plan;
    return Card(
      key: Key('scheduleCard-${entry.id}'),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 8),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            ListTile(
              // Circle identity pairs the name text with an icon cue, never
              // color alone (FR-005).
              leading: const Icon(Icons.event_repeat),
              title: Text(
                plan.title ??
                    (rtl ? _Labels.defaultTitleAr : _Labels.defaultTitleEn),
              ),
              subtitle: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(circleName),
                  Text(
                    '${plan.localStartTime}–${plan.localEndTime} · '
                    '${plan.timezone}',
                  ),
                ],
              ),
              isThreeLine: true,
            ),
            if (entry.stoppedFromLocalDate != null)
              Padding(
                padding: const EdgeInsetsDirectional.only(start: 16, bottom: 4),
                child: Text(
                  rtl
                      ? '${_Labels.stoppedAr}${entry.stoppedFromLocalDate}'
                      : '${_Labels.stoppedEn}${entry.stoppedFromLocalDate}',
                  style: Theme.of(context).textTheme.bodySmall,
                ),
              ),
            if (canMutate)
              OverflowBar(
                alignment: MainAxisAlignment.end,
                children: [
                  IconButton(
                    key: Key('scheduleOccurrence-${entry.id}'),
                    icon: const Icon(Icons.edit_calendar_outlined),
                    tooltip: rtl
                        ? _Labels.occurrenceTooltipAr
                        : _Labels.occurrenceTooltipEn,
                    onPressed: onEditOccurrence,
                  ),
                  IconButton(
                    key: Key('scheduleEdit-${entry.id}'),
                    icon: const Icon(Icons.edit_outlined),
                    tooltip:
                        rtl ? _Labels.editTooltipAr : _Labels.editTooltipEn,
                    onPressed: onEdit,
                  ),
                  IconButton(
                    key: Key('scheduleStop-${entry.id}'),
                    icon: const Icon(Icons.stop_circle_outlined),
                    tooltip:
                        rtl ? _Labels.stopTooltipAr : _Labels.stopTooltipEn,
                    onPressed: onStop,
                  ),
                ],
              ),
          ],
        ),
      ),
    );
  }
}

class _LoadFailure extends StatelessWidget {
  const _LoadFailure({
    required this.failure,
    required this.rtl,
    required this.onRetry,
  });

  final ScheduleFailure? failure;
  final bool rtl;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final permission = failure == ScheduleFailure.permission;
    final message = switch (failure) {
      ScheduleFailure.permission =>
        rtl ? _Labels.permissionAr : _Labels.permissionEn,
      ScheduleFailure.gone => rtl ? _Labels.goneAr : _Labels.goneEn,
      _ => rtl ? _Labels.loadErrorAr : _Labels.loadErrorEn,
    };
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              permission ? Icons.lock_outline : Icons.cloud_off_outlined,
              size: 48,
              color: Theme.of(context).colorScheme.onSurfaceVariant,
            ),
            const SizedBox(height: 16),
            Text(message, textAlign: TextAlign.center),
            if (!permission) ...[
              const SizedBox(height: 16),
              OutlinedButton.icon(
                key: const Key('scheduleRetryButton'),
                onPressed: onRetry,
                icon: const Icon(Icons.refresh),
                label: Text(rtl ? _Labels.retryAr : _Labels.retryEn),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _StaleBanner extends StatelessWidget {
  const _StaleBanner({required this.rtl, required this.onRetry});

  final bool rtl;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Container(
      key: const Key('scheduleStaleBanner'),
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      decoration: BoxDecoration(
        color: scheme.secondaryContainer,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        children: [
          Icon(Icons.cloud_off_outlined, color: scheme.onSecondaryContainer),
          const SizedBox(width: 12),
          Expanded(
            child: Text(
              rtl ? _Labels.staleAr : _Labels.staleEn,
              style: TextStyle(color: scheme.onSecondaryContainer),
            ),
          ),
          TextButton(
            key: const Key('scheduleRetryButton'),
            onPressed: onRetry,
            child: Text(rtl ? _Labels.retryAr : _Labels.retryEn),
          ),
        ],
      ),
    );
  }
}

class _SaveSuccessBanner extends StatelessWidget {
  const _SaveSuccessBanner({required this.message});

  final String message;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Semantics(
      container: true,
      liveRegion: true,
      label: message,
      child: Container(
        key: const Key('scheduleSaveSuccess'),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        decoration: BoxDecoration(
          color: scheme.secondaryContainer,
          borderRadius: BorderRadius.circular(12),
        ),
        child: Row(
          children: [
            Icon(Icons.check_circle, color: scheme.onSecondaryContainer),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                message,
                style: TextStyle(color: scheme.onSecondaryContainer),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Create/change form for one plan. The anchor auto-advances to the first
/// selected weekday on/after tomorrow so the contract rule "anchor weekday
/// must be selected" holds without extra taps; the validator still guards a
/// manually picked anchor.
class _SchedulePlanForm extends ConsumerStatefulWidget {
  const _SchedulePlanForm({
    required this.scope,
    required this.initialTimezone,
    this.existing,
  });

  final ScheduleScope scope;
  final String initialTimezone;
  final CircleScheduleEntry? existing;

  @override
  ConsumerState<_SchedulePlanForm> createState() => _SchedulePlanFormState();
}

class _SchedulePlanFormState extends ConsumerState<_SchedulePlanForm> {
  final _titleController = TextEditingController();
  final _durationController = TextEditingController(text: '60');
  final _intervalCountController = TextEditingController(text: '1');
  late final TextEditingController _timezoneController;

  String _mode = ScheduleModes.weekdayPattern;
  int _weekCadence = 1;
  final Set<int> _weekdays = {};
  String _intervalUnit = 'day';
  late DateTime _anchorDate;
  TimeOfDay _startTime = const TimeOfDay(hour: 18, minute: 0);
  TimeOfDay _endTime = const TimeOfDay(hour: 19, minute: 0);
  DateTime? _endDate;
  final List<String> _selectedDates = [];
  String? _formError;

  bool get _isEdit => widget.existing != null;

  @override
  void initState() {
    super.initState();
    final plan = widget.existing?.plan;
    _timezoneController =
        TextEditingController(text: plan?.timezone ?? widget.initialTimezone);
    _anchorDate = _tomorrow();
    if (plan != null) {
      // An edit retains the stored anchor (interval/selected-dates modes);
      // weekday mode still auto-advances in _syncAnchorToWeekdays below.
      _anchorDate = DateTime.tryParse(plan.anchorLocalDate) ?? _anchorDate;
      _mode = plan.mode == ScheduleModes.weekdayPattern ||
              plan.mode == ScheduleModes.interval ||
              plan.mode == ScheduleModes.selectedDates
          ? plan.mode
          : ScheduleModes.weekdayPattern;
      _titleController.text = plan.title ?? '';
      _durationController.text = '${plan.durationMinutes}';
      _weekCadence = plan.weekCadence ?? 1;
      _weekdays.addAll(plan.weekdays ?? const []);
      _intervalUnit = plan.intervalUnit ?? 'day';
      if (plan.intervalCount != null) {
        _intervalCountController.text = '${plan.intervalCount}';
      }
      _selectedDates.addAll(plan.selectedDates ?? const []);
      _startTime = _parseClock(plan.localStartTime) ?? _startTime;
      _endTime = _parseClock(plan.localEndTime) ?? _endTime;
      _endDate = plan.endLocalDate == null
          ? null
          : DateTime.tryParse(plan.endLocalDate!);
    }
    _syncAnchorToWeekdays();
  }

  @override
  void dispose() {
    _titleController.dispose();
    _durationController.dispose();
    _intervalCountController.dispose();
    _timezoneController.dispose();
    super.dispose();
  }

  static DateTime _tomorrow() {
    final now = DateTime.now();
    return DateTime(now.year, now.month, now.day + 1);
  }

  static TimeOfDay? _parseClock(String value) {
    final parts = value.split(':');
    if (parts.length != 2) return null;
    final hour = int.tryParse(parts[0]);
    final minute = int.tryParse(parts[1]);
    if (hour == null || minute == null) return null;
    return TimeOfDay(hour: hour, minute: minute);
  }

  static String _formatDate(DateTime date) =>
      '${date.year.toString().padLeft(4, '0')}-'
      '${date.month.toString().padLeft(2, '0')}-'
      '${date.day.toString().padLeft(2, '0')}';

  static String _formatClock(TimeOfDay time) =>
      '${time.hour.toString().padLeft(2, '0')}:'
      '${time.minute.toString().padLeft(2, '0')}';

  void _syncAnchorToWeekdays({bool selectionChanged = false}) {
    if (_isEdit && !selectionChanged) return;
    if (_mode != ScheduleModes.weekdayPattern || _weekdays.isEmpty) return;
    var candidate = _tomorrow();
    while (!_weekdays.contains(candidate.weekday % DateTime.daysPerWeek)) {
      candidate = candidate.add(const Duration(days: 1));
    }
    _anchorDate = candidate;
  }

  @override
  Widget build(BuildContext context) {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final saveError = ref.watch(
      scheduleControllerProvider(widget.scope).select((s) => s.saveError),
    );
    final isSaving = ref.watch(
      scheduleControllerProvider(widget.scope).select((s) => s.isSaving),
    );

    return Padding(
      key: const Key('scheduleFormSheet'),
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            _isEdit
                ? (rtl ? _Labels.editTitleAr : _Labels.editTitleEn)
                : (rtl ? _Labels.createTitleAr : _Labels.createTitleEn),
            style: Theme.of(context).textTheme.titleLarge,
          ),
          const SizedBox(height: 16),
          TextFormField(
            key: const Key('scheduleTitleField'),
            controller: _titleController,
            textDirection: rtl ? TextDirection.rtl : TextDirection.ltr,
            decoration: InputDecoration(
              labelText: rtl ? _Labels.titleFieldAr : _Labels.titleFieldEn,
              hintText: rtl ? _Labels.optionalAr : _Labels.optionalEn,
            ),
          ),
          const SizedBox(height: 16),
          SegmentedButton<String>(
            key: const Key('scheduleModeSelector'),
            segments: [
              ButtonSegment(
                value: ScheduleModes.weekdayPattern,
                label:
                    Text(rtl ? _Labels.modeWeekdaysAr : _Labels.modeWeekdaysEn),
              ),
              ButtonSegment(
                value: ScheduleModes.interval,
                label:
                    Text(rtl ? _Labels.modeIntervalAr : _Labels.modeIntervalEn),
              ),
              ButtonSegment(
                value: ScheduleModes.selectedDates,
                label: Text(rtl ? _Labels.modeDatesAr : _Labels.modeDatesEn),
              ),
            ],
            selected: {_mode},
            onSelectionChanged: (selection) => setState(() {
              _mode = selection.first;
              _syncAnchorToWeekdays(selectionChanged: true);
            }),
          ),
          const SizedBox(height: 16),
          if (_mode == ScheduleModes.weekdayPattern) ..._weekdaySection(rtl),
          if (_mode == ScheduleModes.interval) ..._intervalSection(rtl),
          if (_mode == ScheduleModes.selectedDates) ..._datesSection(rtl),
          if (_mode != ScheduleModes.selectedDates)
            _dateButton(
              key: const Key('scheduleAnchorDateButton'),
              label: rtl ? _Labels.anchorDateAr : _Labels.anchorDateEn,
              date: _anchorDate,
              onPicked: (date) => setState(() => _anchorDate = date),
            ),
          const SizedBox(height: 16),
          Row(
            children: [
              Expanded(
                child: _timeButton(
                  key: const Key('scheduleStartTimeButton'),
                  label: rtl ? _Labels.startTimeAr : _Labels.startTimeEn,
                  time: _startTime,
                  onPicked: (time) => setState(() {
                    _startTime = time;
                    _syncDurationToClocks();
                  }),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: _timeButton(
                  key: const Key('scheduleEndTimeButton'),
                  label: rtl ? _Labels.endTimeAr : _Labels.endTimeEn,
                  time: _endTime,
                  onPicked: (time) => setState(() {
                    _endTime = time;
                    _syncDurationToClocks();
                  }),
                ),
              ),
            ],
          ),
          const SizedBox(height: 16),
          TextFormField(
            key: const Key('scheduleDurationField'),
            controller: _durationController,
            keyboardType: TextInputType.number,
            inputFormatters: [FilteringTextInputFormatter.digitsOnly],
            decoration: InputDecoration(
              labelText: rtl ? _Labels.durationAr : _Labels.durationEn,
            ),
          ),
          const SizedBox(height: 16),
          TextFormField(
            key: const Key('scheduleTimezoneField'),
            controller: _timezoneController,
            decoration: InputDecoration(
              labelText: rtl ? _Labels.timezoneAr : _Labels.timezoneEn,
              hintText: 'Africa/Cairo',
            ),
          ),
          const SizedBox(height: 16),
          _dateButton(
            key: const Key('scheduleEndDateButton'),
            label: _endDate == null
                ? (rtl ? _Labels.endDateNoneAr : _Labels.endDateNoneEn)
                : (rtl ? _Labels.endDateAr : _Labels.endDateEn),
            date: _endDate,
            onPicked: (date) => setState(() => _endDate = date),
            onClear:
                _endDate == null ? null : () => setState(() => _endDate = null),
          ),
          if (_formError != null) ...[
            const SizedBox(height: 12),
            _FormErrorText(message: _formError!),
          ],
          if (saveError != null) ...[
            const SizedBox(height: 12),
            _FormErrorText(message: _saveErrorCopy(saveError, rtl)),
          ],
          const SizedBox(height: 16),
          FilledButton(
            key: const Key('scheduleSubmitButton'),
            onPressed: isSaving ? null : _submit,
            child: isSaving
                ? const SizedBox(
                    height: 20,
                    width: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : Text(rtl ? _Labels.saveAr : _Labels.saveEn),
          ),
        ],
      ),
    );
  }

  List<Widget> _weekdaySection(bool rtl) => [
        SegmentedButton<int>(
          key: const Key('scheduleCadenceSelector'),
          segments: [
            ButtonSegment(
              value: 1,
              label:
                  Text(rtl ? _Labels.cadenceWeeklyAr : _Labels.cadenceWeeklyEn),
            ),
            ButtonSegment(
              value: 2,
              label: Text(
                rtl ? _Labels.cadenceBiweeklyAr : _Labels.cadenceBiweeklyEn,
              ),
            ),
          ],
          selected: {_weekCadence},
          onSelectionChanged: (selection) =>
              setState(() => _weekCadence = selection.first),
        ),
        const SizedBox(height: 12),
        Wrap(
          spacing: 8,
          runSpacing: 4,
          children: [
            // Postgres DOW numbering: 0 is Sunday, 1 is Monday … 6 Saturday.
            for (final dow in const [1, 2, 3, 4, 5, 6, 0])
              FilterChip(
                key: Key('scheduleWeekday-$dow'),
                label: Text(_weekdayLabel(dow, rtl)),
                selected: _weekdays.contains(dow),
                onSelected: (selected) => setState(() {
                  if (selected) {
                    _weekdays.add(dow);
                  } else {
                    _weekdays.remove(dow);
                  }
                  _syncAnchorToWeekdays(selectionChanged: true);
                }),
              ),
          ],
        ),
        const SizedBox(height: 16),
      ];

  List<Widget> _intervalSection(bool rtl) => [
        Row(
          children: [
            Expanded(
              child: TextFormField(
                key: const Key('scheduleIntervalCountField'),
                controller: _intervalCountController,
                keyboardType: TextInputType.number,
                inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                decoration: InputDecoration(
                  labelText:
                      rtl ? _Labels.intervalCountAr : _Labels.intervalCountEn,
                ),
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: SegmentedButton<String>(
                key: const Key('scheduleIntervalUnitSelector'),
                segments: [
                  ButtonSegment(
                    value: 'day',
                    label: Text(rtl ? _Labels.unitDaysAr : _Labels.unitDaysEn),
                  ),
                  ButtonSegment(
                    value: 'week',
                    label:
                        Text(rtl ? _Labels.unitWeeksAr : _Labels.unitWeeksEn),
                  ),
                ],
                selected: {_intervalUnit},
                onSelectionChanged: (selection) =>
                    setState(() => _intervalUnit = selection.first),
              ),
            ),
          ],
        ),
        const SizedBox(height: 16),
      ];

  List<Widget> _datesSection(bool rtl) => [
        OutlinedButton.icon(
          key: const Key('scheduleAddDateButton'),
          onPressed: _addSelectedDate,
          icon: const Icon(Icons.event_available_outlined),
          label: Text(rtl ? _Labels.addDateAr : _Labels.addDateEn),
        ),
        if (_selectedDates.isNotEmpty)
          Wrap(
            spacing: 8,
            children: [
              for (final date in _selectedDates)
                InputChip(
                  label: Text(date),
                  onDeleted: () => setState(() => _selectedDates.remove(date)),
                ),
            ],
          ),
        const SizedBox(height: 16),
      ];

  Widget _dateButton({
    required Key key,
    required String label,
    required DateTime? date,
    required ValueChanged<DateTime> onPicked,
    VoidCallback? onClear,
  }) {
    return Row(
      children: [
        Expanded(
          child: OutlinedButton.icon(
            key: key,
            onPressed: () => _pickDate(initial: date ?? _tomorrow())
                .then((picked) => picked == null ? null : onPicked(picked)),
            icon: const Icon(Icons.calendar_today_outlined),
            label: Text(
              date == null ? label : '$label: ${_formatDate(date)}',
            ),
          ),
        ),
        if (onClear != null)
          IconButton(
            onPressed: onClear,
            icon: const Icon(Icons.clear),
          ),
      ],
    );
  }

  Widget _timeButton({
    required Key key,
    required String label,
    required TimeOfDay time,
    required ValueChanged<TimeOfDay> onPicked,
  }) {
    return OutlinedButton.icon(
      key: key,
      onPressed: () async {
        final picked =
            await showTimePicker(context: context, initialTime: time);
        if (picked != null) onPicked(picked);
      },
      icon: const Icon(Icons.schedule_outlined),
      label: Text('$label: ${_formatClock(time)}'),
    );
  }

  Future<DateTime?> _pickDate({required DateTime initial}) {
    final today = DateTime.now();
    return showDatePicker(
      context: context,
      // Past planned dates are rejected; the picker never offers them.
      firstDate: DateTime(today.year, today.month, today.day),
      lastDate: DateTime(9999, 12, 31),
      initialDate:
          initial.isBefore(DateTime(today.year, today.month, today.day))
              ? DateTime(today.year, today.month, today.day)
              : initial,
    );
  }

  Future<void> _addSelectedDate() async {
    final picked = await _pickDate(initial: _tomorrow());
    if (picked == null) return;
    final formatted = _formatDate(picked);
    setState(() {
      // Duplicate selected local dates produce one occurrence (FR-001).
      if (!_selectedDates.contains(formatted)) {
        _selectedDates.add(formatted);
        _selectedDates.sort();
      }
    });
  }

  void _syncDurationToClocks() {
    final minutes =
        (_clockMinutes(_endTime) - _clockMinutes(_startTime)) % 1440;
    _durationController.text = '${minutes == 0 ? 1440 : minutes}';
  }

  static int _clockMinutes(TimeOfDay time) => time.hour * 60 + time.minute;

  String? _validate(bool rtl) {
    if (!isValidIanaTimezone(_timezoneController.text)) {
      return rtl ? _Labels.timezoneInvalidAr : _Labels.timezoneInvalidEn;
    }
    final duration = int.tryParse(_durationController.text.trim());
    if (duration == null || duration < 1 || duration > 44640) {
      return rtl ? _Labels.durationInvalidAr : _Labels.durationInvalidEn;
    }
    final clockDelta =
        (_clockMinutes(_endTime) - _clockMinutes(_startTime)) % 1440;
    if (clockDelta != duration % 1440) {
      return rtl ? _Labels.durationMismatchAr : _Labels.durationMismatchEn;
    }
    final anchor = _mode == ScheduleModes.selectedDates
        ? (_selectedDates.isEmpty ? null : DateTime.parse(_selectedDates.first))
        : _anchorDate;
    if (_endDate != null && anchor != null && _endDate!.isBefore(anchor)) {
      return rtl ? _Labels.endDateInvalidAr : _Labels.endDateInvalidEn;
    }
    return _validateMode(rtl, anchor);
  }

  String? _validateMode(bool rtl, DateTime? anchor) {
    switch (_mode) {
      case ScheduleModes.weekdayPattern:
        if (_weekdays.isEmpty) {
          return rtl ? _Labels.weekdayRequiredAr : _Labels.weekdayRequiredEn;
        }
        if (anchor != null &&
            !_weekdays.contains(anchor.weekday % DateTime.daysPerWeek)) {
          return rtl ? _Labels.anchorWeekdayAr : _Labels.anchorWeekdayEn;
        }
        return null;
      case ScheduleModes.interval:
        final count = int.tryParse(_intervalCountController.text.trim());
        if (count == null || count < 1) {
          return rtl ? _Labels.intervalInvalidAr : _Labels.intervalInvalidEn;
        }
        return null;
      default:
        if (_selectedDates.isEmpty) {
          return rtl ? _Labels.datesRequiredAr : _Labels.datesRequiredEn;
        }
        return null;
    }
  }

  SchedulePlanInput _buildPlan() {
    final title = _titleController.text.trim();
    return SchedulePlanInput(
      mode: _mode,
      anchorLocalDate: _mode == ScheduleModes.selectedDates
          ? _selectedDates.first
          : _formatDate(_anchorDate),
      localStartTime: _formatClock(_startTime),
      localEndTime: _formatClock(_endTime),
      durationMinutes: int.parse(_durationController.text.trim()),
      timezone: _timezoneController.text.trim(),
      title: title.isEmpty ? null : title,
      endLocalDate: _endDate == null ? null : _formatDate(_endDate!),
      weekCadence: _mode == ScheduleModes.weekdayPattern ? _weekCadence : null,
      weekdays: _mode == ScheduleModes.weekdayPattern
          ? (_weekdays.toList()..sort())
          : null,
      intervalCount: _mode == ScheduleModes.interval
          ? int.parse(_intervalCountController.text.trim())
          : null,
      intervalUnit: _mode == ScheduleModes.interval ? _intervalUnit : null,
      selectedDates:
          _mode == ScheduleModes.selectedDates ? List.of(_selectedDates) : null,
    );
  }

  Future<void> _submit() async {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final error = _validate(rtl);
    if (error != null) {
      setState(() => _formError = error);
      return;
    }
    setState(() => _formError = null);
    final controller =
        ref.read(scheduleControllerProvider(widget.scope).notifier);
    final existing = widget.existing;
    final plan = _buildPlan();
    final saved = await saveWithOverlapReconfirmation(
      context: context,
      rtl: rtl,
      timezone: plan.timezone,
      currentWarnings: () =>
          ref.read(scheduleControllerProvider(widget.scope)).overlapWarnings,
      save: (confirmed, warningIDs) => existing == null
          ? controller.createSchedule(plan,
              confirmOverlaps: confirmed, confirmedWarningIDs: warningIDs)
          : controller.changeSeries(
              schedule: existing,
              plan: plan,
              confirmOverlaps: confirmed,
              confirmedWarningIDs: warningIDs,
            ),
    );
    if (saved && mounted) {
      Navigator.of(context).pop();
    }
  }

  static String _weekdayLabel(int dow, bool rtl) {
    const en = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
    const ar = [
      'الأحد',
      'الاثنين',
      'الثلاثاء',
      'الأربعاء',
      'الخميس',
      'الجمعة',
      'السبت',
    ];
    return rtl ? ar[dow] : en[dow];
  }
}

/// Edit or cancel one unstarted occurrence, identified by its original local
/// date (stable even after a move).
class _OccurrenceEditSheet extends ConsumerStatefulWidget {
  const _OccurrenceEditSheet({required this.scope, required this.entry});

  final ScheduleScope scope;
  final CircleScheduleEntry entry;

  @override
  ConsumerState<_OccurrenceEditSheet> createState() =>
      _OccurrenceEditSheetState();
}

class _OccurrenceEditSheetState extends ConsumerState<_OccurrenceEditSheet> {
  late DateTime _originalDate = _tomorrow();
  DateTime? _replacementDate;
  TimeOfDay? _replacementStart;
  TimeOfDay? _replacementEnd;
  bool _cancel = false;
  String? _formError;

  static DateTime _tomorrow() {
    final now = DateTime.now();
    return DateTime(now.year, now.month, now.day + 1);
  }

  static String _formatDate(DateTime date) =>
      _SchedulePlanFormState._formatDate(date);

  @override
  Widget build(BuildContext context) {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final saveError = ref.watch(
      scheduleControllerProvider(widget.scope).select((s) => s.saveError),
    );
    final isSaving = ref.watch(
      scheduleControllerProvider(widget.scope).select((s) => s.isSaving),
    );

    return Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            rtl ? _Labels.occurrenceTitleAr : _Labels.occurrenceTitleEn,
            style: Theme.of(context).textTheme.titleLarge,
          ),
          const SizedBox(height: 16),
          OutlinedButton.icon(
            key: const Key('occurrenceDateButton'),
            onPressed: () async {
              final picked = await _pickDate(_originalDate);
              if (picked != null) setState(() => _originalDate = picked);
            },
            icon: const Icon(Icons.calendar_today_outlined),
            label: Text(
              '${rtl ? _Labels.occurrenceDateAr : _Labels.occurrenceDateEn}: '
              '${_formatDate(_originalDate)}',
            ),
          ),
          SwitchListTile(
            key: const Key('occurrenceCancelSwitch'),
            title: Text(
                rtl ? _Labels.occurrenceCancelAr : _Labels.occurrenceCancelEn),
            value: _cancel,
            onChanged: (value) => setState(() => _cancel = value),
          ),
          if (!_cancel) ...[
            OutlinedButton.icon(
              key: const Key('occurrenceReplacementDateButton'),
              onPressed: () async {
                final picked = await _pickDate(_replacementDate ?? _tomorrow());
                if (picked != null) {
                  setState(() => _replacementDate = picked);
                }
              },
              icon: const Icon(Icons.event_outlined),
              label: Text(
                _replacementDate == null
                    ? (rtl ? _Labels.moveDateAr : _Labels.moveDateEn)
                    : '${rtl ? _Labels.moveDateAr : _Labels.moveDateEn}: '
                        '${_formatDate(_replacementDate!)}',
              ),
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: OutlinedButton.icon(
                    key: const Key('occurrenceReplacementStartButton'),
                    onPressed: () => _pickTime(isStart: true),
                    icon: const Icon(Icons.schedule_outlined),
                    label: Text(
                      _replacementStart == null
                          ? (rtl ? _Labels.newStartAr : _Labels.newStartEn)
                          : _SchedulePlanFormState._formatClock(
                              _replacementStart!),
                    ),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: OutlinedButton.icon(
                    key: const Key('occurrenceReplacementEndButton'),
                    onPressed: () => _pickTime(isStart: false),
                    icon: const Icon(Icons.schedule_outlined),
                    label: Text(
                      _replacementEnd == null
                          ? (rtl ? _Labels.newEndAr : _Labels.newEndEn)
                          : _SchedulePlanFormState._formatClock(
                              _replacementEnd!),
                    ),
                  ),
                ),
              ],
            ),
          ],
          if (_formError != null) ...[
            const SizedBox(height: 12),
            _FormErrorText(message: _formError!),
          ],
          if (saveError != null) ...[
            const SizedBox(height: 12),
            _FormErrorText(message: _saveErrorCopy(saveError, rtl)),
          ],
          const SizedBox(height: 16),
          FilledButton(
            key: const Key('occurrenceSubmitButton'),
            onPressed: isSaving ? null : _submit,
            child: isSaving
                ? const SizedBox(
                    height: 20,
                    width: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : Text(rtl ? _Labels.applyAr : _Labels.applyEn),
          ),
        ],
      ),
    );
  }

  Future<DateTime?> _pickDate(DateTime initial) {
    final today = DateTime.now();
    final firstDate = DateTime(today.year, today.month, today.day);
    return showDatePicker(
      context: context,
      firstDate: firstDate,
      lastDate: DateTime(9999, 12, 31),
      initialDate: initial.isBefore(firstDate) ? firstDate : initial,
    );
  }

  Future<void> _pickTime({required bool isStart}) async {
    final picked = await showTimePicker(
      context: context,
      initialTime: const TimeOfDay(hour: 18, minute: 0),
    );
    if (picked == null) return;
    setState(() {
      if (isStart) {
        _replacementStart = picked;
      } else {
        _replacementEnd = picked;
      }
    });
  }

  Future<void> _submit() async {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    if (!_cancel &&
        _replacementDate == null &&
        _replacementStart == null &&
        _replacementEnd == null) {
      setState(() {
        _formError =
            rtl ? _Labels.occurrenceNoChangeAr : _Labels.occurrenceNoChangeEn;
      });
      return;
    }
    // The server accepts a clock replacement only as a full start/end/
    // duration triple, so one picked time without the other cannot submit.
    if (!_cancel && (_replacementStart == null) != (_replacementEnd == null)) {
      setState(() {
        _formError =
            rtl ? _Labels.occurrenceTimesPairAr : _Labels.occurrenceTimesPairEn;
      });
      return;
    }
    int? durationMinutes;
    if (_replacementStart != null && _replacementEnd != null) {
      final start = _replacementStart!.hour * 60 + _replacementStart!.minute;
      final end = _replacementEnd!.hour * 60 + _replacementEnd!.minute;
      final delta = (end - start) % 1440;
      durationMinutes = delta == 0 ? 1440 : delta;
    }
    final controller =
        ref.read(scheduleControllerProvider(widget.scope).notifier);
    final saved = await saveWithOverlapReconfirmation(
      context: context,
      rtl: rtl,
      timezone: widget.entry.plan.timezone,
      currentWarnings: () =>
          ref.read(scheduleControllerProvider(widget.scope)).overlapWarnings,
      save: (confirmed, warningIDs) => controller.changeOccurrence(
        schedule: widget.entry,
        localDate: _formatDate(_originalDate),
        replacementLocalDate:
            _replacementDate == null ? null : _formatDate(_replacementDate!),
        replacementLocalTime: _replacementStart == null
            ? null
            : _SchedulePlanFormState._formatClock(_replacementStart!),
        replacementEndLocalTime: _replacementEnd == null
            ? null
            : _SchedulePlanFormState._formatClock(_replacementEnd!),
        durationMinutes: durationMinutes,
        cancel: _cancel,
        confirmOverlaps: confirmed,
        confirmedWarningIDs: warningIDs,
      ),
    );
    if (saved && mounted) {
      Navigator.of(context).pop();
    }
  }
}

class _FormErrorText extends StatelessWidget {
  const _FormErrorText({required this.message});

  final String message;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Icon(Icons.error_outline, color: scheme.error),
        const SizedBox(width: 8),
        Expanded(
          child: Text(
            message,
            style: Theme.of(context)
                .textTheme
                .bodyMedium
                ?.copyWith(color: scheme.error),
          ),
        ),
      ],
    );
  }
}

String _saveErrorCopy(ScheduleSaveError error, bool rtl) => switch (error) {
      ScheduleSaveError.pastDate =>
        rtl ? _Labels.savePastAr : _Labels.savePastEn,
      ScheduleSaveError.validation =>
        rtl ? _Labels.saveValidationAr : _Labels.saveValidationEn,
      ScheduleSaveError.conflict =>
        rtl ? _Labels.saveConflictAr : _Labels.saveConflictEn,
      ScheduleSaveError.permission =>
        rtl ? _Labels.savePermissionAr : _Labels.savePermissionEn,
      ScheduleSaveError.network =>
        rtl ? _Labels.saveNetworkAr : _Labels.saveNetworkEn,
      ScheduleSaveError.unknown =>
        rtl ? _Labels.saveUnknownAr : _Labels.saveUnknownEn,
    };

abstract final class _Labels {
  static const titleAr = 'مواعيد الحلقة';
  static const titleEn = 'Circle schedule';
  static const addAr = 'إضافة موعد';
  static const addEn = 'Add schedule';
  static const emptyTitleAr = 'لا توجد مواعيد بعد';
  static const emptyTitleEn = 'No schedule entries yet';
  static const emptyManagerHintAr = 'أضف أول موعد متكرر لهذه الحلقة';
  static const emptyManagerHintEn =
      'Add the first recurring time for this circle';
  static const emptyViewerHintAr = 'لم يُنشر مواعيد بعد';
  static const emptyViewerHintEn = 'The teachers have not published times yet';
  static const loadErrorAr = 'تعذر تحميل المواعيد';
  static const loadErrorEn = 'Could not load the schedule';
  static const permissionAr = 'لا تملك صلاحية عرض هذه المواعيد';
  static const permissionEn = 'You do not have access to this schedule';
  static const goneAr = 'هذه الحلقة لم تعد متاحة';
  static const goneEn = 'This circle is no longer available';
  static const retryAr = 'إعادة المحاولة';
  static const retryEn = 'Retry';
  static const staleAr = 'غير متصل — يتم عرض آخر مواعيد محفوظة';
  static const staleEn = 'Offline — showing the last loaded schedule';
  static const saveSuccessAr = 'تم حفظ الموعد';
  static const saveSuccessEn = 'Schedule saved';
  static const defaultTitleAr = 'جلسة الحلقة';
  static const defaultTitleEn = 'Circle Session';
  static const stoppedAr = 'متوقف منذ ';
  static const stoppedEn = 'Stopped from ';
  static const editTooltipAr = 'تعديل الموعد';
  static const editTooltipEn = 'Edit schedule';
  static const stopTooltipAr = 'إيقاف الموعد';
  static const stopTooltipEn = 'Stop schedule';
  static const occurrenceTooltipAr = 'تعديل جلسة واحدة';
  static const occurrenceTooltipEn = 'Edit one occurrence';
  static const stopTitleAr = 'إيقاف هذا الموعد؟';
  static const stopTitleEn = 'Stop this schedule?';
  static const stopBodyAr =
      'تتوقف الجلسات القادمة اعتباراً من اليوم، وتبقى الجلسات السابقة محفوظة.';
  static const stopBodyEn =
      'Future occurrences stop from today; past sessions are kept.';
  static const stopConfirmAr = 'إيقاف الموعد';
  static const stopConfirmEn = 'Stop schedule';
  static const cancelAr = 'إلغاء';
  static const cancelEn = 'Cancel';
  static const createTitleAr = 'موعد جديد';
  static const createTitleEn = 'New schedule entry';
  static const editTitleAr = 'تعديل الموعد';
  static const editTitleEn = 'Edit schedule entry';
  static const titleFieldAr = 'العنوان';
  static const titleFieldEn = 'Title';
  static const optionalAr = 'اختياري';
  static const optionalEn = 'Optional';
  static const modeWeekdaysAr = 'أيام أسبوعية';
  static const modeWeekdaysEn = 'Weekly days';
  static const modeIntervalAr = 'فاصل زمني';
  static const modeIntervalEn = 'Interval';
  static const modeDatesAr = 'تواريخ محددة';
  static const modeDatesEn = 'Selected dates';
  static const cadenceWeeklyAr = 'أسبوعي';
  static const cadenceWeeklyEn = 'Weekly';
  static const cadenceBiweeklyAr = 'كل أسبوعين';
  static const cadenceBiweeklyEn = 'Every two weeks';
  static const anchorDateAr = 'أول تاريخ';
  static const anchorDateEn = 'First date';
  static const startTimeAr = 'البداية';
  static const startTimeEn = 'Start';
  static const endTimeAr = 'النهاية';
  static const endTimeEn = 'End';
  static const durationAr = 'المدة (بالدقائق)';
  static const durationEn = 'Duration (minutes)';
  static const timezoneAr = 'المنطقة الزمنية';
  static const timezoneEn = 'Time zone';
  static const endDateNoneAr = 'تاريخ الانتهاء (اختياري)';
  static const endDateNoneEn = 'End date (optional)';
  static const endDateAr = 'تاريخ الانتهاء';
  static const endDateEn = 'End date';
  static const intervalCountAr = 'عدد الوحدات';
  static const intervalCountEn = 'Repeat every';
  static const unitDaysAr = 'أيام';
  static const unitDaysEn = 'Days';
  static const unitWeeksAr = 'أسابيع';
  static const unitWeeksEn = 'Weeks';
  static const addDateAr = 'إضافة تاريخ';
  static const addDateEn = 'Add date';
  static const saveAr = 'حفظ الموعد';
  static const saveEn = 'Save schedule';
  static const applyAr = 'تطبيق';
  static const applyEn = 'Apply';
  static const weekdayRequiredAr = 'اختر يومًا واحدًا على الأقل';
  static const weekdayRequiredEn = 'Select at least one day';
  static const anchorWeekdayAr = 'يجب تحديد يوم التاريخ الأول';
  static const anchorWeekdayEn = "The first date's weekday must be selected";
  static const intervalInvalidAr = 'أدخل رقمًا صحيحًا موجبًا';
  static const intervalInvalidEn = 'Enter a positive whole number';
  static const datesRequiredAr = 'أضف تاريخًا واحدًا على الأقل';
  static const datesRequiredEn = 'Add at least one date';
  static const timezoneInvalidAr = 'أدخل منطقة زمنية صالحة، مثل Africa/Cairo';
  static const timezoneInvalidEn = 'Enter a valid time zone, e.g. Africa/Cairo';
  static const durationInvalidAr = 'أدخل مدة بين 1 و44640 دقيقة';
  static const durationInvalidEn =
      'Enter a duration between 1 and 44640 minutes';
  static const durationMismatchAr =
      'وقت النهاية يجب أن يطابق وقت البداية مضافًا إليه المدة';
  static const durationMismatchEn =
      'End time must match start time plus duration';
  static const endDateInvalidAr =
      'تاريخ الانتهاء يجب أن يكون في أو بعد أول تاريخ';
  static const endDateInvalidEn =
      'The end date must be on or after the first date';
  static const occurrenceTitleAr = 'تعديل جلسة واحدة';
  static const occurrenceTitleEn = 'Edit one occurrence';
  static const occurrenceDateAr = 'التاريخ الأصلي';
  static const occurrenceDateEn = 'Original date';
  static const occurrenceCancelAr = 'إلغاء هذه الجلسة';
  static const occurrenceCancelEn = 'Cancel this occurrence';
  static const occurrenceNoChangeAr = 'اختر تغييرًا أو ألغِ الجلسة';
  static const occurrenceNoChangeEn =
      'Pick a new date or time, or cancel the occurrence';
  static const occurrenceTimesPairAr = 'حدد وقتي البداية والنهاية معًا';
  static const occurrenceTimesPairEn = 'Pick both a new start and a new end';
  static const moveDateAr = 'تاريخ جديد';
  static const moveDateEn = 'New date';
  static const newStartAr = 'بداية جديدة';
  static const newStartEn = 'New start';
  static const newEndAr = 'نهاية جديدة';
  static const newEndEn = 'New end';
  static const savePastAr = 'لا يمكن جدولة تواريخ سابقة';
  static const savePastEn = 'Past dates cannot be scheduled';
  static const saveValidationAr = 'تم رفض الموعد. تحقق من القيم.';
  static const saveValidationEn =
      'The schedule was rejected. Check the values.';
  static const saveConflictAr =
      'تغير هذا الموعد في مكان آخر. راجع وحاول مجددًا.';
  static const saveConflictEn =
      'This schedule changed elsewhere. Review and retry.';
  static const savePermissionAr = 'لا تملك صلاحية تعديل هذه المواعيد';
  static const savePermissionEn =
      'You do not have permission to change this schedule';
  static const saveNetworkAr = 'لا يوجد اتصال. لم يتم حفظ التغيير.';
  static const saveNetworkEn = 'No connection. Your change was not saved.';
  static const saveUnknownAr = 'تعذر الحفظ. حاول مرة أخرى.';
  static const saveUnknownEn = 'Could not save. Try again.';
}
