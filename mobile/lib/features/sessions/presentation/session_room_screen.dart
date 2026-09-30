import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_ui_labels.dart';
import 'package:halaqaty_mobile/features/chat/presentation/group_chat_screen.dart';
import 'package:halaqaty_mobile/features/attendance/presentation/attendance_screen.dart';
import 'package:halaqaty_mobile/features/sessions/application/queue_controller.dart';
import 'package:halaqaty_mobile/features/sessions/application/session_room_controller.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/domain/session_models.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/queue/queue_manager_panel.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/queue/queue_student_panel.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/queue/queue_grading_panel.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_ui_labels.dart';

class SessionRoomScreen extends ConsumerWidget {
  const SessionRoomScreen(
      {super.key,
      required this.sessionId,
      this.canStart = false,
      this.canCorrectAttendance = false,
      this.isEnded = false});
  final String sessionId;
  final bool canStart;
  final bool canCorrectAttendance;
  final bool isEnded;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(sessionRoomControllerProvider(sessionId));
    final displayState =
        isEnded ? state.copyWith(status: SessionRoomStatus.ended) : state;
    final controller =
        ref.read(sessionRoomControllerProvider(sessionId).notifier);
    final queueState = state.queueState;
    final showRoomControls = _showRoomControls(state);
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final circleId = state.connection?.session.circleId;
    return Scaffold(
      appBar: AppBar(title: Text(rtl ? SessionUiLabels.title : 'Live session')),
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          // Fixed header: audio-only status, connection state, and the dominant
          // start/join action stay visible and reachable at any scroll offset.
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(rtl ? SessionUiLabels.audioOnly : 'Audio-only session',
                    style: Theme.of(context).textTheme.headlineSmall),
                const SizedBox(height: 8),
                _RoomStatusSection(
                  state: displayState,
                  rtl: rtl,
                  onRetry: controller.retry,
                  onLeave: controller.leave,
                ),
                if (state.status == SessionRoomStatus.connected &&
                    circleId != null)
                  Padding(
                    padding: const EdgeInsets.only(top: 8),
                    child: OutlinedButton(
                      onPressed: () => Navigator.of(context).push(
                        MaterialPageRoute<void>(
                          builder: (_) => GroupChatScreen(circleId: circleId),
                        ),
                      ),
                      child:
                          Text(rtl ? ChatUiLabels.title : ChatUiLabels.titleEn),
                    ),
                  ),
                // Action failures never render raw errors; the room stays connected.
                if (state.actionErrorMessage != null)
                  Text(rtl ? SessionUiLabels.actionFailed : 'Action failed',
                      style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                          color: Theme.of(context).colorScheme.error)),
                // Once connected, the status section carries the state and the
                // Join/Start action would be contradictory — exactly one
                // truthful dominant action per state.
                if (displayState.status != SessionRoomStatus.connected) ...[
                  const SizedBox(height: 8),
                  FilledButton(
                      key: const Key('sessionRoomPrimaryAction'),
                      // No dominant re-entry once the session has ended or the
                      // failure is terminal: the status section owns the safe
                      // exit copy (FR-009).
                      onPressed: displayState.status ==
                                  SessionRoomStatus.loading ||
                              displayState.status == SessionRoomStatus.ended ||
                              state.recovery == SessionRoomRecovery.terminal
                          ? null
                          : () => canStart
                              ? controller.start(sessionId)
                              : controller.join(sessionId),
                      child: Text(canStart
                          ? (rtl ? SessionUiLabels.start : 'Start session')
                          : (rtl ? SessionUiLabels.join : 'Join'))),
                ],
                if (displayState.status == SessionRoomStatus.ended)
                  OutlinedButton(
                    key: const Key('sessionReviewAttendance'),
                    onPressed: () => Navigator.of(context).push(
                      MaterialPageRoute<void>(
                        builder: (_) => AttendanceScreen(
                          sessionId: sessionId,
                          canCorrect: canCorrectAttendance,
                          participantNames: {
                            for (final participant in state.participants)
                              participant.userId: participant.displayName,
                          },
                        ),
                      ),
                    ),
                    child: Text(rtl ? 'مراجعة الحضور' : 'Review attendance'),
                  ),
              ],
            ),
          ),
          Expanded(
            child: SingleChildScrollView(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  if (state.isModerator && queueState != null) ...[
                    const SizedBox(height: 8),
                    QueueManagerPanel(
                      queue: queueState.queue,
                      status: _queuePanelStatus(state),
                      onPrepare: () => _showRoundDetailsDialog(
                        context,
                        rtl: rtl,
                        title: rtl
                            ? SessionUiLabels.prepareRound
                            : 'Prepare round',
                        onConfirm: controller.prepareQueueRound,
                      ),
                      onReorder: () {
                        final queue = queueState.queue;
                        if (queue != null && queue.lifecycle == 'prepared') {
                          _showQueueReorderDialog(
                            context,
                            rtl: rtl,
                            preorder: queue.preorder,
                            onConfirm: (order) => controller.reorderQueue(
                              order,
                              expectedVersion: queue.version,
                            ),
                          );
                        }
                      },
                      onMove: () => _showMoveEntryDialog(
                        context,
                        rtl: rtl,
                        entries:
                            queueState.queue?.entries ?? const <QueueEntry>[],
                        onConfirm: controller.moveQueueEntry,
                      ),
                      onAdvance: controller.advanceQueue,
                      onStart: controller.startSelectedQueueEntry,
                      onSkip: controller.skipSelectedQueueEntry,
                      onComplete: () => _showGradeDialog(
                        context,
                        rtl: rtl,
                        entryId: queueState.queue?.selectedEntryId,
                        title:
                            rtl ? 'تسجيل إتمام التلاوة' : 'Complete recitation',
                        onConfirm: (
                                {required entryId,
                                grade,
                                notes,
                                required clearNotes}) =>
                            controller.completeQueueEntry(
                          entryId: entryId,
                          grade: grade,
                          notes: notes,
                        ),
                      ),
                      onCorrect: (entryId) {
                        final entries =
                            queueState.queue?.entries ?? const <QueueEntry>[];
                        final matching =
                            entries.where((entry) => entry.id == entryId);
                        final entry = matching.isEmpty ? null : matching.first;
                        if (entry != null) {
                          _showGradeDialog(
                            context,
                            rtl: rtl,
                            entryId: entry.id,
                            initialGrade: entry.grade,
                            initialNotes: entry.gradeNotes,
                            title: rtl ? 'تصحيح التقييم' : 'Correct grade',
                            onConfirm: (
                                    {required entryId,
                                    grade,
                                    notes,
                                    required clearNotes}) =>
                                controller.correctQueueGrade(
                              entryId: entryId,
                              grade: grade,
                              notes: notes,
                              clearNotes: clearNotes,
                            ),
                          );
                        }
                      },
                      onReset: () => _showRoundDetailsDialog(
                        context,
                        rtl: rtl,
                        title: rtl ? SessionUiLabels.resetQueue : 'Reset round',
                        consequence: rtl
                            ? SessionUiLabels.resetConsequence
                            : 'Current round progress will be discarded.',
                        initialQueue: queueState.queue,
                        onConfirm: controller.resetQueueRound,
                      ),
                      onEditPolicy: () {
                        final policy = queueState.queue?.policy;
                        if (policy != null) {
                          _showQueuePolicyDialog(
                            context,
                            rtl: rtl,
                            policy: policy,
                            onConfirm: (
                                    {population,
                                    unfinishedFinalization,
                                    optOut,
                                    gradeVisibility,
                                    gradeCorrection}) =>
                                controller.updateQueuePolicy(
                              expectedVersion: policy.version,
                              population: population,
                              unfinishedFinalization: unfinishedFinalization,
                              optOut: optOut,
                              gradeVisibility: gradeVisibility,
                              gradeCorrection: gradeCorrection,
                            ),
                          );
                        }
                      },
                    ),
                    if (queueState.actionErrorMessage != null)
                      Text(
                        rtl
                            ? SessionUiLabels.queueUpdateFailed
                            : 'Unable to update queue',
                        style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                            color: Theme.of(context).colorScheme.error),
                      ),
                    if (_gradingEntry(queueState.queue) case final entry?)
                      QueueGradingPanel(
                        entry: entry,
                        gradingRequired: queueState.queue!.gradingRequired,
                        lifecycle: queueState.queue!.lifecycle,
                        onComplete: (grade, notes) => unawaited(
                          ref
                              .read(queueControllerProvider(sessionId).notifier)
                              .completeQueueEntry(entry.id,
                                  grade: grade, notes: notes),
                        ),
                        onCorrect: (grade, notes, clearNotes) => unawaited(
                          ref
                              .read(queueControllerProvider(sessionId).notifier)
                              .correctQueueEntry(entry.id,
                                  grade: grade,
                                  notes: notes,
                                  clearNotes: clearNotes),
                        ),
                      ),
                  ],
                  if (!state.isModerator && queueState != null) ...[
                    const SizedBox(height: 8),
                    QueueStudentPanel(
                      queue: queueState.queue,
                      myEntry: _myEntry(
                          queueState.queue?.entries, state.currentUserId),
                      status: _queueStudentPanelStatus(state),
                      optOutStatus:
                          _studentOptOutStatus(queueState.optOutFeedback),
                      onRequestOptOut: controller.requestQueueOptOut,
                    ),
                  ],
                  if (showRoomControls) ...[
                    const SizedBox(height: 8),
                    SectionHeader(
                        title: rtl
                            ? SessionUiLabels.participantsTitle
                            : 'Participants'),
                    if (!state.participants.any((p) => p.isCurrentlyPresent))
                      Text(
                        rtl
                            ? SessionUiLabels.noParticipants
                            : 'No participants are present yet',
                        style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                            color:
                                Theme.of(context).colorScheme.onSurfaceVariant),
                      ),
                    _ParticipantList(
                        participants: state.participants,
                        isModerator: state.isModerator,
                        rtl: rtl,
                        onMute: controller.muteParticipant,
                        onRemove: controller.removeParticipant),
                    const SizedBox(height: 8),
                    _RoomControls(
                        state: state,
                        rtl: rtl,
                        onRaiseHand: controller.raiseHand,
                        onLowerHand: controller.lowerHand,
                        onToggleLock: () => controller.setLock(!state.isLocked),
                        onMuteAll: controller.muteAll),
                    // Destructive session actions are separated from the primary
                    // queue and moderation controls.
                    if (state.isModerator) ...[
                      const Divider(height: 32),
                      FilledButton.tonal(
                        key: const Key('sessionRoomEndSession'),
                        onPressed: controller.endSession,
                        style: FilledButton.styleFrom(
                          backgroundColor:
                              Theme.of(context).colorScheme.errorContainer,
                          foregroundColor:
                              Theme.of(context).colorScheme.onErrorContainer,
                        ),
                        child: Text(
                            rtl ? SessionUiLabels.endSession : 'End session'),
                      ),
                    ],
                  ],
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// Connection state block: branded loading for waits (FR-007), live-region
/// status announcements (FR-014), and distinct retryable versus terminal
/// recovery with a safe exit (FR-009).
class _RoomStatusSection extends StatelessWidget {
  const _RoomStatusSection({
    required this.state,
    required this.rtl,
    required this.onRetry,
    required this.onLeave,
  });

