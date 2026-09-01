import 'package:flutter/material.dart';
import 'package:halaqaty_mobile/features/sessions/data/queue_api_client.dart';
import 'package:halaqaty_mobile/features/sessions/presentation/session_ui_labels.dart';

class QueueGradingPanel extends StatefulWidget {
  const QueueGradingPanel({
    super.key,
    required this.entry,
    required this.gradingRequired,
    required this.lifecycle,
    required this.onComplete,
    required this.onCorrect,
  });

  final QueueEntry entry;
  final bool gradingRequired;
  final String lifecycle;
  final void Function(String? grade, String? notes) onComplete;
  final void Function(String? grade, String? notes, bool clearNotes) onCorrect;

  @override
  State<QueueGradingPanel> createState() => _QueueGradingPanelState();
}

class _QueueGradingPanelState extends State<QueueGradingPanel> {
  late String? _grade = widget.entry.grade;
  late final TextEditingController _notes =
      TextEditingController(text: widget.entry.gradeNotes ?? '');

  bool get _readOnly => widget.lifecycle == 'finalized';
  bool get _completion => widget.entry.status == 'reciting';

  @override
  void dispose() {
    _notes.dispose();
    super.dispose();
  }

  @override
  void didUpdateWidget(covariant QueueGradingPanel oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.entry.id != widget.entry.id ||
        oldWidget.entry.version != widget.entry.version) {
      _grade = widget.entry.grade;
      _notes.text = widget.entry.gradeNotes ?? '';
    }
  }

  @override
  Widget build(BuildContext context) {
    final rtl = Directionality.of(context) == TextDirection.rtl;
    final labels = _Labels(rtl);
    if (_readOnly) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(rtl ? SessionUiLabels.queueEnded : labels.finalized),
          _VisibleGrade(entry: widget.entry, labels: labels),
        ],
      );
    }
    if (!_completion && widget.entry.status != 'completed') {
      return const SizedBox.shrink();
    }
    final title = _completion ? labels.title : labels.correction;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(title, style: Theme.of(context).textTheme.titleSmall),
        if (!_completion) _VisibleGrade(entry: widget.entry, labels: labels),
        const SizedBox(height: 8),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            for (final grade in _grades)
              Semantics(
                button: true,
                selected: _grade == grade,
                label: labels.grade(grade),
                child: ConstrainedBox(
                  constraints:
                      const BoxConstraints(minWidth: 48, minHeight: 48),
                  child: OutlinedButton(
                    onPressed: () => setState(() => _grade = grade),
                    child: ExcludeSemantics(child: Text(labels.grade(grade))),
                  ),
                ),
              ),
          ],
        ),
        const SizedBox(height: 8),
        TextField(
          controller: _notes,
          maxLength: 500,
          decoration: InputDecoration(labelText: labels.notes),
        ),
        Align(
          alignment: rtl ? Alignment.centerLeft : Alignment.centerRight,
          child: Semantics(
            button: true,
            label: labels.save,
            child: ConstrainedBox(
              constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
              child: FilledButton(
                onPressed: (_grade == null &&
                        (widget.gradingRequired || !_completion))
                    ? null
                    : () {
                        final note = _notes.text.isEmpty ? null : _notes.text;
                        if (_completion) {
                          widget.onComplete(_grade, note);
                        } else {
                          widget.onCorrect(_grade, note, false);
                        }
                      },
                child: ExcludeSemantics(child: Text(labels.save)),
              ),
            ),
          ),
        ),
      ],
    );
  }
}

const _grades = ['excellent', 'good', 'acceptable', 'needs_review', 'repeat'];

class _Labels {
  const _Labels(this.rtl);

  final bool rtl;

  String get title => rtl ? 'تقييم التلاوة' : 'Grade recitation';
  String get correction =>
      rtl ? 'تصحيح التقييم أو الملاحظة' : 'Correct grade or note';
  String get notes => rtl ? 'ملاحظات المعلم' : 'Teacher notes';
  String get currentGrade => rtl ? 'التقييم الحالي' : 'Current grade';
  String get currentNotes =>
      rtl ? 'ملاحظة المعلم الحالية' : 'Current teacher note';
  String get save => rtl ? 'حفظ التقييم' : 'Save correction';
  String get finalized => 'Round finalized; grading is read-only';

  String grade(String value) => switch (value) {
        'excellent' => rtl ? 'ممتاز' : 'Excellent',
        'good' => rtl ? 'جيد' : 'Good',
        'acceptable' => rtl ? 'مقبول' : 'Acceptable',
        'needs_review' => rtl ? 'يحتاج مراجعة' : 'Needs review',
        _ => rtl ? 'إعادة' : 'Repeat',
      };
}

class _VisibleGrade extends StatelessWidget {
  const _VisibleGrade({required this.entry, required this.labels});

  final QueueEntry entry;
  final _Labels labels;

  @override
  Widget build(BuildContext context) {
    if (entry.grade == null && entry.gradeNotes == null) {
      return const SizedBox.shrink();
    }
    return Padding(
      padding: const EdgeInsets.only(top: 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (entry.grade != null)
            Text('${labels.currentGrade}: ${labels.grade(entry.grade!)}'),
          if (entry.gradeNotes != null && entry.gradeNotes!.isNotEmpty)
            Text('${labels.currentNotes}: ${entry.gradeNotes}'),
        ],
      ),
    );
  }
}
