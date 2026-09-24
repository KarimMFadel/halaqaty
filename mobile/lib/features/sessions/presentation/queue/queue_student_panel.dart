import 'package:flutter/material.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_ui_labels.dart';

enum QueueStudentPanelStatus {
  loading,
  empty,
  reconnecting,
  recoverableError,
  terminal,
  ready,
}

enum StudentOptOutStatus {
  notRequested,
  requesting,
  pending,
  declined,
  approved,
  autoApproved,
}

class QueueStudentPanel extends StatelessWidget {
  const QueueStudentPanel({
    super.key,
    required this.queue,
    required this.myEntry,
    required this.status,
    required this.optOutStatus,
    required this.onRequestOptOut,
  });

  final QueueState? queue;
  final QueueEntry? myEntry;
  final QueueStudentPanelStatus status;
  final StudentOptOutStatus optOutStatus;
  final VoidCallback onRequestOptOut;

  @override
  Widget build(BuildContext context) {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final labels = _QueueLabels(rtl);
    final isTerminal = status == QueueStudentPanelStatus.terminal;
    final entries = _orderedEntries();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(labels.title, style: Theme.of(context).textTheme.titleMedium),
        const SizedBox(height: 8),
        _StatusMessage(status: status, labels: labels),
        if (entries.isNotEmpty) ...[
          const SizedBox(height: 8),
          for (final entry in entries)
            entry.id == myEntry?.id
                ? _MyEntryRow(entry: entry, labels: labels)
                : _EntryRow(entry: entry, labels: labels),
        ],
        if (!isTerminal && status != QueueStudentPanelStatus.empty) ...[
          const SizedBox(height: 8),
          _OptOutFeedback(
            status: optOutStatus,
            labels: labels,
            // The queue is read-only while a snapshot is in flight or stale;
            // only a ready queue accepts an opt-out request.
            interactive: status == QueueStudentPanelStatus.ready,
            onRequest: onRequestOptOut,
          ),
        ],
      ],
    );
  }

  /// Entries render in position order; the student's own row stays in place
  /// with emphasis instead of trailing below the peers.
  List<QueueEntry> _orderedEntries() {
    final entries = [...?queue?.entries];
    final mine = myEntry;
    if (mine != null && !entries.any((entry) => entry.id == mine.id)) {
      entries.add(mine);
    }
    entries.sort((a, b) => a.position.compareTo(b.position));
    return entries;
  }
}

class _StatusMessage extends StatelessWidget {
  const _StatusMessage({required this.status, required this.labels});

  final QueueStudentPanelStatus status;
  final _QueueLabels labels;

  @override
  Widget build(BuildContext context) {
    final message = switch (status) {
      QueueStudentPanelStatus.loading => labels.loading,
      QueueStudentPanelStatus.empty => labels.empty,
      QueueStudentPanelStatus.reconnecting => labels.reconnecting,
      QueueStudentPanelStatus.recoverableError => labels.recoverableError,
      QueueStudentPanelStatus.terminal => labels.terminal,
      QueueStudentPanelStatus.ready => null,
    };
    if (message == null) return const SizedBox.shrink();
    return Text(message, style: Theme.of(context).textTheme.bodyMedium);
  }
}

class _EntryRow extends StatelessWidget {
  const _EntryRow({required this.entry, required this.labels});

  final QueueEntry entry;
  final _QueueLabels labels;

  @override
  Widget build(BuildContext context) {
    // Never inherit the ambient default text style: panel rows must size from
    // the theme (a Scaffold-less host may default to a display-size style).
    final rowStyle = Theme.of(context).textTheme.bodyMedium;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Semantics(
                container: true,
                label: labels.position(entry.position),
                child: ExcludeSemantics(
                  child: Text('${entry.position}. ', style: rowStyle),
                ),
              ),
              Expanded(child: Text(entry.studentName, style: rowStyle)),
              const SizedBox(width: 8),
              Semantics(
                container: true,
                label: labels.entryStatus(entry.status),
                child: ExcludeSemantics(
                  child:
                      Text(labels.entryStatus(entry.status), style: rowStyle),
                ),
              ),
            ],
          ),
          _VisibleGrade(entry: entry, labels: labels),
        ],
      ),
    );
  }
}

class _MyEntryRow extends StatelessWidget {
  const _MyEntryRow({required this.entry, required this.labels});

  final QueueEntry entry;
  final _QueueLabels labels;

  @override
  Widget build(BuildContext context) {
    final rowStyle = Theme.of(context).textTheme.bodyMedium;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Semantics(
                container: true,
                liveRegion: true,
                label: labels.yourPosition(entry.position),
                child: ExcludeSemantics(
                  child: Text('${entry.position}. ', style: rowStyle),
                ),
              ),
              Expanded(child: Text(entry.studentName, style: rowStyle)),
              const SizedBox(width: 8),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                decoration: BoxDecoration(
                  color: Theme.of(context).colorScheme.secondaryContainer,
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Text(
                  labels.you,
                  style: Theme.of(context).textTheme.labelSmall?.copyWith(
                      color:
                          Theme.of(context).colorScheme.onSecondaryContainer),
                ),
              ),
              const SizedBox(width: 8),
              Semantics(
                container: true,
                liveRegion: true,
                label: labels.entryStatus(entry.status),
                child: ExcludeSemantics(
                  child:
                      Text(labels.entryStatus(entry.status), style: rowStyle),
                ),
              ),
            ],
          ),
          _VisibleGrade(entry: entry, labels: labels),
        ],
      ),
    );
  }
}