  final SessionRoomState state;
  final bool rtl;
  final Future<void> Function() onRetry;
  final Future<void> Function() onLeave;

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    switch (state.status) {
      case SessionRoomStatus.loading:
        // With a prior connection this wait is a reconnect; match the queue
        // panel's reconnecting copy instead of the first-load copy.
        final reconnecting = state.connection != null;
        return Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const HalaqatyLoading(),
            const SizedBox(height: 8),
            Text(
              reconnecting
                  ? (rtl ? SessionUiLabels.reconnecting : 'Reconnecting...')
                  : (rtl
                      ? SessionUiLabels.loadingParticipants
                      : 'Loading participants...'),
              textAlign: TextAlign.center,
            ),
          ],
        );
      case SessionRoomStatus.error:
        final terminal = state.recovery == SessionRoomRecovery.terminal;
        final message = rtl
            ? (terminal
                ? SessionUiLabels.terminalConnectionError
                : SessionUiLabels.unableToConnect)
            : (terminal
                ? 'Session access has ended'
                : 'Connection was interrupted');
        return Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _StatusAnnouncement(
                label: message,
                style: Theme.of(context)
                    .textTheme
                    .bodyLarge
                    ?.copyWith(color: scheme.error)),
            if (terminal) ...[
              const SizedBox(height: 4),
              Text(
                rtl
                    ? SessionUiLabels.retryUnavailable
                    : 'Retry is unavailable. Leave the session.',
                style: Theme.of(context)
                    .textTheme
                    .bodyMedium
                    ?.copyWith(color: scheme.onSurfaceVariant),
              ),
            ],
            const SizedBox(height: 8),
            Wrap(
              spacing: 8,
              alignment: WrapAlignment.center,
              children: [
                if (!terminal)
                  OutlinedButton(
                    key: const Key('sessionRoomRetry'),
                    onPressed: onRetry,
                    child: Text(rtl ? SessionUiLabels.retry : 'Retry'),
                  ),
                FilledButton(
                  key: const Key('sessionRoomLeave'),
                  onPressed: onLeave,
                  child: Text(rtl ? SessionUiLabels.leave : 'Leave'),
                ),
              ],
            ),
          ],
        );
      case SessionRoomStatus.connected:
        return _StatusAnnouncement(
            label:
                rtl ? SessionUiLabels.connected : 'Connected. Audio is ready.');
      case SessionRoomStatus.ended:
        return _StatusAnnouncement(
            label: rtl ? SessionUiLabels.sessionEnded : 'Session ended');
      case SessionRoomStatus.idle:
        return const SizedBox.shrink();
    }
  }
}

