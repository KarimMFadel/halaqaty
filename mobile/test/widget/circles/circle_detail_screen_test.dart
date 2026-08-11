import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_detail_screen.dart';

void main() {
  testWidgets('CircleDetailScreen: uses ambient LTR labels', (tester) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
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
  });

  testWidgets('CircleDetailScreen: keeps provider errors private',
      (tester) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
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
}

CircleResponse _circle() => CircleResponse(
      id: 'circle-1',
      name: 'Circle',
      inviteCode: 'HLQ-7X2K',
      inviteLink: 'https://halaqaty.app/join/HLQ-7X2K',
      createdAt: DateTime.utc(2026, 8, 1),
    );
