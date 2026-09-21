import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_detail_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_management_screen.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_retirement_screen.dart';

import '../../helpers/stub_auth_notifier.dart';

void main() {
  testWidgets('CircleDetailScreen: uses ambient LTR labels', (tester) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authControllerProvider.overrideWith((_) => StubAuthNotifier()),
          circleDetailProvider('circle-1').overrideWith(
            (_) => Future.value(_circle()),
          ),
        ],
        child: const MaterialApp(
          home: CircleDetailScreen(circleId: 'circle-1'),
        ),
      ),
    );
    await tester.pump();

    expect(find.text('Circle details'), findsOneWidget);
    expect(find.text('Maximum capacity'), findsOneWidget);
    expect(find.text('Members'), findsOneWidget);
    final membersTile = tester.widget<ListTile>(
      find
          .ancestor(
            of: find.text('Members'),
            matching: find.byType(ListTile),
          )
          .first,
    );
    expect((membersTile.trailing! as Icon).icon, Icons.chevron_right);
  });

  testWidgets('CircleDetailScreen: constrains long circle names',
      (tester) async {
    const longName = 'T064-direct-denied-teacher-teacher-178948869274982';
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authControllerProvider.overrideWith((_) => StubAuthNotifier()),
          circleDetailProvider('circle-1').overrideWith(
            (_) => Future.value(_circle(name: longName)),
          ),
        ],
        child: const MaterialApp(
          home: CircleDetailScreen(circleId: 'circle-1'),
        ),
      ),
    );
    await tester.pumpAndSettle();

    final title = tester.widget<Text>(find.text(longName));
    expect(title.maxLines, 2);
    expect(title.overflow, TextOverflow.ellipsis);
  });

  testWidgets('CircleDetailScreen: mirrors archived banner in RTL',
      (tester) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authControllerProvider.overrideWith((_) => StubAuthNotifier()),
          circleDetailProvider('circle-1').overrideWith(
            (_) => Future.value(_circle(isArchived: true)),
          ),
        ],
        child: const MaterialApp(
          home: Directionality(
            textDirection: TextDirection.rtl,
            child: CircleDetailScreen(circleId: 'circle-1'),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('تفاصيل الحلقة'), findsOneWidget);
    expect(find.text('الأعضاء'), findsOneWidget);
    expect(find.text('المحادثة'), findsOneWidget);
    final banner = tester.widget<Container>(
      find.byKey(const Key('circleArchivedBanner')),
    );
    final scheme =
        Theme.of(tester.element(find.byType(CircleDetailScreen))).colorScheme;
    expect(banner.color, scheme.secondaryContainer);
    expect(find.byKey(const Key('circleArchivedBanner')), findsOneWidget);
    final archiveIcon = tester.widget<Icon>(
      find.descendant(
        of: find.byKey(const Key('circleArchivedBanner')),
        matching: find.byIcon(Icons.archive),
      ),
    );
    expect(archiveIcon.color, scheme.onSecondaryContainer);
    final membersTile = tester.widget<ListTile>(
      find
          .ancestor(
            of: find.text('الأعضاء'),
            matching: find.byType(ListTile),
          )
          .first,
    );
    expect((membersTile.trailing! as Icon).icon, Icons.chevron_left);
  });

  testWidgets('CircleDetailScreen: keeps provider errors private',
      (tester) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authControllerProvider.overrideWith((_) => StubAuthNotifier()),
          circleDetailProvider('circle-1')
              .overrideWith((_) => throw Exception('database secret')),
        ],
        child: const MaterialApp(
          home: CircleDetailScreen(circleId: 'circle-1'),
        ),
      ),
    );
    await tester.pump();

    expect(find.text('Could not load circle details'), findsOneWidget);
    expect(find.textContaining('database secret'), findsNothing);
  });

  testWidgets('CircleDetailScreen: management and retirement are reachable',
      (tester) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          circleDetailProvider('circle-1').overrideWith(
            (_) => Future.value(_circle()),
          ),
          circleMembersProvider('circle-1').overrideWith(
            (_) => Future.value([
              CircleMember(
                userId: 'teacher-1',
                displayName: 'Teacher',
                role: CircleRole.teacher,
                joinedAt: DateTime.utc(2026, 8, 1),
              ),
            ]),
          ),
        ],
        child: const MaterialApp(
          home: CircleDetailScreen(
            circleId: 'circle-1',
            currentUserId: 'teacher-1',
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    // The chat entry (F-004) sits above management and can push it below
    // the fold, so scroll it into view first — same as the retirement tile.
    await tester.ensureVisible(find.byKey(const Key('openCircleManagement')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('openCircleManagement')));
    await tester.pumpAndSettle();
    expect(find.byType(CircleManagementScreen), findsOneWidget);

    await tester.pageBack();
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.byKey(const Key('openCircleRetirement')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('openCircleRetirement')));
    await tester.pumpAndSettle();
    expect(find.byType(CircleRetirementScreen), findsOneWidget);
  });
}

CircleResponse _circle({String name = 'Circle', bool isArchived = false}) =>
    CircleResponse(
      id: 'circle-1',
      name: name,
      inviteCode: 'HLQ-7X2K',
      inviteLink: 'https://halaqaty.app/join/HLQ-7X2K',
      isArchived: isArchived,
      createdAt: DateTime.utc(2026, 8, 1),
    );