/// Announces dynamic connection state changes to screen readers (FR-014).
class _StatusAnnouncement extends StatelessWidget {
  const _StatusAnnouncement({required this.label, this.style});

  final String label;
  final TextStyle? style;

  @override
  Widget build(BuildContext context) => Semantics(
        container: true,
        liveRegion: true,
        label: label,
        child: ExcludeSemantics(child: Text(label, style: style)),
      );
}

QueueEntry? _gradingEntry(QueueState? queue) {
  if (queue == null) return null;
  for (final entry in queue.entries) {
    if (entry.id == queue.selectedEntryId || entry.status == 'reciting') {
      return entry;
    }
  }
  for (final entry in queue.entries) {
    if (entry.status == 'completed') return entry;
  }
  return null;
}

bool _showRoomControls(SessionRoomState state) =>
    state.status == SessionRoomStatus.connected ||
    (state.status == SessionRoomStatus.error &&
        state.recovery == SessionRoomRecovery.retryable &&
        state.connection != null);

QueueManagerPanelStatus _queuePanelStatus(
  SessionRoomState room,
) {
  final queue = room.queueState;
  if (room.status == SessionRoomStatus.ended ||
      queue?.status == QueueControllerStatus.ended) {
    return QueueManagerPanelStatus.terminal;
  }
  if (room.status == SessionRoomStatus.loading && queue?.queue != null) {
    return QueueManagerPanelStatus.reconnecting;
  }
  return switch (queue?.status) {
    QueueControllerStatus.loading => QueueManagerPanelStatus.loading,
    QueueControllerStatus.idle => QueueManagerPanelStatus.empty,
    QueueControllerStatus.ready when queue?.queue == null =>
      QueueManagerPanelStatus.empty,
    QueueControllerStatus.ready => QueueManagerPanelStatus.ready,
    QueueControllerStatus.error => QueueManagerPanelStatus.recoverableError,
    QueueControllerStatus.ended => QueueManagerPanelStatus.terminal,
    null => QueueManagerPanelStatus.loading,
  };
}

