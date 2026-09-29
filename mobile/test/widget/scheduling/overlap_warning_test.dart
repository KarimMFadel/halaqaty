import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/scheduling/data/calendar_api_client.dart';
import 'package:halaqaty_mobile/features/scheduling/presentation/overlap_warning.dart';

void main() {
  testWidgets('warning names and local overlap interval have a non-color cue',
      (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: CalendarOverlapWarnings(
          warnings: [
            CalendarWarning(
              warningId: 'warn-1',
              firstCircleName: 'Hifz Circle',
              secondCircleName: 'Tajweed Circle',
              overlapStartsAt: _start,
              overlapEndsAt: _end,
            ),
          ],
          rtl: false,
          timezone: 'Africa/Cairo',
        ),
      ),
    ));

    expect(find.text('Schedule overlap'), findsOneWidget);
    expect(find.textContaining('Hifz Circle'), findsOneWidget);
    expect(find.textContaining('Tajweed Circle'), findsOneWidget);
    expect(find.textContaining('18:30–19:00'), findsOneWidget);
    expect(find.byIcon(Icons.warning_amber_rounded), findsOneWidget);
    expect(find.textContaining('you may attend any session'), findsOneWidget);
  });

  testWidgets('warning remains readable in Arabic RTL', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Directionality(
        textDirection: TextDirection.rtl,
        child: Scaffold(
          body: CalendarOverlapWarnings(
            warnings: [
              CalendarWarning(
                warningId: 'warn-1',
                firstCircleName: 'حلقة الحفظ',
                secondCircleName: 'حلقة التجويد',
                overlapStartsAt: _start,
                overlapEndsAt: _end,
              ),
            ],
            rtl: true,
            timezone: 'Africa/Cairo',
          ),
        ),
      ),
    ));

    expect(find.text('تنبيه: تداخل في المواعيد'), findsOneWidget);
    expect(find.textContaining('حلقة الحفظ'), findsOneWidget);
    expect(find.textContaining('18:30–19:00'), findsOneWidget);
    expect(find.byIcon(Icons.warning_amber_rounded), findsOneWidget);
  });

  testWidgets('recurring warnings show the date for each overlap',
      (tester) async {
    final warnings = [
      _warning('first-day', 'Hifz Circle'),
      CalendarWarning(
        warningId: 'second-day',
        firstCircleName: 'Hifz Circle',
        secondCircleName: 'Other Circle',
        overlapStartsAt: DateTime.utc(2026, 10, 7, 15, 30),
        overlapEndsAt: DateTime.utc(2026, 10, 7, 16),
      ),
    ];
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: CalendarOverlapWarnings(
          warnings: warnings,
          rtl: false,
          timezone: 'UTC',
        ),
      ),
    ));

    final localizations = MaterialLocalizations.of(
      tester.element(find.byType(CalendarOverlapWarnings)),
    );
    expect(
      find.textContaining(
          localizations.formatMediumDate(DateTime(2026, 10, 6))),
      findsOneWidget,
    );
    expect(
      find.textContaining(
          localizations.formatMediumDate(DateTime(2026, 10, 7))),
      findsOneWidget,
    );
  });

  testWidgets('a changed 409 warning must be confirmed again', (tester) async {
    var warnings = [_warning('reviewed-id', 'First current warning')];
    final reviewed = <List<String>>[];
    var calls = 0;
    var saved = false;
    await tester.pumpWidget(MaterialApp(
      home: Builder(
          builder: (context) => Scaffold(
                body: FilledButton(
                  onPressed: () async {
                    saved = await saveWithOverlapReconfirmation(
                      context: context,
                      rtl: false,
                      timezone: 'UTC',
                      currentWarnings: () => warnings,
                      save: (confirmed, ids) async {
                        reviewed.add(List.of(ids));
                        calls++;
                        if (calls == 1) {
                          warnings = [
                            _warning('first-id', 'First current warning')
                          ];
                          return false;
                        }
                        if (calls == 2) {
                          warnings = [
                            _warning('fresh-id', 'Refreshed warning')
                          ];
                          return false;
                        }
                        return confirmed && ids.single == 'fresh-id';
                      },
                    );
                  },
                  child: const Text('Save plan'),
                ),
              )),
    ));

    await tester.tap(find.text('Save plan'));
    await tester.pumpAndSettle();
    expect(find.textContaining('First current warning'), findsOneWidget);
    await tester.tap(find.text('Save anyway'));
    await tester.pumpAndSettle();
    expect(find.textContaining('Refreshed warning'), findsOneWidget);
    await tester.tap(find.text('Save anyway'));
    await tester.pumpAndSettle();

    expect(calls, 3);
    expect(reviewed, [
      <String>[],
      ['first-id'],
      ['fresh-id']
    ]);
    expect(saved, isTrue);
  });
}

final _start = DateTime.utc(2026, 10, 6, 15, 30);
final _end = DateTime.utc(2026, 10, 6, 16);

CalendarWarning _warning(String id, String circle) => CalendarWarning(
      warningId: id,
      firstCircleName: circle,
      secondCircleName: 'Other Circle',
      overlapStartsAt: _start,
      overlapEndsAt: _end,
    );
