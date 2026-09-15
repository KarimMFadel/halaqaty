import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/domain/chat_models.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_widgets.dart';

void main() {
  testWidgets('delete action is localized, accessible, and at least 44dp',
      (tester) async {
    final message = ChatMessage(
      id: 'm1',
      senderId: 'me',
      circleId: 'c',
      content: 'رسالة',
      type: ChatMessageType.text,
      sentAt: DateTime.utc(2026),
      deliveryStatus: ChatDeliveryStatus.delivered,
    );
    await tester.pumpWidget(MaterialApp(
      home: Directionality(
        textDirection: TextDirection.rtl,
        child: ChatMessageBubble(
            message: message, isOwn: true, canDelete: true, onDelete: () {}),
      ),
    ));

    final action = find.bySemanticsLabel('حذف الرسالة');
    expect(action, findsOneWidget);
    expect(tester.getSize(action).width, greaterThanOrEqualTo(44));
    expect(tester.getSize(action).height, greaterThanOrEqualTo(44));
  });
}