QueueEntry? _myEntry(List<QueueEntry>? entries, String? currentUserId) {
  if (entries == null || currentUserId == null) return null;
  for (final entry in entries) {
    if (entry.studentId == currentUserId) return entry;
  }
  return null;
}

QueueStudentPanelStatus _queueStudentPanelStatus(SessionRoomState room) {
  final queue = room.queueState;
  if (room.status == SessionRoomStatus.ended ||
      queue?.status == QueueControllerStatus.ended) {
    return QueueStudentPanelStatus.terminal;
  }
  if (room.status == SessionRoomStatus.loading && queue?.queue != null) {
    return QueueStudentPanelStatus.reconnecting;
  }
  return switch (queue?.status) {
    QueueControllerStatus.loading => QueueStudentPanelStatus.loading,
    QueueControllerStatus.idle => QueueStudentPanelStatus.empty,
    QueueControllerStatus.ready when queue?.queue?.entries.isEmpty ?? true =>
      QueueStudentPanelStatus.empty,
    QueueControllerStatus.ready => QueueStudentPanelStatus.ready,
    QueueControllerStatus.error => QueueStudentPanelStatus.recoverableError,
    QueueControllerStatus.ended => QueueStudentPanelStatus.terminal,
    null => QueueStudentPanelStatus.loading,
  };
}

StudentOptOutStatus _studentOptOutStatus(QueueOptOutFeedback? feedback) {
  if (feedback == null) return StudentOptOutStatus.notRequested;
  return switch (feedback) {
    QueueOptOutFeedback.pending => StudentOptOutStatus.pending,
    QueueOptOutFeedback.declined => StudentOptOutStatus.declined,
    QueueOptOutFeedback.approved => StudentOptOutStatus.approved,
    QueueOptOutFeedback.autoApproved => StudentOptOutStatus.autoApproved,
  };
}

/// Round types from the StartRoundRequest contract; the server validates.
const _roundTypeValues = [
  'new_memorization',
  'revision',
  'old_revision',
  'test',
];

String _roundTypeLabel(String value, bool rtl) => switch (value) {
      'new_memorization' =>
        rtl ? SessionUiLabels.roundTypeNewMemorization : 'New memorization',
      'revision' => rtl ? SessionUiLabels.roundTypeRevision : 'Revision',
      'old_revision' =>
        rtl ? SessionUiLabels.roundTypeOldRevision : 'Old revision',
      _ => rtl ? SessionUiLabels.roundTypeTest : 'Test',
    };

/// Contract grade values always render localized (FR-028), never raw.
const gradeValues = [
  'excellent',
  'good',
  'acceptable',
  'needs_review',
  'repeat',
];

typedef _RoundDetailsAction = Future<void> Function({
  required String roundType,
  required int surahId,
  required int fromAyah,
  required int toAyah,
  required bool gradingRequired,
});

class _RoundValidationMessage {
  const _RoundValidationMessage(this.arabic, this.english);

  final String arabic;
  final String english;
}

_RoundValidationMessage? _validateRoundDetails(
  int? surahId,
  int? fromAyah,
  int? toAyah,
) {
  if (surahId == null || surahId < 1 || surahId > 114) {
    return const _RoundValidationMessage(
      SessionUiLabels.invalidSurah,
      'Surah number must be between 1 and 114',
    );
  }
  if (fromAyah == null || fromAyah <= 0 || toAyah == null || toAyah <= 0) {
    return const _RoundValidationMessage(
      SessionUiLabels.invalidAyah,
      'Ayah numbers must be positive',
    );
  }
  if (fromAyah > toAyah) {
    return const _RoundValidationMessage(
      SessionUiLabels.invalidAyahRange,
      'From ayah must not exceed to ayah',
    );
  }
  return null;
}

typedef _GradeAction = Future<void> Function({
  required String entryId,
  String? grade,
  String? notes,
  required bool clearNotes,
});

typedef _PolicyAction = Future<void> Function({
  String? population,
  String? unfinishedFinalization,
  String? optOut,
  String? gradeVisibility,
  String? gradeCorrection,
});

