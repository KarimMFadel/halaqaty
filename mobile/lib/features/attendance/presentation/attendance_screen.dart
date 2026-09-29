import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/attendance/application/attendance_controller.dart';
import 'package:halaqaty_mobile/features/attendance/data/attendance_api_client.dart';

class AttendanceScreen extends ConsumerStatefulWidget {
  const AttendanceScreen({
    super.key,
    required this.sessionId,
    this.canCorrect = false,
  });

  final String sessionId;
  final bool canCorrect;

  @override
  ConsumerState<AttendanceScreen> createState() => _AttendanceScreenState();
}

class _AttendanceScreenState extends ConsumerState<AttendanceScreen> {
  ({String sessionId, bool canCorrect}) get _args =>
      (sessionId: widget.sessionId, canCorrect: widget.canCorrect);

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) {
        ref.read(attendanceControllerProvider(_args).notifier).load();
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(attendanceControllerProvider(_args));
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final controller = ref.read(attendanceControllerProvider(_args).notifier);
    return Scaffold(
      appBar: AppBar(title: Text(rtl ? 'الحضور' : 'Attendance')),
      body: switch (state.status) {
        AttendanceViewStatus.loading when state.records.isEmpty =>
          const Center(child: CircularProgressIndicator()),
        AttendanceViewStatus.error when state.records.isEmpty => Center(
            child: Column(mainAxisSize: MainAxisSize.min, children: [
              Text(_errorLabel(state.failure, rtl)),
              const SizedBox(height: 8),
              OutlinedButton(
                key: const Key('attendanceRetry'),
                onPressed: controller.load,
                child: Text(rtl ? 'إعادة المحاولة' : 'Retry'),
              ),
            ]),
          ),
        _ when state.records.isEmpty => Center(
            child: Text(rtl ? 'لا توجد سجلات حضور' : 'No attendance records'),
          ),
        _ => ListView(
            padding: const EdgeInsets.all(16),
            children: [
              for (final record in state.records)
                Card(
                  key: Key('attendance-${record.userId}'),
                  child: ListTile(
                    title: Text(record.userId),
                    subtitle: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(_statusLabel(record.status, rtl)),
                        if (record.correction case final correction?)
                          Text(rtl
                              ? 'تم التصحيح: ${correction.reason}'
                              : 'Corrected: ${correction.reason}'),
                      ],
                    ),
                    trailing: widget.canCorrect
                        ? IconButton(
                            key: Key('attendanceCorrect-${record.userId}'),
                            tooltip:
                                rtl ? 'تصحيح الحضور' : 'Correct attendance',
                            onPressed: state.savingUserId == record.userId
                                ? null
                                : () => _showCorrection(context, record, rtl),
                            icon: const Icon(Icons.edit_outlined),
                          )
                        : null,
                  ),
                ),
              if (state.failure != null)
                Padding(
                  padding: const EdgeInsets.all(8),
                  child: Text(_errorLabel(state.failure, rtl),
                      style: TextStyle(
                          color: Theme.of(context).colorScheme.error)),
                ),
            ],
          ),
      },
    );
  }

  Future<void> _showCorrection(
    BuildContext context,
    AttendanceRecord record,
    bool rtl,
  ) async {
    final reason = TextEditingController();
    var status = record.status;
    try {
      final result = await showDialog<(AttendanceStatus, String)?>(
        context: context,
        builder: (context) => StatefulBuilder(
          builder: (context, setDialogState) => AlertDialog(
            title: Text(rtl ? 'تصحيح الحضور' : 'Correct attendance'),
            content: Column(mainAxisSize: MainAxisSize.min, children: [
              DropdownButton<AttendanceStatus>(
                key: const Key('attendanceStatus'),
                value: status,
                isExpanded: true,
                items: [
                  for (final value in AttendanceStatus.values)
                    DropdownMenuItem(
                      value: value,
                      child: Text(_statusLabel(value, rtl)),
                    ),
                ],
                onChanged: (value) {
                  if (value != null) setDialogState(() => status = value);
                },
              ),
              TextField(
                key: const Key('attendanceReason'),
                controller: reason,
                maxLength: 1000,
                maxLines: 3,
                onChanged: (_) => setDialogState(() {}),
                decoration: InputDecoration(
                  labelText: rtl ? 'سبب التصحيح' : 'Reason for correction',
                ),
              ),
            ]),
            actions: [
              TextButton(
                onPressed: () => Navigator.of(context).pop(),
                child: Text(rtl ? 'إلغاء' : 'Cancel'),
              ),
              FilledButton(
                key: const Key('attendanceSaveCorrection'),
                onPressed: reason.text.trim().isEmpty
                    ? null
                    : () => Navigator.of(context).pop((status, reason.text)),
                child: Text(rtl ? 'حفظ' : 'Save'),
              ),
            ],
          ),
        ),
      );
      if (result != null && mounted) {
        await ref.read(attendanceControllerProvider(_args).notifier).correct(
              userId: record.userId,
              status: result.$1,
              reason: result.$2,
            );
      }
    } finally {
      reason.dispose();
    }
  }
}

String _statusLabel(AttendanceStatus status, bool rtl) => switch (status) {
      AttendanceStatus.present => rtl ? 'حاضر' : 'Present',
      AttendanceStatus.late => rtl ? 'متأخر' : 'Late',
      AttendanceStatus.absent => rtl ? 'غائب' : 'Absent',
      AttendanceStatus.excused => rtl ? 'بعذر' : 'Excused',
    };

String _errorLabel(AttendanceFailure? failure, bool rtl) => switch (failure) {
      AttendanceFailure.permission =>
        rtl ? 'لا يمكن عرض الحضور' : 'Attendance is unavailable',
      AttendanceFailure.network =>
        rtl ? 'تعذر الاتصال. حاول مرة أخرى.' : 'Connection failed. Try again.',
      _ => rtl ? 'تعذر تحميل الحضور' : 'Unable to load attendance',
    };
