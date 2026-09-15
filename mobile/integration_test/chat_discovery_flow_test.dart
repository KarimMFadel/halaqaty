import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_discovery_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/domain/chat_models.dart';
import 'package:halaqaty_mobile/features/chat/presentation/chat_discovery_widgets.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  const circleId = 'circle-discovery';

  ChatDiscoveryController buildController(_DiscoveryApi api) {
    return ChatDiscoveryController(
      api,
      () async => (
        token: 'test-token',
        sessionId: 'test-session',
        userId: 'teacher-1',
      ),
    );
  }

  testWidgets('T079: replies to a message through the discovery seam',
      (tester) async {
    final api = _DiscoveryApi()
      ..replyResult = _message('reply-1', content: 'new reply');
    final controller = buildController(api);
    addTearDown(controller.dispose);

    controller.selectReplyTarget(const ChatReplyPreview(
      id: 'source-1',
      senderName: 'Amina',
      preview: 'السلام عليكم',
      deleted: false,
    ));

    expect(await controller.sendReply(circleId, 'وعليكم السلام'), isTrue);
    expect(api.replyToIds, ['source-1']);
    expect(api.replyContents, ['وعليكم السلام']);
    expect(controller.state.replyTarget, isNull);
  });

  testWidgets('T079: searches Arabic and Latin retained history',
      (tester) async {
    final api = _DiscoveryApi()
      ..searchPages.addAll([
        _page([_message('arabic', content: 'مُحَمَّد')]),
        _page([_message('latin', content: 'Muhammad')]),
      ]);
    final controller = buildController(api);
    addTearDown(controller.dispose);

    await controller.search(circleId, 'محمد');
    expect(controller.state.searchResults.single.id, 'arabic');
    await controller.search(circleId, 'muhammad');
    expect(controller.state.searchResults.single.id, 'latin');
    expect(api.queries, ['محمد', 'muhammad']);
  });

  testWidgets('T079: pins and unpins messages in server order', (tester) async {
    final api = _DiscoveryApi()
      ..pinned = [_message('older-pin')]
      ..pinResult = _message('newer-pin');
    final controller = buildController(api);
    addTearDown(controller.dispose);

    await controller.loadPinned(circleId);
    await controller.pin(circleId, 'newer-pin');
    expect(controller.state.pinnedMessages.map((message) => message.id),
        ['newer-pin', 'older-pin']);

    await controller.unpin(circleId, 'newer-pin');
    expect(controller.state.pinnedMessages.map((message) => message.id),
        ['older-pin']);
    expect(api.pinnedIds, ['newer-pin']);
    expect(api.unpinnedIds, ['newer-pin']);
  });

  testWidgets('T079: deletion redacts reply content in the rendered preview',
      (tester) async {
    final deleted = ChatMessage(
      id: 'deleted-source',
      senderId: 'sender-1',
      circleId: circleId,
      content: 'private secret',
      type: ChatMessageType.text,
      sentAt: DateTime.utc(2026, 9, 15, 9),
      deliveryStatus: ChatDeliveryStatus.delivered,
      deletedAt: DateTime.utc(2026, 9, 15, 10),
    );
    final preview = ChatReplyPreview.fromMessage(deleted);

    await tester.pumpWidget(MaterialApp(
      home: Directionality(
        textDirection: TextDirection.rtl,
        child: ChatReplyPreviewView(reply: preview),
      ),
    ));

    expect(find.textContaining('private secret'), findsNothing);
    expect(find.textContaining('الرسالة الأصلية محذوفة'), findsOneWidget);
    expect(preview.preview, isEmpty);
    expect(preview.deleted, isTrue);
  });

  testWidgets('T079: archived discovery remains readable but denies mutations',
      (tester) async {
    final api = _DiscoveryApi()
      ..searchPages.add(_page([_message('retained', content: 'ذكر')]))
      ..replyResult = _message('should-not-send');
    final controller = buildController(api);
    addTearDown(controller.dispose);
    controller.setReadOnly(true);
    controller.selectReplyTarget(const ChatReplyPreview(
      id: 'retained',
      senderName: 'Amina',
      preview: 'ذكر',
      deleted: false,
    ));

    await controller.search(circleId, 'ذكر');
    expect(controller.state.searchResults.single.id, 'retained');
    expect(await controller.sendReply(circleId, 'محاولة تعديل'), isFalse);
    await controller.pin(circleId, 'retained');
    await controller.unpin(circleId, 'retained');
    expect(api.replyToIds, isEmpty);
    expect(api.pinnedIds, isEmpty);
    expect(api.unpinnedIds, isEmpty);
  });
}

ChatMessage _message(String id, {String content = 'text'}) => ChatMessage(
      id: id,
      senderId: 'sender-1',
      circleId: 'circle-discovery',
      content: content,
      type: ChatMessageType.text,
      sentAt: DateTime.utc(2026, 9, 15, 9),
      deliveryStatus: ChatDeliveryStatus.delivered,
    );

ChatMessagePage _page(List<ChatMessage> messages) =>
    ChatMessagePage(messages: messages, hasMore: false);

class _DiscoveryApi implements ChatDiscoveryApi {
  final searchPages = <ChatMessagePage>[];
  final queries = <String>[];
  final replyToIds = <String?>[];
  final replyContents = <String>[];
  final pinnedIds = <String>[];
  final unpinnedIds = <String>[];
  List<ChatMessage> pinned = const [];
  ChatMessage? replyResult;
  ChatMessage? pinResult;

  @override
  Future<ChatMessage> sendReply({
    required String token,
    required String sessionId,
    required String circleId,
    required String content,
    required String? replyToId,
  }) async {
    replyToIds.add(replyToId);
    replyContents.add(content);
    return replyResult ?? _message('reply');
  }

  @override
  Future<ChatMessagePage> searchMessages({
    required String token,
    required String sessionId,
    required String circleId,
    required String query,
  }) async {
    queries.add(query);
    return searchPages.removeAt(0);
  }

  @override
  Future<List<ChatMessage>> listPinnedMessages({
    required String token,
    required String sessionId,
    required String circleId,
  }) async =>
      pinned;

  @override
  Future<ChatMessage> pinMessage({
    required String token,
    required String sessionId,
    required String circleId,
    required String messageId,
  }) async {
    pinnedIds.add(messageId);
    return pinResult ?? _message(messageId);
  }

  @override
  Future<void> unpinMessage({
    required String token,
    required String sessionId,
    required String circleId,
    required String messageId,
  }) async {
    unpinnedIds.add(messageId);
  }
}