Future<void> _showQueueReorderDialog(
  BuildContext context, {
  required bool rtl,
  required List<QueuePreorderItem> preorder,
  required Future<void> Function(List<String>) onConfirm,
}) async {
  final order = List<QueuePreorderItem>.of(preorder)
    ..sort((a, b) => a.position.compareTo(b.position));
  final originalOrder = [for (final student in order) student.studentId];
  await showDialog<void>(
    context: context,
    builder: (_) => StatefulBuilder(
      builder: (context, setDialogState) => AlertDialog(
        title: Text(rtl ? SessionUiLabels.reorderQueue : 'Reorder queue'),
        content: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              for (var index = 0; index < order.length; index++)
                ListTile(
                  title: Text(order[index].studentName),
                  leading: Text('${index + 1}'),
                  trailing: Wrap(
                    children: [
                      IconButton(
                        key: Key('queueMoveUp-${order[index].studentId}'),
                        tooltip: rtl
                            ? 'نقل ${order[index].studentName} للأعلى إلى الموضع $index'
                            : 'Move ${order[index].studentName} up to position $index',
                        onPressed: index == 0
                            ? null
                            : () => setDialogState(() {
                                  final item = order.removeAt(index);
                                  order.insert(index - 1, item);
                                }),
                        icon: const Icon(Icons.arrow_upward),
                      ),
                      IconButton(
                        key: Key('queueMoveDown-${order[index].studentId}'),
                        tooltip: rtl
                            ? 'نقل ${order[index].studentName} للأسفل إلى الموضع ${index + 2}'
                            : 'Move ${order[index].studentName} down to position ${index + 2}',
                        onPressed: index == order.length - 1
                            ? null
                            : () => setDialogState(() {
                                  final item = order.removeAt(index);
                                  order.insert(index + 1, item);
                                }),
                        icon: const Icon(Icons.arrow_downward),
                      ),
                    ],
                  ),
                ),
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: Text(rtl ? SessionUiLabels.cancel : 'Cancel'),
          ),
          FilledButton(
            onPressed: !order.asMap().entries.any((entry) =>
                    entry.value.studentId != originalOrder[entry.key])
                ? null
                : () {
                    Navigator.of(context).pop();
                    unawaited(onConfirm([
                      for (final item in order) item.studentId,
                    ]));
                  },
            child: Text(rtl ? 'حفظ' : 'Save'),
          ),
        ],
      ),
    ),
  );
}

Future<void> _showQueuePolicyDialog(
  BuildContext context, {
  required bool rtl,
  required QueuePolicy policy,
  required _PolicyAction onConfirm,
}) async {
  var population = policy.population;
  var finalization = policy.unfinishedFinalization;
  var optOut = policy.optOut;
  var visibility = policy.gradeVisibility;
  var correction = policy.gradeCorrection;
  await showDialog<void>(
    context: context,
    builder: (_) => StatefulBuilder(
      builder: (context, setDialogState) {
        Widget choice(
          String field,
          String label,
          String value,
          List<String> values,
          ValueChanged<String> onChanged,
        ) =>
            DropdownButtonFormField<String>(
              key: Key('queuePolicy$field'),
              initialValue: value,
              isExpanded: true,
              decoration: InputDecoration(labelText: label),
              items: [
                for (final option in values)
                  DropdownMenuItem(
                    value: option,
                    child: Text(_queuePolicyValueLabel(option, rtl)),
                  ),
              ],
              onChanged: (next) {
                if (next != null) setDialogState(() => onChanged(next));
              },
            );

        return AlertDialog(
          title: Text(rtl ? SessionUiLabels.queuePolicy : 'Queue policy'),
          content: SingleChildScrollView(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                choice(
                    'Population',
                    rtl ? 'المشاركون' : 'Population',
                    population,
                    const ['present_at_activation', 'all_active_students'],
                    (value) => population = value),
                choice(
                    'Finalization',
                    rtl ? 'إنهاء الجولة' : 'Unfinished turns',
                    finalization,
                    const ['mark_unfinished_skipped', 'preserve_last_state'],
                    (value) => finalization = value),
                choice(
                    'OptOut',
                    rtl ? 'الاعتذار' : 'Opt-out',
                    optOut,
                    const ['approval_required', 'auto_approve'],
                    (value) => optOut = value),
                choice(
                    'Visibility',
                    rtl ? 'ظهور التقييم' : 'Grade visibility',
                    visibility,
                    const [
                      'managers_and_student',
                      'managers_only',
                      'all_participants'
                    ],
                    (value) => visibility = value),
                choice(
                    'Correction',
                    rtl ? 'تصحيح التقييم' : 'Grade correction',
                    correction,
                    const [
                      'audited_any_time',
                      'before_round_finalization',
                      'immutable'
                    ],
                    (value) => correction = value),
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(context).pop(),
              child: Text(rtl ? SessionUiLabels.cancel : 'Cancel'),
            ),
            FilledButton(
              onPressed: population == policy.population &&
                      finalization == policy.unfinishedFinalization &&
                      optOut == policy.optOut &&
                      visibility == policy.gradeVisibility &&
                      correction == policy.gradeCorrection
                  ? null
                  : () {
                      Navigator.of(context).pop();
                      unawaited(onConfirm(
                        population:
                            population == policy.population ? null : population,
                        unfinishedFinalization:
                            finalization == policy.unfinishedFinalization
                                ? null
                                : finalization,
                        optOut: optOut == policy.optOut ? null : optOut,
                        gradeVisibility: visibility == policy.gradeVisibility
                            ? null
                            : visibility,
                        gradeCorrection: correction == policy.gradeCorrection
                            ? null
                            : correction,
                      ));
                    },
              child: Text(rtl ? 'حفظ' : 'Save'),
            ),
          ],
        );
      },
    ),
  );
}

