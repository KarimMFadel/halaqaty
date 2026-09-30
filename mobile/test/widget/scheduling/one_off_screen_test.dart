import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/scheduling/presentation/one_off_screen.dart';

void main() {
  testWidgets('invalid start time does not crash end-time validation',
      (tester) async {
    await tester.pumpWidget(_screen(now: DateTime.utc(2026, 10, 6, 17)));
    await tester.enterText(find.byKey(const Key('oneOffStart')), 'bad-time');
    await tester.tap(find.byKey(const Key('oneOffCreate')));
    await tester.pumpAndSettle();

    expect(tester.takeException(), isNull);
    expect(find.text('Enter a valid time'), findsOneWidget);
  });

  testWidgets('rejects a start date before today in the planning timezone',
      (tester) async {
    await tester.pumpWidget(_screen(now: DateTime.utc(2026, 10, 6, 17)));
    await tester.enterText(find.byKey(const Key('oneOffDate')), '2026-10-05');
    await tester.tap(find.byKey(const Key('oneOffCreate')));
    await tester.pumpAndSettle();

    expect(find.text('Start date must be today or later'), findsOneWidget);
  });

  testWidgets('rejects a past start time on today in the planning timezone',
      (tester) async {
    await tester.pumpWidget(_screen(now: DateTime.utc(2026, 10, 6, 19)));
    await tester.tap(find.byKey(const Key('oneOffCreate')));
    await tester.pumpAndSettle();

    expect(find.text('Start time must be now or later'), findsOneWidget);
  });

  testWidgets('shows a validation message for an unknown IANA timezone',
      (tester) async {
    await tester.pumpWidget(_screen(now: DateTime.utc(2026, 10, 6, 17)));
    await tester.enterText(
        find.byKey(const Key('oneOffTimezone')), 'Mars/Olympus');
    await tester.tap(find.byKey(const Key('oneOffCreate')));
    await tester.pumpAndSettle();

    expect(find.text('Enter a valid IANA time zone'), findsOneWidget);
  });
}

Widget _screen({required DateTime now}) => ProviderScope(
      child: MaterialApp(
        home: OneOffScreen(
          circleId: 'circle-1',
          circleName: 'Tajweed Circle',
          initialTimezone: 'UTC',
          now: () => now,
        ),
      ),
    );
