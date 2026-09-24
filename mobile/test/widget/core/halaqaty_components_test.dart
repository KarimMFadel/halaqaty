import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/core/design/halaqaty_components.dart';

/// Builds a minimal harness whose only action invokes the shared
/// under-implementation notice; no navigation, controller, or API exists here.
Future<void> _pumpHarness(
  WidgetTester tester, {
  TextDirection direction = TextDirection.ltr,
}) {
  return tester.pumpWidget(
    MaterialApp(
      home: Directionality(
        textDirection: direction,
        child: Scaffold(
          body: Builder(
            builder: (context) => FilledButton(
              key: const Key('plannedAction'),
              onPressed: () => showHalaqatyUnderImplementationNotice(context),
              child: const Text('Planned action'),
            ),
          ),
        ),
      ),
    ),
  );
}

void main() {
  group('showHalaqatyUnderImplementationNotice', () {
    testWidgets('shows the exact English copy with the stable key',
        (WidgetTester tester) async {
      await _pumpHarness(tester);

      await tester.tap(find.byKey(const Key('plannedAction')));
      await tester.pump();

      expect(
        find.byKey(const Key('halaqatyUnderImplementationSnackBar')),
        findsOneWidget,
      );
      expect(
        find.text(
          'This feature is under implementation and is not available yet.',
        ),
        findsOneWidget,
      );
    });

    testWidgets('shows the exact Arabic copy in RTL',
        (WidgetTester tester) async {
      await _pumpHarness(tester, direction: TextDirection.rtl);

      await tester.tap(find.byKey(const Key('plannedAction')));
      await tester.pump();

      expect(
        find.byKey(const Key('halaqatyUnderImplementationSnackBar')),
        findsOneWidget,
      );
      expect(
        find.text('هذه الميزة قيد التنفيذ وغير متاحة حالياً.'),
        findsOneWidget,
      );
    });

    testWidgets('is dismissible via its action', (WidgetTester tester) async {
      await _pumpHarness(tester);

      await tester.tap(find.byKey(const Key('plannedAction')));
      await tester.pumpAndSettle();
      expect(
        find.byKey(const Key('halaqatyUnderImplementationSnackBar')),
        findsOneWidget,
      );

      await tester.tap(find.text('Dismiss'));
      await tester.pumpAndSettle();

      expect(
        find.byKey(const Key('halaqatyUnderImplementationSnackBar')),
        findsNothing,
      );
    });

    testWidgets('re-invocation replaces the prior instance',
        (WidgetTester tester) async {
      await _pumpHarness(tester);

      await tester.tap(find.byKey(const Key('plannedAction')));
      await tester.pump();
      await tester.tap(find.byKey(const Key('plannedAction')));
      await tester.pump();

      expect(
        find.byKey(const Key('halaqatyUnderImplementationSnackBar')),
        findsOneWidget,
      );
    });

    testWidgets('performs no navigation and invokes no callback',
        (WidgetTester tester) async {
      await _pumpHarness(tester);

      await tester.tap(find.byKey(const Key('plannedAction')));
      await tester.pumpAndSettle();

      // Still on the harness page: nothing pushed, nothing popped, and the
      // helper exposes no callback surface at all.
      expect(find.byKey(const Key('plannedAction')), findsOneWidget);
      expect(find.text('Planned action'), findsOneWidget);
      expect(
        find.byKey(const Key('halaqatyUnderImplementationSnackBar')),
        findsOneWidget,
      );
    });
  });
}
