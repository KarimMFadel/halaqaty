import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
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
      {super.key, required this.sessionId, this.canStart = false});
  final String sessionId;
  final bool canStart;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(sessionRoomControllerProvider(sessionId));
    final controller =
        ref.read(sessionRoomControllerProvider(sessionId).notifier);
    final queueState = state.queueState;
    final showRoomControls = _showRoomControls(state);
    final rtl = Directionality.of(context) == TextDirection.rtl;
    return Scaffold(
      appBar: AppBar(title: Text(rtl ? SessionUiLabels.title : 'Live session')),
      body: Padding(
        padding: const EdgeInsets.all(16),
        child:
            Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          Text(rtl ? SessionUiLabels.audioOnly : 'Audio-only session',
              style: Theme.of(context).textTheme.headlineSmall),
          const SizedBox(height: 16),
          if (state.status == SessionRoomStatus.loading)
            const Center(child: CircularProgressIndicator()),
          if (state.status == SessionRoomStatus.loading)
            Text(rtl
                ? SessionUiLabels.loadingParticipants
                : 'Loading participants...'),
          if (state.status == SessionRoomStatus.error) ...[
            Text(
                rtl
                    ? (state.recovery == SessionRoomRecovery.terminal
                        ? SessionUiLabels.terminalConnectionError
                        : SessionUiLabels.unableToConnect)
                    : (state.recovery == SessionRoomRecovery.terminal
                        ? 'Session access has ended'
                        : 'Connection was interrupted'),
                style: TextStyle(color: Theme.of(context).colorScheme.error)),
            const SizedBox(height: 8),
            Wrap(
              spacing: 8,
              alignment: WrapAlignment.center,
              children: [
                if (state.recovery == SessionRoomRecovery.retryable)
                  OutlinedButton(
                    onPressed: controller.retry,
                    child: Text(rtl ? SessionUiLabels.retry : 'Retry'),
                  ),
                FilledButton(
                  onPressed: controller.leave,
                  child: Text(rtl ? SessionUiLabels.leave : 'Leave'),
                ),
              ],
            ),
          ],
          if (state.status == SessionRoomStatus.connected)
            Text(
                rtl ? SessionUiLabels.connected : 'Connected. Audio is ready.'),
          if (state.status == SessionRoomStatus.ended)
            Text(rtl ? SessionUiLabels.sessionEnded : 'Session ended'),
          // Action failures never render raw errors; the room stays connected.
          if (state.actionErrorMessage != null)
            Text(rtl ? SessionUiLabels.actionFailed : 'Action failed',
                style: TextStyle(color: Theme.of(context).colorScheme.error)),
          if (state.isModerator && queueState != null) ...[
            const SizedBox(height: 8),
            QueueManagerPanel(
              queue: queueState.queue,
              status: _queuePanelStatus(state),
              onPrepare: () => _showRoundDetailsDialog(
                context,
                rtl: rtl,
                title: rtl ? SessionUiLabels.prepareRound : 'Prepare round',
                onConfirm: controller.prepareQueueRound,
              ),
              // Reorder and policy editing are owned by later tasks.
              onReorder: () => _showQueueActionPrompt(context, rtl),
              onMove: () => _showMoveEntryDialog(
                context,
                rtl: rtl,
                entries: queueState.queue?.entries ?? const <QueueEntry>[],
                onConfirm: controller.moveQueueEntry,
              ),
              onAdvance: controller.advanceQueue,
              onStart: controller.startSelectedQueueEntry,
              onSkip: controller.skipSelectedQueueEntry,
              onReset: () => _showRoundDetailsDialog(
                context,
                rtl: rtl,
                title: rtl ? SessionUiLabels.resetQueue : 'Reset round',
                initialQueue: queueState.queue,
                onConfirm: controller.resetQueueRound,
              ),
              onEditPolicy: () => _showQueueActionPrompt(context, rtl),
            ),
            if (_gradingEntry(queueState.queue) case final entry?)
              QueueGradingPanel(
                entry: entry,
                gradingRequired: queueState.queue!.gradingRequired,
                lifecycle: queueState.queue!.lifecycle,
                onComplete: (grade, notes) => unawaited(
                  ref
                      .read(queueControllerProvider(sessionId).notifier)
                      .completeQueueEntry(entry.id, grade: grade, notes: notes),
                ),
                onCorrect: (grade, notes, clearNotes) => unawaited(
                  ref
                      .read(queueControllerProvider(sessionId).notifier)
                      .correctQueueEntry(entry.id,
                          grade: grade, notes: notes, clearNotes: clearNotes),
                ),
              ),
          ],
          if (!state.isModerator && queueState != null) ...[
            const SizedBox(height: 8),
            QueueStudentPanel(
              queue: queueState.queue,
              myEntry: _myEntry(queueState.queue?.entries, state.currentUserId),
              status: _queueStudentPanelStatus(state),
              optOutStatus: _studentOptOutStatus(queueState.optOutFeedback),
              onRequestOptOut: controller.requestQueueOptOut,
            ),
          ],
          if (showRoomControls) ...[
            const SizedBox(height: 8),
            Text(rtl ? SessionUiLabels.participantsTitle : 'Participants',
                style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 4),
            Expanded(
                child: _ParticipantList(
                    participants: state.participants,
                    isModerator: state.isModerator,
                    rtl: rtl,
                    onMute: controller.muteParticipant,
                    onRemove: controller.removeParticipant)),
            const SizedBox(height: 8),
            _RoomControls(
                state: state,
                rtl: rtl,
                onRaiseHand: controller.raiseHand,
                onLowerHand: controller.lowerHand,
                onToggleLock: () => controller.setLock(!state.isLocked),
                onMuteAll: controller.muteAll,
                onEnd: controller.endSession),
          ],
          const Spacer(),
          FilledButton(
              onPressed: state.status == SessionRoomStatus.loading
                  ? null
                  : () => canStart
                      ? controller.start(sessionId)
                      : controller.join(sessionId),
              child: Text(canStart
                  ? (rtl ? SessionUiLabels.start : 'Start session')
                  : (rtl ? SessionUiLabels.join : 'Join'))),
        ]),
      ),
    );
  }
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
    QueueControllerStatus.ready when queue?.queue?.entries.isEmpty ?? true =>
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

