import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/domain/chat_models.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_status_widgets.dart';

void main() {
  testWidgets('delivery status is not color-only', (tester) async {
    await tester.pumpWidget(const MaterialApp(
      home: ChatDeliveryStatusView(status: ChatDeliveryStatus.read),
    ));
    expect(find.text('Read'), findsOneWidget);
    expect(find.byIcon(Icons.visibility), findsOneWidget);
  });

  testWidgets('all four delivery states have distinct text and icons',
      (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Column(
        children: ChatDeliveryStatus.values
            .map((status) => ChatDeliveryStatusView(status: status))
            .toList(),
      ),
    ));

    for (final status in ChatDeliveryStatus.values) {
      expect(find.text(status.name[0].toUpperCase() + status.name.substring(1)),
          findsOneWidget);
    }
  });

  testWidgets('typing indicator renders accessible text', (tester) async {
    await tester.pumpWidget(const MaterialApp(
      home: ChatTypingIndicator(userNames: ['Amina']),
    ));
    expect(find.text('Amina typing…'), findsOneWidget);
  });
}