String _queuePolicyValueLabel(String value, bool rtl) => switch (value) {
      'present_at_activation' =>
        rtl ? 'الحاضرون عند البدء' : 'Present at activation',
      'all_active_students' =>
        rtl ? 'كل الطلاب النشطين' : 'All active students',
      'mark_unfinished_skipped' =>
        rtl ? 'تخطي غير المكتمل' : 'Skip unfinished turns',
      'preserve_last_state' =>
        rtl ? 'الإبقاء على الحالة' : 'Preserve last state',
      'approval_required' => rtl ? 'موافقة مطلوبة' : 'Approval required',
      'auto_approve' => rtl ? 'موافقة تلقائية' : 'Auto approve',
      'managers_and_student' =>
        rtl ? 'المديرون والطالب' : 'Managers and student',
      'managers_only' => rtl ? 'المديرون فقط' : 'Managers only',
      'all_participants' => rtl ? 'جميع المشاركين' : 'All participants',
      'audited_any_time' => rtl ? 'في أي وقت مع تسجيل' : 'Audited any time',
      'before_round_finalization' =>
        rtl ? 'قبل إنهاء الجولة' : 'Before round finalization',
      'immutable' => rtl ? 'غير قابل للتعديل' : 'Immutable',
      _ => value,
    };

Future<void> _showGradeDialog(
  BuildContext context, {
  required bool rtl,
  required String? entryId,
  required String title,
  String? initialGrade,
  String? initialNotes,
  required _GradeAction onConfirm,
}) async {
  if (entryId == null) return;
  final gradeController = ValueNotifier<String?>(initialGrade);
  final notesController = TextEditingController(text: initialNotes ?? '');
  try {
    await showDialog<void>(
      context: context,
      builder: (_) => AlertDialog(
        title: Text(title),
        content: ValueListenableBuilder<String?>(
          valueListenable: gradeController,
          builder: (context, grade, _) => Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              DropdownButton<String>(
                value: grade,
                hint: Text(rtl ? 'اختر التقييم' : 'Select grade'),
                isExpanded: true,
                items: [
                  for (final value in gradeValues)
                    DropdownMenuItem(
                      value: value,
                      child: Text(SessionUiLabels.gradeLabel(value, rtl)),
                    ),
                ],
                onChanged: (value) => gradeController.value = value,
              ),
              TextField(
                controller: notesController,
                maxLength: 500,
                maxLines: 3,
                decoration: InputDecoration(
                  labelText: rtl ? 'ملاحظات' : 'Notes',
                ),
              ),
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: Text(rtl ? 'إلغاء' : 'Cancel'),
          ),
          FilledButton(
            onPressed: () {
              Navigator.of(context).pop();
              onConfirm(
                entryId: entryId,
                grade: gradeController.value,
                notes: notesController.text,
                clearNotes:
                    initialGrade != null && notesController.text.isEmpty,
              );
            },
            child: Text(rtl ? 'حفظ' : 'Save'),
          ),
        ],
      ),
    );
  } finally {
    gradeController.dispose();
    notesController.dispose();
  }
}

Future<void> _showRoundDetailsDialog(
  BuildContext context, {
  required bool rtl,
  required String title,
  String? consequence,
  QueueState? initialQueue,
  required _RoundDetailsAction onConfirm,
}) =>
    showDialog<void>(
      context: context,
      builder: (_) => _RoundDetailsDialog(
        rtl: rtl,
        title: title,
        consequence: consequence,
        initialQueue: initialQueue,
        onConfirm: onConfirm,
      ),
    );

/// Collects the StartRoundRequest fields. Defaults stay contract-valid;
/// [initialQueue] prefills the current round when resetting.
class _RoundDetailsDialog extends StatefulWidget {
  const _RoundDetailsDialog({
    required this.rtl,
    required this.title,
    this.consequence,
    this.initialQueue,
    required this.onConfirm,
  });

  final bool rtl;
  final String title;

  /// Consequence warning for destructive uses (e.g. reset); null for prepare.
  final String? consequence;
  final QueueState? initialQueue;
  final _RoundDetailsAction onConfirm;

  @override
  State<_RoundDetailsDialog> createState() => _RoundDetailsDialogState();
}

class _RoundDetailsDialogState extends State<_RoundDetailsDialog> {
  final _surahController = TextEditingController();
  final _fromAyahController = TextEditingController();
  final _toAyahController = TextEditingController();
  late String _roundType;
  late bool _gradingRequired;

  @override
  void initState() {
    super.initState();
    final queue = widget.initialQueue;
    final roundType = queue?.roundType;
    _roundType = _roundTypeValues.contains(roundType) ? roundType! : 'revision';
    _surahController.text = (queue?.surahId ?? 1).toString();
    _fromAyahController.text = (queue?.fromAyah ?? 1).toString();
    _toAyahController.text = (queue?.toAyah ?? 7).toString();
    _gradingRequired = queue?.gradingRequired ?? false;
  }