void _showQueueActionPrompt(BuildContext context, bool rtl) {
  ScaffoldMessenger.of(context).showSnackBar(
    SnackBar(
      content: Text(
          rtl ? 'اختر تفاصيل الجولة أولًا' : 'Choose the round details first'),
    ),
  );
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

Future<void> _showRoundDetailsDialog(
  BuildContext context, {
  required bool rtl,
  required String title,
  QueueState? initialQueue,
  required _RoundDetailsAction onConfirm,
}) =>
    showDialog<void>(
      context: context,
      builder: (_) => _RoundDetailsDialog(
        rtl: rtl,
        title: title,
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
    this.initialQueue,
    required this.onConfirm,
  });

  final bool rtl;
  final String title;
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
        child: TextFormField(
          controller: controller,
          keyboardType: TextInputType.number,
          onChanged: onChanged,
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
                        child: Text(
                            rtl ? SessionUiLabels.muteParticipant : 'Mute'),
                      ),
                      TextButton(
                        onPressed: () => onRemove(participant.userId),
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
      required this.onMuteAll,
      required this.onEnd});

  final SessionRoomState state;
  final bool rtl;
  final VoidCallback onRaiseHand;
  final VoidCallback onLowerHand;
  final VoidCallback onToggleLock;
  final VoidCallback onMuteAll;
  final VoidCallback onEnd;

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
          FilledButton.tonal(
            onPressed: onEnd,
            style: FilledButton.styleFrom(
                backgroundColor: Theme.of(context).colorScheme.errorContainer),
            child: Text(rtl ? SessionUiLabels.endSession : 'End session'),
          ),
        ],
      ],
    );
  }
}
