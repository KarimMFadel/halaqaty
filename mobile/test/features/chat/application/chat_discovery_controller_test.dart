import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_discovery_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';

void main() {
  const circleId = 'circle-1';
  Future<({String token, String sessionId, String userId})>
      credentials() async =>
          (token: 'token', sessionId: 'session', userId: 'teacher-1');

  ChatDiscoveryController buildController(_DiscoveryApi api) =>
      ChatDiscoveryController(api, credentials);

  test('reply selection keeps a safe preview from the same conversation', () {
    final controller = buildController(_DiscoveryApi());
    addTearDown(controller.dispose);
    const target = ChatReplyPreview(
      id: 'target',
      senderName: 'أمينة',
      preview: 'السلام عليكم',
      deleted: false,
    );

    controller.selectReplyTarget(target);

    expect(controller.state.replyTarget?.id, target.id);
    expect(controller.state.replyTarget?.preview, 'السلام عليكم');
    expect(controller.state.replyTarget?.deleted, isFalse);
  });

  test('deleted reply target is redacted without preserving its content', () {
    final controller = buildController(_DiscoveryApi());
    addTearDown(controller.dispose);

    controller.selectReplyTarget(const ChatReplyPreview(
      id: 'deleted-target',
      senderName: 'Amina',
      preview: 'private text',
      deleted: true,
    ));

    expect(controller.state.replyTarget?.deleted, isTrue);
    expect(controller.state.replyTarget?.preview, isEmpty);
  });

  test('searches Arabic and Latin queries through retained group history',
      () async {
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

  test('keeps the server-provided pinned-bar order', () async {
    final api = _DiscoveryApi()
      ..pinned = [
        _message('pinned-5'),
        _message('pinned-4'),
        _message('pinned-3'),
        _message('pinned-2'),
        _message('pinned-1'),
      ];
    final controller = buildController(api);
    addTearDown(controller.dispose);

    await controller.loadPinned(circleId);

    expect(
      controller.state.pinnedMessages.map((message) => message.id),
      ['pinned-5', 'pinned-4', 'pinned-3', 'pinned-2', 'pinned-1'],
    );
  });

  test('surfaces the contracted conflict when a sixth message is pinned',
      () async {
    final api = _DiscoveryApi()
      ..pinFailure = const ChatApiException(
        statusCode: 409,
        code: 'ERR_CONFLICT',
        message: 'circle has reached the maximum of five pinned messages',
      );
    final controller = buildController(api);
    addTearDown(controller.dispose);

    await controller.pin(circleId, 'sixth');

    expect(controller.state.pinLimitReached, isTrue);
  });

  test('search remains available when the circle is archived', () async {
    final api = _DiscoveryApi()
      ..searchPages.add(_page([_message('retained', content: 'ذكر')]));
    final controller = buildController(api);
    addTearDown(controller.dispose);

    controller.setReadOnly(true);
    await controller.search(circleId, 'ذكر');

    expect(controller.state.searchResults.single.id, 'retained');
  });

  test('reply send forwards the target and retains the draft on failure',
      () async {
    final api = _DiscoveryApi()
      ..replyFailure = const ChatApiException(
        statusCode: 503,
        code: 'ERR_UNAVAILABLE',
        message: 'private backend detail',
      );
    final controller = buildController(api);
    addTearDown(controller.dispose);
    controller.selectReplyTarget(const ChatReplyPreview(
      id: 'target',
      senderName: 'Amina',
      preview: 'hello',
      deleted: false,
    ));

    final sent = await controller.sendReply(circleId, 'draft');

    expect(sent, isFalse);
    expect(api.replyToIds, ['target']);
    expect(controller.state.draft, 'draft');
    expect(controller.state.errorMessage, isNot(contains('private backend')));
  });
}

ChatMessage _message(String id,
        {String content = 'text', String? senderName}) =>
    ChatMessage(
      id: id,
      senderId: 'sender-1',
      circleId: 'circle-1',
      content: content,
      type: ChatMessageType.text,
      sentAt: DateTime.utc(2026, 9, 14),
      deliveryStatus: ChatDeliveryStatus.delivered,
      senderName: senderName,
    );

ChatMessagePage _page(List<ChatMessage> messages) =>
    ChatMessagePage(messages: messages, hasMore: false);

class _DiscoveryApi implements ChatDiscoveryApi {
  final searchPages = <ChatMessagePage>[];
  final queries = <String>[];
  List<ChatMessage> pinned = const [];
  Object? pinFailure;
  Object? replyFailure;
  final replyToIds = <String?>[];

  @override
  Future<ChatMessage> sendReply({
    required String token,
    required String sessionId,
    required String circleId,
    required String content,
    required String? replyToId,
  }) async {
    replyToIds.add(replyToId);
    if (replyFailure != null) throw replyFailure!;
    throw UnimplementedError();
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
    if (pinFailure != null) throw pinFailure!;
    throw UnimplementedError();
  }

  @override
  Future<void> unpinMessage({
    required String token,
    required String sessionId,
    required String circleId,
    required String messageId,
  }) async {}
}
