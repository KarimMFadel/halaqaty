import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_discovery_controller.dart';
import 'package:halaqaty_mobile/features/chat/domain/chat_models.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_discovery_widgets.dart';

void main() {
  testWidgets('reply preview is Arabic-first and exposes its sender and text',
      (tester) async {
    final semantics = tester.ensureSemantics();
    await tester.pumpWidget(_host(
      direction: TextDirection.rtl,
      child: const ChatReplyPreviewView(
        reply: ChatReplyPreview(
          id: 'reply-1',
          senderName: 'أمينة',
          preview: 'السلام عليكم',
          deleted: false,
        ),
      ),
    ));

    expect(find.text('أمينة'), findsOneWidget);
    expect(find.text('السلام عليكم'), findsOneWidget);
    expect(find.bySemanticsLabel('الرد على أمينة'), findsOneWidget);
    semantics.dispose();
  });

  testWidgets('deleted reply preview redacts the original text in RTL and LTR',
      (tester) async {
    for (final direction in TextDirection.values) {
      await tester.pumpWidget(_host(
        direction: direction,
        child: const ChatReplyPreviewView(
          reply: ChatReplyPreview(
            id: 'deleted-1',
            senderName: 'Amina',
            preview: '',
            deleted: true,
          ),
        ),
      ));

      expect(
          find.textContaining('deleted', findRichText: true), findsOneWidget);
      expect(find.text('private text'), findsNothing);
    }
  });

  testWidgets('search results render Arabic and Latin matches safely',
      (tester) async {
    await tester.pumpWidget(_host(
      direction: TextDirection.rtl,
      child: ChatSearchResults(
        messages: [
          _message('arabic', 'مُحَمَّد'),
          _message('latin', 'Muhammad'),
        ],
      ),
    ));

    expect(find.text('مُحَمَّد'), findsOneWidget);
    expect(find.text('Muhammad'), findsOneWidget);
  });

  testWidgets('pinned bar preserves the supplied server order', (tester) async {
    await tester.pumpWidget(_host(
      direction: TextDirection.rtl,
      child: ChatPinnedBar(messages: [
        _message('newest-pin', 'الأحدث'),
        _message('older-pin', 'الأقدم'),
      ]),
    ));

    expect(
      tester.getTopLeft(find.text('الأحدث')).dy,
      lessThan(tester.getTopLeft(find.text('الأقدم')).dy),
    );
  });

  testWidgets('five-pin conflict is conveyed by text, not color alone',
      (tester) async {
    await tester.pumpWidget(_host(
      direction: TextDirection.rtl,
      child: const ChatPinLimitNotice(),
    ));

    expect(find.bySemanticsLabel('تم تثبيت خمس رسائل بالفعل'), findsOneWidget);
    expect(find.byIcon(Icons.info_outline), findsOneWidget);
  });
}

Widget _host({required TextDirection direction, required Widget child}) =>
    MaterialApp(home: Directionality(textDirection: direction, child: child));

ChatMessage _message(String id, String content) => ChatMessage(
      id: id,
      senderId: 'sender-1',
      circleId: 'circle-1',
      content: content,
      type: ChatMessageType.text,
      sentAt: DateTime.utc(2026, 9, 14),
      deliveryStatus: ChatDeliveryStatus.delivered,
    );
