import 'package:flutter/material.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_ui_labels.dart';

enum QueueManagerPanelStatus {
  loading,
  empty,
  reconnecting,
  recoverableError,
  terminal,
  ready,
}

class QueueManagerPanel extends StatelessWidget {
  const QueueManagerPanel({
    super.key,
    required this.queue,
    required this.status,
    required this.onPrepare,
    required this.onReorder,
    required this.onMove,
    required this.onAdvance,
    required this.onStart,
    required this.onSkip,
    required this.onEditPolicy,
  });

  final QueueState? queue;
  final QueueManagerPanelStatus status;
  final VoidCallback onPrepare;
  final VoidCallback onReorder;
  final VoidCallback onMove;
  final VoidCallback onAdvance;
  final VoidCallback onStart;
  final VoidCallback onSkip;
  final VoidCallback onEditPolicy;

  @override
  Widget build(BuildContext context) {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final labels = _QueueLabels(rtl);
    final isTerminal = status == QueueManagerPanelStatus.terminal;
    final entries = queue?.entries ?? const <QueueEntry>[];
    final selected = _selectedEntry(entries);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(labels.title, style: Theme.of(context).textTheme.titleMedium),
        const SizedBox(height: 8),
        _StatusMessage(status: status, labels: labels),
        if (selected != null) ...[
          const SizedBox(height: 8),
          Semantics(
            container: true,
            liveRegion: true,
            label: labels.turnAnnouncement(selected.studentName),
            child: ExcludeSemantics(
              child: Text(
                labels.turnAnnouncement(selected.studentName),
                style: Theme.of(context).textTheme.titleSmall,
              ),
            ),
          ),
        ],
        if (entries.isNotEmpty) ...[
          const SizedBox(height: 8),
          for (final entry in entries)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 4),
              child: Row(
                children: [
                  Semantics(
                    container: true,
                    label: labels.entryLabel(entry.position),
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
            ),
        ],
        const SizedBox(height: 8),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            _QueueAction(
              label: labels.prepare,
              onPressed: isTerminal ? null : onPrepare,
            ),
            _QueueAction(
              label: labels.reorder,
              onPressed: isTerminal ? null : onReorder,
            ),
            _QueueAction(
              label: labels.move,
              onPressed: isTerminal ? null : onMove,
            ),
            _QueueAction(
              label: labels.advance,
              onPressed: isTerminal ? null : onAdvance,
            ),
            _QueueAction(
              label: labels.start,
              onPressed: isTerminal ? null : onStart,
            ),
            _QueueAction(
              label: labels.skip,
              onPressed: isTerminal ? null : onSkip,
            ),
            _QueueAction(
              label: labels.policy,
              onPressed: isTerminal ? null : onEditPolicy,
            ),
          ],
        ),
      ],
    );
  }

  QueueEntry? _selectedEntry(List<QueueEntry> entries) {
    final selectedId = queue?.selectedEntryId;
    if (selectedId == null) return null;
    for (final entry in entries) {
      if (entry.id == selectedId) return entry;
    }
    return null;
  }
}

class _StatusMessage extends StatelessWidget {
  const _StatusMessage({required this.status, required this.labels});

  final QueueManagerPanelStatus status;
  final _QueueLabels labels;

  @override
  Widget build(BuildContext context) {
    final message = switch (status) {
      QueueManagerPanelStatus.loading => labels.loading,
      QueueManagerPanelStatus.empty => labels.empty,
      QueueManagerPanelStatus.reconnecting => labels.reconnecting,
      QueueManagerPanelStatus.recoverableError => labels.recoverableError,
      QueueManagerPanelStatus.terminal => labels.terminal,
      QueueManagerPanelStatus.ready => null,
    };
    if (message == null) return const SizedBox.shrink();
    return Text(
      message,
      style: status == QueueManagerPanelStatus.recoverableError
          ? TextStyle(color: Theme.of(context).colorScheme.error)
          : null,
    );
  }
}

class _QueueAction extends StatelessWidget {
  const _QueueAction({required this.label, required this.onPressed});

  final String label;
  final VoidCallback? onPressed;

  @override
  Widget build(BuildContext context) => Semantics(
        button: true,
        label: label,
        child: ConstrainedBox(
          constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
          child: OutlinedButton(
            onPressed: onPressed,
            child: ExcludeSemantics(child: Text(label)),
          ),
        ),
      );
}

class _QueueLabels {
  const _QueueLabels(this.rtl);

  final bool rtl;

  String get title => rtl ? SessionUiLabels.queueTitle : 'Recitation queue';
  String get loading => rtl ? SessionUiLabels.queueLoading : 'Loading queue...';
  String get empty => rtl ? SessionUiLabels.queueEmpty : 'No recitation round';
  String get reconnecting =>
      rtl ? SessionUiLabels.queueReconnecting : 'Reconnecting to queue...';
  String get recoverableError =>
      rtl ? SessionUiLabels.queueUpdateFailed : 'Unable to update queue';
  String get terminal =>
      rtl ? SessionUiLabels.queueEnded : 'Recitation round ended';
  String get prepare => rtl ? SessionUiLabels.prepareRound : 'Prepare round';
  String get reorder => rtl ? SessionUiLabels.reorderQueue : 'Reorder queue';
  String get move => rtl ? SessionUiLabels.moveStudent : 'Move student';
  String get advance => rtl ? SessionUiLabels.selectNext : 'Select next';
  String get start =>
      rtl ? SessionUiLabels.startRecitation : 'Start recitation';
  String get skip => rtl ? SessionUiLabels.skipTurn : 'Skip turn';
  String get policy => rtl ? SessionUiLabels.queuePolicy : 'Queue policy';

  String entryLabel(int position) =>
      rtl ? 'الموضع $position' : 'Position $position';

  String entryStatus(String status) => switch (status) {
        'reciting' => rtl ? SessionUiLabels.reciting : 'Reciting',
        'waiting' => rtl ? SessionUiLabels.waiting : 'Waiting',
        'selected' => rtl ? SessionUiLabels.selected : 'Selected',
        'skipped' => rtl ? SessionUiLabels.skipped : 'Skipped',
        _ => status,
      };

  String turnAnnouncement(String studentName) => rtl
      ? 'دور التلاوة الحالي: $studentName'
      : 'Current recitation turn: $studentName';
}