  @override
  void dispose() {
    _surahController.dispose();
    _fromAyahController.dispose();
    _toAyahController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final rtl = widget.rtl;
    final surahId = int.tryParse(_surahController.text);
    final fromAyah = int.tryParse(_fromAyahController.text);
    final toAyah = int.tryParse(_toAyahController.text);
    final validation = _validateRoundDetails(surahId, fromAyah, toAyah);
    final canConfirm = validation == null;
    return AlertDialog(
      title: Text(widget.title),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (widget.consequence != null)
              Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: Text(
                  widget.consequence!,
                  style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                      color: Theme.of(context).colorScheme.onSurfaceVariant),
                ),
              ),
            // The closed button's item text would merge into this label, so
            // descendant semantics stay excluded; popup items still announce.
            MergeSemantics(
              child: Semantics(
                label: rtl ? SessionUiLabels.roundType : 'Round type',
                child: DropdownButton<String>(
                  value: _roundType,
                  isExpanded: true,
                  items: [
                    for (final value in _roundTypeValues)
                      DropdownMenuItem(
                        value: value,
                        child: Text(_roundTypeLabel(value, rtl)),
                      ),
                  ],
                  onChanged: (value) =>
                      setState(() => _roundType = value ?? _roundType),
                ),
              ),
            ),
            _LabeledNumberField(
              label: rtl ? SessionUiLabels.surahNumber : 'Surah number',
              controller: _surahController,
              onChanged: (_) => setState(() {}),
            ),
            _LabeledNumberField(
              label: rtl ? SessionUiLabels.fromAyah : 'From ayah',
              controller: _fromAyahController,
              onChanged: (_) => setState(() {}),
            ),
            _LabeledNumberField(
              label: rtl ? SessionUiLabels.toAyah : 'To ayah',
              controller: _toAyahController,
              onChanged: (_) => setState(() {}),
            ),
            if (validation != null)
              Semantics(
                container: true,
                excludeSemantics: true,
                liveRegion: true,
                label: rtl ? validation.arabic : validation.english,
                child: ExcludeSemantics(
                  child: Text(
                    rtl ? validation.arabic : validation.english,
                    style:
                        TextStyle(color: Theme.of(context).colorScheme.error),
                  ),
                ),
              ),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              value: _gradingRequired,
              onChanged: (value) => setState(() => _gradingRequired = value),
              title: Text(
                  rtl ? SessionUiLabels.gradingRequired : 'Grading required'),
            ),
          ],
        ),
      ),
      actions: [
        _DialogAction(
          label: rtl ? SessionUiLabels.cancel : 'Cancel',
          onPressed: () => Navigator.of(context).pop(),
        ),
        _DialogAction(
          label: rtl ? SessionUiLabels.confirm : 'Confirm',
          filled: true,
          onPressed: canConfirm
              ? () {
                  Navigator.of(context).pop();
                  widget.onConfirm(
                    roundType: _roundType,
                    surahId: surahId!,
                    fromAyah: fromAyah!,
                    toAyah: toAyah!,
                    gradingRequired: _gradingRequired,
                  );
                }
              : null,
        ),
      ],
    );
  }
}

Future<void> _showMoveEntryDialog(
  BuildContext context, {
  required bool rtl,
  required List<QueueEntry> entries,
  required Future<void> Function(String entryId, int newPosition) onConfirm,
}) =>
    showDialog<void>(
      context: context,
      builder: (_) => _MoveEntryDialog(
        rtl: rtl,
        entries: entries,
        onConfirm: onConfirm,
      ),
    );

class _MoveEntryDialog extends StatefulWidget {
  const _MoveEntryDialog({
    required this.rtl,
    required this.entries,
    required this.onConfirm,
  });

  final bool rtl;
  final List<QueueEntry> entries;
  final Future<void> Function(String entryId, int newPosition) onConfirm;

  @override
  State<_MoveEntryDialog> createState() => _MoveEntryDialogState();
}

class _MoveEntryDialogState extends State<_MoveEntryDialog> {
  final _positionController = TextEditingController();
  String? _entryId;

  @override
  void initState() {
    super.initState();
    final entries = _waitingEntries;
    if (entries.isNotEmpty) {
      _entryId = entries.first.id;
      _positionController.text = entries.first.position.toString();
    }
  }

  @override
  void dispose() {
    _positionController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final rtl = widget.rtl;
    final entries = _waitingEntries;
    final entryId = _entryId;
    final position = int.tryParse(_positionController.text);
    final canConfirm = entryId != null &&
        position != null &&
        position >= 1 &&
        position <= widget.entries.length;
    return AlertDialog(
      title: Text(rtl ? SessionUiLabels.moveStudent : 'Move student'),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          MergeSemantics(
            child: Semantics(
              label: rtl ? SessionUiLabels.student : 'Student',
              child: DropdownButton<String>(
                value: entryId,
                isExpanded: true,
                items: [
                  for (final entry in entries)
                    DropdownMenuItem(
                      value: entry.id,
                      child: Text(entry.studentName),
                    ),
                ],
                onChanged: entries.isEmpty
                    ? null
                    : (value) => setState(() => _entryId = value),
              ),
            ),
          ),
          _LabeledNumberField(
            label: rtl ? SessionUiLabels.newPosition : 'New position',
            controller: _positionController,
            onChanged: (_) => setState(() {}),
          ),
        ],
      ),
      actions: [
        _DialogAction(
          label: rtl ? SessionUiLabels.cancel : 'Cancel',
          onPressed: () => Navigator.of(context).pop(),
        ),
        _DialogAction(
          label: rtl ? SessionUiLabels.confirm : 'Confirm',
          filled: true,
          onPressed: canConfirm
              ? () {
                  Navigator.of(context).pop();
                  widget.onConfirm(entryId, position);
                }
              : null,
        ),
      ],
    );
  }

  List<QueueEntry> get _waitingEntries => widget.entries
      .where((entry) => entry.status == 'waiting')
      .toList(growable: false);
}

