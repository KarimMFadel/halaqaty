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
    final peers = _peerEntries(queue?.entries ?? const <QueueEntry>[]);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(labels.title, style: Theme.of(context).textTheme.titleMedium),
        const SizedBox(height: 8),
        _StatusMessage(status: status, labels: labels),
        if (peers.isNotEmpty) ...[
          const SizedBox(height: 8),
          for (final entry in peers) _EntryRow(entry: entry, labels: labels),
        ],
        if (myEntry != null) ...[
          const SizedBox(height: 8),
          _MyEntryRow(entry: myEntry!, labels: labels),
        ],
        if (!isTerminal && status != QueueStudentPanelStatus.empty) ...[
          const SizedBox(height: 8),
          _OptOutFeedback(
            status: optOutStatus,
            labels: labels,
            onRequest: onRequestOptOut,
          ),
        ],
      ],
    );
  }

  List<QueueEntry> _peerEntries(List<QueueEntry> entries) {
    final myId = myEntry?.id;
    return entries.where((entry) => entry.id != myId).toList(growable: false);
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
    return Text(message);
  }
}

class _EntryRow extends StatelessWidget {
  const _EntryRow({required this.entry, required this.labels});

  final QueueEntry entry;
  final _QueueLabels labels;

  @override
  Widget build(BuildContext context) => Padding(
        padding: const EdgeInsets.symmetric(vertical: 4),
        child: Row(
          children: [
            Semantics(
              container: true,
              label: labels.position(entry.position),
              child: ExcludeSemantics(
                child: Text('${entry.position}. '),
              ),
            ),
            Expanded(child: Text(entry.studentName)),
            const SizedBox(width: 8),
            Semantics(
              container: true,
              label: labels.entryStatus(entry.status),
              child: ExcludeSemantics(
                child: Text(labels.entryStatus(entry.status)),
              ),
            ),
          ],
        ),
      );
}

class _MyEntryRow extends StatelessWidget {
  const _MyEntryRow({required this.entry, required this.labels});

  final QueueEntry entry;
  final _QueueLabels labels;

  @override
  Widget build(BuildContext context) => Padding(
        padding: const EdgeInsets.symmetric(vertical: 4),
        child: Row(
          children: [
            Semantics(
              container: true,
              label: labels.yourPosition(entry.position),
              child: ExcludeSemantics(
                child: Text('${entry.position}. '),
              ),
            ),
            Expanded(child: Text(entry.studentName)),
            const SizedBox(width: 8),
            Semantics(
              container: true,
              label: labels.entryStatus(entry.status),
              child: ExcludeSemantics(
                child: Text(labels.entryStatus(entry.status)),
              ),
            ),
          ],
        ),
      );
}

class _OptOutFeedback extends StatelessWidget {
  const _OptOutFeedback({
    required this.status,
    required this.labels,
    required this.onRequest,
  });

  final StudentOptOutStatus status;
  final _QueueLabels labels;
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
          onPressed: isRequesting ? null : onRequest,
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
}