class _VisibleGrade extends StatelessWidget {
  const _VisibleGrade({required this.entry, required this.labels});

  final QueueEntry entry;
  final _QueueLabels labels;

  @override
  Widget build(BuildContext context) {
    if (entry.grade == null && entry.gradeNotes == null) {
      return const SizedBox.shrink();
    }
    return Padding(
      padding: const EdgeInsets.only(top: 4),
      child: Text(labels.grade(entry.grade, entry.gradeNotes),
          style: Theme.of(context).textTheme.bodySmall),
    );
  }
}

class _OptOutFeedback extends StatelessWidget {
  const _OptOutFeedback({
    required this.status,
    required this.labels,
    required this.interactive,
    required this.onRequest,
  });

  final StudentOptOutStatus status;
  final _QueueLabels labels;

  /// False while the queue is loading/reconnecting/stale: the action stays
  /// visible but disabled, mirroring the room's paused Join treatment.
  final bool interactive;
  final VoidCallback onRequest;

  @override
  Widget build(BuildContext context) {
    final message = switch (status) {
      StudentOptOutStatus.pending => labels.optOutPending,
      StudentOptOutStatus.declined => labels.optOutDeclined,
      StudentOptOutStatus.approved => labels.optOutApproved,
      StudentOptOutStatus.autoApproved => labels.optOutAutoApproved,
      _ => null,
    };
    if (message != null) {
      return Semantics(
        container: true,
        liveRegion: status == StudentOptOutStatus.pending,
        label: message,
        child: ExcludeSemantics(
          child: Text(message),
        ),
      );
    }

    final isRequesting = status == StudentOptOutStatus.requesting;
    final label = isRequesting ? labels.optOutRequesting : labels.optOutAction;

    return Semantics(
      button: true,
      label: label,
      child: ConstrainedBox(
        constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
        child: OutlinedButton(
          onPressed: isRequesting || !interactive ? null : onRequest,
          child: ExcludeSemantics(child: Text(label)),
        ),
      ),
    );
  }
}

class _QueueLabels {
  const _QueueLabels(this.rtl);

  final bool rtl;

  String get title => rtl ? SessionUiLabels.queueTitle : 'Recitation queue';
  String get loading => rtl ? SessionUiLabels.queueLoading : 'Loading queue...';
  String get empty => rtl
      ? SessionUiLabels.queueEmptyGuidance
      : 'No recitation round yet; your turn will appear here';
  String get reconnecting =>
      rtl ? SessionUiLabels.queueReconnecting : 'Reconnecting to queue...';
  String get recoverableError =>
      rtl ? SessionUiLabels.queueUpdateFailed : 'Unable to update queue';
  String get terminal =>
      rtl ? SessionUiLabels.queueEnded : 'Recitation round ended';
  String get you => rtl ? 'أنت' : 'You';
  String get optOutAction =>
      rtl ? SessionUiLabels.optOutAction : 'Opt out of turn';
  String get optOutRequesting =>
      rtl ? SessionUiLabels.optOutRequesting : 'Sending opt-out...';
  String get optOutPending =>
      rtl ? SessionUiLabels.optOutPending : 'Awaiting teacher approval';
  String get optOutDeclined =>
      rtl ? SessionUiLabels.optOutDeclined : 'Your turn stays saved for you';
  String get optOutApproved =>
      rtl ? SessionUiLabels.optOutApproved : 'Opt-out approved';
  String get optOutAutoApproved => rtl
      ? SessionUiLabels.optOutAutoApproved
      : 'Opt-out approved automatically';

  String yourPosition(int position) =>
      rtl ? SessionUiLabels.yourPosition(position) : 'Your position: $position';

  String position(int position) =>
      rtl ? SessionUiLabels.position(position) : 'Position $position';

  String entryStatus(String status) => switch (status) {
        'reciting' => rtl ? SessionUiLabels.reciting : 'Reciting',
        'waiting' => rtl ? SessionUiLabels.waiting : 'Waiting',
        'selected' => rtl ? SessionUiLabels.selected : 'Selected',
        'skipped' => rtl ? SessionUiLabels.skipped : 'Skipped',
        'opted_out' => rtl ? SessionUiLabels.optedOut : 'Opted out',
        _ => status,
      };

  String grade(String? value, String? notes) {
    final parts = <String>[
      if (value != null)
        rtl ? 'التقييم: ${_gradeLabel(value)}' : 'Grade: ${_gradeLabel(value)}',
      if (notes != null && notes.isNotEmpty)
        rtl ? 'ملاحظة: $notes' : 'Note: $notes',
    ];
    return parts.join(' · ');
  }

  String _gradeLabel(String value) => SessionUiLabels.gradeLabel(value, rtl);
}