class _LabeledNumberField extends StatelessWidget {
  const _LabeledNumberField({
    required this.label,
    required this.controller,
    this.onChanged,
  });

  final String label;
  final TextEditingController controller;
  final ValueChanged<String>? onChanged;

  @override
  Widget build(BuildContext context) => Semantics(
        container: true,
        label: label,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: [
            // Visible caption; the accessible name stays on the wrapper so the
            // label is never announced twice.
            ExcludeSemantics(
              child: Text(label, style: Theme.of(context).textTheme.bodySmall),
            ),
            TextFormField(
              controller: controller,
              keyboardType: TextInputType.number,
              onChanged: onChanged,
            ),
          ],
        ),
      );
}

class _DialogAction extends StatelessWidget {
  const _DialogAction({
    required this.label,
    this.onPressed,
    this.filled = false,
  });

  final String label;
  final VoidCallback? onPressed;
  final bool filled;

  @override
  Widget build(BuildContext context) => Semantics(
        button: true,
        label: label,
        child: ConstrainedBox(
          constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
          child: filled
              ? FilledButton(
                  onPressed: onPressed,
                  child: ExcludeSemantics(child: Text(label)),
                )
              : OutlinedButton(
                  onPressed: onPressed,
                  child: ExcludeSemantics(child: Text(label)),
                ),
        ),
      );
}

class _ParticipantList extends StatelessWidget {
  const _ParticipantList(
      {required this.participants,
      required this.isModerator,
      required this.rtl,
      required this.onMute,
      required this.onRemove});

  final List<SessionParticipant> participants;
  final bool isModerator;
  final bool rtl;
  final void Function(String userId) onMute;
  final void Function(String userId) onRemove;

  @override
  Widget build(BuildContext context) {
    return ListView(
      shrinkWrap: true,
      physics: const NeverScrollableScrollPhysics(),
      children: [
        for (final participant
            in participants.where((p) => p.isCurrentlyPresent))
          ListTile(
            contentPadding: EdgeInsets.zero,
            leading: participant.isHandRaised
                ? Semantics(
                    container: true,
                    label: rtl ? SessionUiLabels.handRaised : 'Hand raised',
                    child: Icon(Icons.pan_tool,
                        color: Theme.of(context).colorScheme.primary),
                  )
                : null,
            title: Text(participant.displayName),
            subtitle: Text(_roleLabel(participant.role, rtl)),
            trailing: isModerator
                ? Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      TextButton(
                        onPressed: () => onMute(participant.userId),
                        style: TextButton.styleFrom(
                            minimumSize: const Size(48, 48)),
                        child: Text(
                            rtl ? SessionUiLabels.muteParticipant : 'Mute'),
                      ),
                      TextButton(
                        onPressed: () => onRemove(participant.userId),
                        style: TextButton.styleFrom(
                            foregroundColor:
                                Theme.of(context).colorScheme.error,
                            minimumSize: const Size(48, 48)),
                        child: Text(
                            rtl ? SessionUiLabels.removeParticipant : 'Remove'),
                      ),
                    ],
                  )
                : null,
          ),
      ],
    );
  }

  String _roleLabel(CircleRole role, bool rtl) => switch (role) {
        CircleRole.teacher => rtl ? SessionUiLabels.roleTeacher : 'Teacher',
        CircleRole.supervisor =>
          rtl ? SessionUiLabels.roleSupervisor : 'Supervisor',
        CircleRole.student => rtl ? SessionUiLabels.roleStudent : 'Student',
      };
}

class _RoomControls extends StatelessWidget {
  const _RoomControls(
      {required this.state,
      required this.rtl,
      required this.onRaiseHand,
      required this.onLowerHand,
      required this.onToggleLock,
      required this.onMuteAll});

  final SessionRoomState state;
  final bool rtl;
  final VoidCallback onRaiseHand;
  final VoidCallback onLowerHand;
  final VoidCallback onToggleLock;
  final VoidCallback onMuteAll;

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      alignment: WrapAlignment.center,
      children: [
        OutlinedButton(
          onPressed: onRaiseHand,
          child: Text(rtl ? SessionUiLabels.raiseHand : 'Raise hand'),
        ),
        OutlinedButton(
          onPressed: onLowerHand,
          child: Text(rtl ? SessionUiLabels.lowerHand : 'Lower hand'),
        ),
        if (state.isModerator) ...[
          FilledButton.tonal(
            onPressed: onToggleLock,
            child: Text(state.isLocked
                ? (rtl ? SessionUiLabels.unlockSession : 'Unlock session')
                : (rtl ? SessionUiLabels.lockSession : 'Lock session')),
          ),
          FilledButton.tonal(
            onPressed: onMuteAll,
            child: Text(rtl ? SessionUiLabels.muteAll : 'Mute all'),
          ),
        ],
      ],
    );
  }
}
