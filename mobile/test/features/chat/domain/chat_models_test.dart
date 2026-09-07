import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/domain/chat_models.dart';

const _messageId = '11111111-1111-1111-1111-111111111111';
const _circleId = '22222222-2222-2222-2222-222222222222';
const _senderId = '33333333-3333-3333-3333-333333333333';

Map<String, dynamic> _messageJson({
  String id = _messageId,
  String? circleId = _circleId,
  String messageType = 'text',
  String? content = 'السلام عليكم',
  String deliveryStatus = 'delivered',
  String sentAt = '2026-09-03T12:00:00Z',
}) =>
    {
      'id': id,
      'circle_id': circleId,
      'sender_id': _senderId,
      'sender_name': 'مريم',
      'message_type': messageType,
      'content': content,
      'sent_at': sentAt,
      'delivery_status': deliveryStatus,
    };

ChatMessage _message(
  String id, {
  DateTime? sentAt,
  ChatDeliveryStatus status = ChatDeliveryStatus.delivered,
}) =>
    ChatMessage(
      id: id,
      senderId: _senderId,
      circleId: _circleId,
      content: 'نص',
      type: ChatMessageType.text,
      sentAt: sentAt ?? DateTime.utc(2026, 9, 3, 12),
      deliveryStatus: status,
    );

void main() {
  group('ChatMessage', () {
    test('parses a contract Message projection with plain-text passthrough',
        () {
      final content = '<b>السلام</b> <script>alert(1)</script> & "quoted"';
      final message = ChatMessage.fromJson(_messageJson(content: content));

      expect(message.id, _messageId);
      expect(message.circleId, _circleId);
      expect(message.senderId, _senderId);
      expect(message.senderName, 'مريم');
      expect(message.type, ChatMessageType.text);
      expect(message.sentAt, DateTime.utc(2026, 9, 3, 12));
      expect(message.deliveryStatus, ChatDeliveryStatus.delivered);
      // Safe text: the model never interprets markup; content is stored raw
      // and rendering escapes it later in the presentation layer.
      expect(message.content, content);
    });

    test('parses server-authoritative read status', () {
      final message =
          ChatMessage.fromJson(_messageJson(deliveryStatus: 'read'));

      expect(message.deliveryStatus, ChatDeliveryStatus.read);
    });

    test('keeps client-local pending and sent statuses constructible', () {
      // `pending` and `sent` never arrive from the server (contract enum is
      // delivered/read); they exist for optimistic local projections only.
      final pending = _message('local-1', status: ChatDeliveryStatus.pending);
      final sent = _message('local-2', status: ChatDeliveryStatus.sent);

      expect(pending.deliveryStatus, ChatDeliveryStatus.pending);
      expect(sent.deliveryStatus, ChatDeliveryStatus.sent);
    });

    test('rejects values outside the contract enums', () {
      expect(() => ChatMessage.fromJson(_messageJson(messageType: 'sticker')),
          throwsFormatException);
      expect(() => ChatMessage.fromJson(_messageJson(deliveryStatus: 'queued')),
          throwsFormatException);
    });
  });

  group('ChatMessagePage', () {
    test('parses the cursor pagination envelope', () {
      final page = ChatMessagePage.fromJson({
        'data': [
          _messageJson(),
          _messageJson(id: '44444444-4444-4444-4444-444444444444')
        ],
        'has_more': true,
        'next_before': '55555555-5555-5555-5555-555555555555',
      });

      expect(page.messages, hasLength(2));
      expect(page.hasMore, isTrue);
      expect(page.nextBefore, '55555555-5555-5555-5555-555555555555');
    });

    test('parses the terminal page without a cursor', () {
      final page = ChatMessagePage.fromJson({
        'data': [_messageJson()],
        'has_more': false,
        'next_before': null,
      });

      expect(page.hasMore, isFalse);
      expect(page.nextBefore, isNull);
    });
  });

  group('validateChatText', () {
    test('rejects empty and whitespace-only text', () {
      expect(validateChatText(null), ChatTextValidation.empty);
      expect(validateChatText(''), ChatTextValidation.empty);
      expect(validateChatText('   \n\t'), ChatTextValidation.empty);
    });

    test('accepts 4000 characters and rejects 4001', () {
      final arabic = 'س' * 4000;
      expect(validateChatText(arabic), ChatTextValidation.valid);
      expect(validateChatText('س' * 4001), ChatTextValidation.tooLong);
      expect(validateChatText('a' * 4001), ChatTextValidation.tooLong);
    });
  });

  group('mergeChatMessages', () {
    test('deduplicates by message id with incoming winning', () {
      final existing = [_message('a'), _message('b')];
      final incoming = [
        _message('a', status: ChatDeliveryStatus.read),
        _message('c'),
      ];

      final merged = mergeChatMessages(existing, incoming);

      expect(merged.map((m) => m.id), ['c', 'b', 'a']);
      final replaced = merged.firstWhere((m) => m.id == 'a');
      expect(replaced.deliveryStatus, ChatDeliveryStatus.read);
    });

    test('orders deterministically by sent_at then id, newest first', () {
      final noon = DateTime.utc(2026, 9, 3, 12);
      final merged = mergeChatMessages(
        [
          _message('b', sentAt: noon),
        ],
        [
          _message('a', sentAt: noon), // same instant, lower id sorts after
          _message('z', sentAt: noon.add(const Duration(minutes: 5))),
        ],
      );

      expect(merged.map((m) => m.id), ['z', 'b', 'a']);
    });
  });
}
