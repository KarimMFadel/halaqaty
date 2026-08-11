import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_members_screen.dart';

void main() {
  Widget createWidgetUnderTest({
    required List<Override> overrides,
  }) {
    return ProviderScope(
      overrides: overrides,
      child: const MaterialApp(
        home: CircleMembersScreen(circleId: 'circle-1'),
      ),
    );
  }

  testWidgets('displays loading state initially', (tester) async {
    final completer = Completer<List<CircleMember>>();
    await tester.pumpWidget(createWidgetUnderTest(
      overrides: [
        circleMembersProvider('circle-1').overrideWith((ref) => completer.future),
        circleDetailProvider('circle-1')
            .overrideWith((ref) => Future.value(_mockCircle(isArchived: false))),
      ],
    ));

    expect(find.byKey(const Key('circleMembersLoading')), findsOneWidget);
    completer.complete([]);
  });

  testWidgets('displays error state', (tester) async {
    await tester.pumpWidget(createWidgetUnderTest(
      overrides: [
        circleMembersProvider('circle-1').overrideWith((ref) => throw Exception('Network error')),
        circleDetailProvider('circle-1').overrideWith((ref) => Future.value(_mockCircle(isArchived: false))),
      ],
    ));

    await tester.pump();

    expect(find.textContaining('حدث خطأ أثناء تحميل الأعضاء'), findsOneWidget);
  });

  testWidgets('displays members with correct roles', (tester) async {
    final members = [
      CircleMember(
        userId: 'u1',
        displayName: 'أحمد',
        role: CircleRole.student,
        joinedAt: DateTime(2023, 1, 1),
      ),
      CircleMember(
        userId: 'u2',
        displayName: 'محمد',
        role: CircleRole.teacher,
        joinedAt: DateTime(2023, 1, 2),
      ),
    ];

    await tester.pumpWidget(createWidgetUnderTest(
      overrides: [
        circleMembersProvider('circle-1').overrideWith((ref) => Future.value(members)),
        circleDetailProvider('circle-1').overrideWith((ref) => Future.value(_mockCircle(isArchived: false))),
      ],
    ));

    await tester.pump();

    expect(find.text('أحمد'), findsOneWidget);
    expect(find.text('محمد'), findsOneWidget);
    expect(find.text('طالب'), findsOneWidget);
    expect(find.text('معلم'), findsOneWidget);
  });

  testWidgets('displays archived warning if circle is archived', (tester) async {
    await tester.pumpWidget(createWidgetUnderTest(
      overrides: [
        circleMembersProvider('circle-1').overrideWith((ref) => Future.value([])),
        circleDetailProvider('circle-1').overrideWith((ref) => Future.value(_mockCircle(isArchived: true))),
      ],
    ));

    await tester.pumpAndSettle();

    expect(find.byKey(const Key('circleArchivedReadOnlyBanner')), findsOneWidget);
    expect(
      find.textContaining('الحلقة مؤرشفة. لا يمكن تعديل الأعضاء.'),
      findsOneWidget,
    );
  });

  testWidgets('RTL directionality and semantics are applied', (tester) async {
    final semantics = tester.ensureSemantics();
    final members = [
      CircleMember(
        userId: 'u1',
        displayName: 'أحمد',
        role: CircleRole.student,
        joinedAt: DateTime(2023, 1, 1),
      ),
    ];

    await tester.pumpWidget(createWidgetUnderTest(
      overrides: [
        circleMembersProvider('circle-1')
            .overrideWith((ref) => Future.value(members)),
        circleDetailProvider('circle-1').overrideWith((ref) => Future.value(_mockCircle(isArchived: false))),
      ],
    ));

    await tester.pump();

    final directionality = tester.widget<Directionality>(
      find.descendant(
        of: find.byType(CircleMembersScreen),
        matching: find.byType(Directionality),
      ).first,
    );
    expect(directionality.textDirection, TextDirection.rtl);
    expect(
      find.byWidgetPredicate((widget) =>
          widget is Semantics &&
          widget.properties.label == 'دور العضو أحمد: طالب'),
      findsOneWidget,
    );
    semantics.dispose();
  });
}

CircleResponse _mockCircle({required bool isArchived}) {
  return CircleResponse(
    id: 'circle-1',
    name: 'Test Circle',
    inviteCode: '12345',
    inviteLink: 'link',
    createdAt: DateTime(2023, 1, 1),
    isArchived: isArchived,
  );
}
