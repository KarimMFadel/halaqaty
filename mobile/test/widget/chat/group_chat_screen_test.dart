import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/auth/application/auth_controller.dart';
import 'package:halaqaty_mobile/features/auth/data/auth_api_client.dart';
import 'package:halaqaty_mobile/features/chat/application/group_chat_controller.dart';
import 'package:halaqaty_mobile/features/chat/application/chat_moderation_controller.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_api_client.dart';
import 'package:halaqaty_mobile/features/chat/data/chat_realtime_client.dart';
import 'package:halaqaty_mobile/features/chat/data/pending_message_store.dart';
import 'package:halaqaty_mobile/features/chat/presentation/group_chat_screen.dart';
import 'package:halaqaty_mobile/features/circles/application/circle_detail_controller.dart';
import 'package:halaqaty_mobile/features/circles/data/circle_api_client.dart';
import 'package:halaqaty_mobile/features/circles/presentation/circle_detail_screen.dart';

import '../../helpers/stub_auth_notifier.dart';

const _circleId = 'circle-1';
const _meId = 'me-1';
const _otherId = 'other-1';
const _circleName = 'حلقة الفجر';

ChatMessage _message(
  String id, {
  String content = 'نص',
  String senderId = _otherId,
  String? senderName = 'مريم',
  DateTime? sentAt,
  ChatDeliveryStatus status = ChatDeliveryStatus.delivered,
}) =>
    ChatMessage(
      id: id,
      senderId: senderId,
      circleId: _circleId,
      content: content,
      type: ChatMessageType.text,
      sentAt: sentAt ?? DateTime.utc(2026, 9, 3, 12),
      deliveryStatus: status,
      senderName: senderName,
    );

ChatMessagePage _page(
  List<ChatMessage> messages, {
  bool hasMore = false,
  String? nextBefore,
}) =>
    ChatMessagePage(
        messages: messages, hasMore: hasMore, nextBefore: nextBefore);

/// Authenticated stand-in so the screen can align own bubbles without
/// touching Firebase.
StubAuthNotifier _authenticatedAuth() => StubAuthNotifier(
      initialState: AuthState(
        status: AuthStatus.authenticated,
        sessionId: 'backend-session',
        user: BackendUser(
          id: _meId,
          firebaseUid: 'fb-$_meId',
          preferredLanguage: 'ar',
          createdAt: DateTime.utc(2026, 1, 1),
        ),
      ),
    );

void main() {
  testWidgets(
      'renders message text literally without interpreting markup (RTL + LTR)',
      (tester) async {
    final semantics = tester.ensureSemantics();
    for (final direction in TextDirection.values) {
      final labels = _ChatLabels(direction == TextDirection.rtl);
      final api = _FakeChatApi()
        ..pages.add(_page([
          _message('m1',
              content: "<script>alert('x')</script>", senderName: 'مريم'),
          _message('m2', content: 'b <i>bold</i> & "quotes"', senderName: null),
        ]));
      await _pumpChat(tester, direction: direction, api: api);

      // Raw markup is shown as plain text, never interpreted or stripped.
      expect(find.text("<script>alert('x')</script>"), findsOneWidget);
      expect(find.text('b <i>bold</i> & "quotes"'), findsOneWidget);

      // Sender attribution uses the name, with a neutral fallback when the
      // server omits it; sender ids are never rendered.
      expect(find.bySemanticsLabel('مريم'), findsOneWidget);
      expect(find.bySemanticsLabel(labels.memberFallback), findsOneWidget);
      expect(find.textContaining(_otherId), findsNothing);
    }
    semantics.dispose();
  });

  testWidgets('teacher can delete another member message', (tester) async {
    final message = _message('other-message');
    final api = _FakeChatApi()..pages.add(_page([message]));
    final chat = GroupChatController(
      api,
      () async => (token: 'token', sessionId: 'backend-session', userId: _meId),
      realtime: _FakeChatRealtimeClient(),
    );
    final moderation = ChatModerationController(
      api,
      () async => (token: 'token', sessionId: 'backend-session', userId: _meId),
    );

    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authControllerProvider.overrideWith((_) => _authenticatedAuth()),
          circleMembersProvider(_circleId).overrideWith(
            (_) => Future.value([
              CircleMember(
                userId: _meId,
                displayName: 'Teacher',
                role: CircleRole.teacher,
                joinedAt: DateTime.utc(2026, 1, 1),
              ),
            ]),
          ),
          groupChatControllerProvider(_circleId).overrideWith((_) => chat),
          chatModerationControllerProvider.overrideWith((_) => moderation),
        ],
        child: const MaterialApp(
          home: GroupChatScreen(circleId: _circleId),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.bySemanticsLabel('Delete message'), findsOneWidget);
    await tester.tap(find.bySemanticsLabel('Delete message'));
    await tester.pumpAndSettle();

    expect(api.deletedCircleMessageId, message.id);
    expect(find.text('Message deleted'), findsOneWidget);
  });

  testWidgets(
      'composer validation: empty and overlong disable send, counter and '
      'error label shown, valid text sends once (RTL + LTR)', (tester) async {
    final semantics = tester.ensureSemantics();
    for (final direction in TextDirection.values) {
      final labels = _ChatLabels(direction == TextDirection.rtl);
      final api = _FakeChatApi()
        ..pages.add(_page([]))
        ..nextSendResult = _message('server-1',
            content: 'مرحبا', senderId: _meId, senderName: null);
      await _pumpChat(tester, direction: direction, api: api);
      final field = _editableWithin(labels.composerHint);

      // Empty composer: send is a disabled no-op.
      await tester.tap(find.bySemanticsLabel(labels.send), warnIfMissed: false);
      await tester.pump();
      expect(api.sentContents, isEmpty);

      // Overlong composer: counter plus live-region error label, still no-op.
      await tester.enterText(field, 'x' * 4001);
      await tester.pump();
      expect(find.text(labels.tooLong), findsOneWidget);
      expect(find.text(labels.counter(4001)), findsOneWidget);
      expect(
        tester
            .getSemantics(find.bySemanticsLabel(labels.tooLong))
            .flagsCollection
            .isLiveRegion,
        isTrue,
      );
      await tester.tap(find.bySemanticsLabel(labels.send), warnIfMissed: false);
      await tester.pump();
      expect(api.sentContents, isEmpty);

      // Valid text: sends once, clears the field, and the accepted server
      // projection replaces the optimistic item.
      await tester.enterText(field, 'مرحبا');
      await tester.pump();
      expect(find.text(labels.counter(5)), findsOneWidget);
      await tester.tap(find.bySemanticsLabel(labels.send));
      await tester.pumpAndSettle();

      expect(api.sentContents, ['مرحبا']);
      expect(api.sentIdempotencyKeys, hasLength(1));
      // Exactly one "مرحبا": the bubble. The composer was cleared.
      expect(find.text('مرحبا'), findsOneWidget);
      expect(find.bySemanticsLabel(labels.statusDelivered), findsOneWidget);
      expect(find.text(labels.counter(5)), findsNothing);
    }
    semantics.dispose();
  });

  testWidgets('composer emits typing start and stop for its open circle',
      (tester) async {
    final api = _FakeChatApi()..pages.add(_page([]));
    final realtime = _FakeChatRealtimeClient();
    await _pumpChat(tester,
        direction: TextDirection.ltr, api: api, realtime: realtime);

    final field = _editableWithin('Write a message');
    await tester.enterText(field, 'typing');
    await tester.pump();
    await tester.enterText(field, '');
    await tester.pump();

    expect(realtime.typing, [true, false]);
  });

  testWidgets(
      'composer keeps the draft on failed send and retries from it '
      '(RTL + LTR)', (tester) async {
    for (final direction in TextDirection.values) {
      final labels = _ChatLabels(direction == TextDirection.rtl);
      final api = _FakeChatApi()
        ..pages.add(_page([]))
        ..sendFailure = StateError('send rejected');
      await _pumpChat(tester, direction: direction, api: api);
      final field = _editableWithin(labels.composerHint);

      await tester.enterText(field, 'مرحبا');
      await tester.pump();
      await tester.tap(find.bySemanticsLabel(labels.send));
      await tester.pumpAndSettle();

      // Rejected send: the draft stays in the composer for retry.
      expect(api.sentContents, ['مرحبا']);
      expect(tester.widget<EditableText>(field).controller.text, 'مرحبا');

      // The preserved draft can be retried once the failure clears.
      api.sendFailure = null;
      api.nextSendResult = _message('server-1',
          content: 'مرحبا', senderId: _meId, senderName: null);
      await tester.tap(find.bySemanticsLabel(labels.send));
      await tester.pumpAndSettle();
      expect(api.sentContents, ['مرحبا', 'مرحبا']);
      expect(tester.widget<EditableText>(field).controller.text, isEmpty);
    }
  });

  testWidgets(
      'send is disabled while a send is in flight and the draft is kept '
      'until acceptance (RTL + LTR)', (tester) async {
    final semantics = tester.ensureSemantics();
    for (final direction in TextDirection.values) {
      final labels = _ChatLabels(direction == TextDirection.rtl);
      final sendGate = Completer<ChatMessage>();
      final api = _FakeChatApi()
        ..pages.add(_page([]))
        ..pendingSend = sendGate;
      await _pumpChat(tester, direction: direction, api: api);
      final field = _editableWithin(labels.composerHint);

      await tester.enterText(field, 'مرحبا');
      await tester.pump();
      await tester.tap(find.bySemanticsLabel(labels.send));
      await tester.pump();

      // In flight: the draft is still in the field (clearing waits for
      // acceptance) and the send button is disabled, so a second tap is a
      // no-op instead of a duplicate message.
      expect(tester.widget<EditableText>(field).controller.text, 'مرحبا');
      await tester.tap(find.bySemanticsLabel(labels.send), warnIfMissed: false);
      await tester.pump();
      expect(api.sentContents, hasLength(1));

      sendGate.complete(_message('server-1',
          content: 'مرحبا', senderId: _meId, senderName: null));
      await tester.pumpAndSettle();
      expect(api.sentContents, hasLength(1));
      expect(tester.widget<EditableText>(field).controller.text, isEmpty);
    }
    semantics.dispose();
  });

  testWidgets(
      'keeps deterministic order with newest at the bottom, pages older '
      'history on demand, and dedups repeated realtime events (RTL + LTR)',
      (tester) async {
    final semantics = tester.ensureSemantics();
    for (final direction in TextDirection.values) {
      final labels = _ChatLabels(direction == TextDirection.rtl);
      final api = _FakeChatApi()
        ..pages.add(_page([
          _message('m2',
              content: 'الأحدث', sentAt: DateTime.utc(2026, 9, 3, 12, 5)),
          _message('m1', content: 'الأقدم'),
        ], hasMore: true, nextBefore: 'cursor-1'))
        ..pages.add(_page([
          _message('m0',
              content: 'الأقدم جدًا', sentAt: DateTime.utc(2026, 9, 3, 11)),
        ]));
      final realtime = _FakeChatRealtimeClient();
      final chat = await _pumpChat(tester,
          direction: direction, api: api, realtime: realtime);

      // Newest-first page order renders oldest-at-top, newest-at-bottom.
      expect(
        tester.getCenter(find.text('الأقدم')).dy,
        lessThan(tester.getCenter(find.text('الأحدث')).dy),
      );

      // The pagination trigger loads the older page through the cursor.
      await tester.tap(find.bySemanticsLabel(labels.loadOlder));
      await tester.pumpAndSettle();
      expect(
        api.listCalls.singleWhere((call) => call.before != null).before,
        'cursor-1',
      );
      expect(find.text('الأقدم جدًا'), findsOneWidget);
      // No more pages: the trigger disappears.
      expect(find.bySemanticsLabel(labels.loadOlder), findsNothing);

      // An at-least-once realtime delivery (two distinct event ids, one
      // message id) projects exactly one visible message.
      final live = _message('m3',
          content: 'الرسالة الحية', sentAt: DateTime.utc(2026, 9, 3, 12, 10));
      realtime.emit(ChatMessageEvent(eventId: 'event-1', message: live));
      realtime.emit(ChatMessageEvent(eventId: 'event-2', message: live));
      await tester.pump();
      expect(find.text('الرسالة الحية'), findsOneWidget);
      expect(
        chat.state.messages.where((message) => message.id == 'm3'),
        hasLength(1),
      );
    }
    semantics.dispose();
  });

  testWidgets(
      'labels every delivery status with icon and text, never color alone '
      '(RTL + LTR)', (tester) async {
    final semantics = tester.ensureSemantics();
    for (final direction in TextDirection.values) {
      final labels = _ChatLabels(direction == TextDirection.rtl);
      final statusLabel = {
        ChatDeliveryStatus.pending: labels.statusPending,
        ChatDeliveryStatus.sent: labels.statusSent,
        ChatDeliveryStatus.delivered: labels.statusDelivered,
        ChatDeliveryStatus.read: labels.statusRead,
      };
      final api = _FakeChatApi()
        ..pages.add(_page([
          for (final (index, status) in ChatDeliveryStatus.values.indexed)
            _message('own-$index',
                content: 'رسالة $index',
                senderId: _meId,
                senderName: null,
                status: status),
        ]));
      await _pumpChat(tester, direction: direction, api: api);

      for (final label in statusLabel.values) {
        expect(find.bySemanticsLabel(label), findsOneWidget);
        // Meaning is carried by icon plus text, not color alone.
        expect(
          find.descendant(
            of: find.bySemanticsLabel(label),
            matching: find.byType(Icon),
          ),
          findsOneWidget,
        );
      }
    }
    semantics.dispose();
  });

  testWidgets(
      'deterministic loading, empty, and error states with safe copy '
      '(RTL + LTR)', (tester) async {
    final semantics = tester.ensureSemantics();
    for (final direction in TextDirection.values) {
      final labels = _ChatLabels(direction == TextDirection.rtl);

      // Loading: spinner plus label while the first page is in flight.
      final pendingPage = Completer<ChatMessagePage>();
      final pendingApi = _FakeChatApi()..pendingPage = pendingPage;
      await _pumpChat(tester,
          direction: direction, api: pendingApi, settle: false);
      expect(find.text(labels.loading), findsOneWidget);
      expect(find.byType(CircularProgressIndicator), findsOneWidget);
      pendingPage.complete(_page([_message('m1')]));
      await tester.pumpAndSettle();
      expect(find.text('نص'), findsOneWidget);

      // Empty: deterministic guidance; the composer stays available.
      final emptyApi = _FakeChatApi()..pages.add(_page([]));
      await _pumpChat(tester, direction: direction, api: emptyApi);
      await tester.pumpAndSettle();
      expect(find.text(labels.empty), findsOneWidget);
      expect(find.bySemanticsLabel(labels.composerHint), findsOneWidget);

      // Error: safe localized copy, never the raw failure, with retry.
      // The credentials fake fails the first open entirely (history load
      // AND realtime subscribe), so no live subscription exists; retrying
      // re-opens without the fake-clock-incompatible awaited stream cancel.
      // The third call succeeds, proving retry recovers to ready.
      final failingApi = _FakeChatApi()..pages.add(_page([_message('m1')]));
      var credentialCalls = 0;
      final errorChat = GroupChatController(
        failingApi,
        () async {
          credentialCalls++;
          if (credentialCalls <= 2) {
            throw StateError('auth backend secret');
          }
          return (token: 'token', sessionId: 'backend-session', userId: _meId);
        },
        realtime: _FakeChatRealtimeClient(),
      );
      await _pumpChat(tester,
          direction: direction, api: failingApi, controller: errorChat);
      await tester.pumpAndSettle();
      expect(errorChat.state.status, GroupChatStatus.error);
      expect(find.text(labels.historyError), findsOneWidget);
      expect(find.textContaining('auth backend secret'), findsNothing);
      expect(find.bySemanticsLabel(labels.retry), findsOneWidget);

      await tester.tap(find.bySemanticsLabel(labels.retry));
      await tester.pumpAndSettle();
      expect(errorChat.state.status, GroupChatStatus.ready);
      expect(find.text('نص'), findsOneWidget);
    }
    semantics.dispose();
  });

  testWidgets(
      'aligns own bubbles to the directional end and keeps controls '
      'uncut with 48dp targets (RTL + LTR)', (tester) async {
    final semantics = tester.ensureSemantics();
    for (final direction in TextDirection.values) {
      final labels = _ChatLabels(direction == TextDirection.rtl);
      final api = _FakeChatApi()
        ..pages.add(_page([
          _message('mine', content: 'مني', senderId: _meId, senderName: null),
          _message('theirs', content: 'من مريم'),
        ], hasMore: true, nextBefore: 'cursor-1'));
      await _pumpChat(tester, direction: direction, api: api);

      final own = tester.getCenter(find.text('مني'));
      final other = tester.getCenter(find.text('من مريم'));
      if (direction == TextDirection.rtl) {
        expect(own.dx, lessThan(other.dx),
            reason: 'own messages sit on the directional end (left in RTL)');
      } else {
        expect(own.dx, greaterThan(other.dx),
            reason: 'own messages sit on the directional end (right in LTR)');
      }

      // Interactive controls meet the target size and stay fully on-screen.
      final screen = tester.getSize(find.byType(GroupChatScreen));
      for (final control in [
        find.bySemanticsLabel(labels.send),
        find.bySemanticsLabel(labels.loadOlder),
      ]) {
        final target = tester.getSize(control);
        expect(target.width, greaterThanOrEqualTo(48));
        expect(target.height, greaterThanOrEqualTo(48));
        final rect = tester.getRect(control);
        expect(rect.left, greaterThanOrEqualTo(0));
        expect(rect.top, greaterThanOrEqualTo(0));
        expect(rect.right, lessThanOrEqualTo(screen.width));
        expect(rect.bottom, lessThanOrEqualTo(screen.height));
      }
      final field = tester.getRect(_editableWithin(labels.composerHint));
      expect(field.left, greaterThanOrEqualTo(0));
      expect(field.right, lessThanOrEqualTo(screen.width));
    }
    semantics.dispose();
  });

  testWidgets(
      'terminal send failure keeps the draft visible with edit, discard, and '
      'retry affordances (RTL + LTR)', (tester) async {
    final semantics = tester.ensureSemantics();
    for (final direction in TextDirection.values) {
      final labels = _ChatLabels(direction == TextDirection.rtl);
      final api = _FakeChatApi()
        ..pages.add(_page([]))
        ..sendFailure = const ChatApiException(
            statusCode: 422, code: 'ERR_VALIDATION_FAILED', message: 'invalid');
      final chat = GroupChatController(
        api,
        () async =>
            (token: 'token', sessionId: 'backend-session', userId: _meId),
        realtime: _FakeChatRealtimeClient(),
        pendingStore: PendingMessageStore(_MemorySecureStorage()),
      );
      await _pumpChat(tester, direction: direction, api: api, controller: chat);

      await tester.enterText(_editableWithin(labels.composerHint), 'مسودة');
      await tester.pump();
      await tester.tap(find.bySemanticsLabel(labels.send));
      await tester.pumpAndSettle();

      // FR-008: a terminal failure stays visible for edit/discard; meaning
      // never depends on color alone and controls are labeled. The draft
      // legitimately appears twice: the failed bubble and the kept composer
      // draft (a rejected send never discards user input).
      expect(api.sentContents, ['مسودة']);
      expect(find.text('مسودة'), findsNWidgets(2));
      expect(find.bySemanticsLabel(labels.sendFailed), findsOneWidget);
      expect(find.bySemanticsLabel(labels.editDraft), findsOneWidget);
      expect(find.bySemanticsLabel(labels.discardDraft), findsOneWidget);
      expect(find.bySemanticsLabel(labels.retry), findsOneWidget);

      await tester.tap(find.bySemanticsLabel(labels.discardDraft));
      await tester.pumpAndSettle();
      // Only the composer draft remains after discarding the failed item.
      expect(find.text('مسودة'), findsOneWidget);
    }
    semantics.dispose();
  });

  testWidgets('leaving the screen disposes the circle chat controller',
      (tester) async {
    final api = _FakeChatApi()..pages.add(_page([_message('m1')]));
    final chat = _DisposalTrackingChat(
      api,
      () async => (token: 'token', sessionId: 'backend-session', userId: _meId),
      realtime: _FakeChatRealtimeClient(),
    );
    await _pumpChat(tester,
        direction: TextDirection.ltr, api: api, controller: chat);
    expect(chat.disposed, isFalse);

    // Replace only the screen; the surrounding scope (like the app root)
    // survives, so disposal must come from the provider's own lifecycle
    // instead of the whole container going away.
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          authControllerProvider.overrideWith((_) => _authenticatedAuth()),
          groupChatControllerProvider(_circleId).overrideWith((_) => chat),
        ],
        child: const Directionality(
          textDirection: TextDirection.ltr,
          child: SizedBox.shrink(),
        ),
      ),
    );
    await tester.pump();
    await tester.pump();
    expect(chat.disposed, isTrue);
  });

  testWidgets('circle details navigates into the group chat (RTL + LTR)',
      (tester) async {
    for (final direction in TextDirection.values) {
      final api = _FakeChatApi()..pages.add(_page([_message('m1')]));
      final chat = GroupChatController(
        api,
        () async =>
            (token: 'token', sessionId: 'backend-session', userId: _meId),
        realtime: _FakeChatRealtimeClient(),
      );

      // Unmount the previous direction's tree so this iteration mounts a
      // fresh screen with its own controller (element reuse would skip
      // initState and keep the previous projection).
      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            authControllerProvider.overrideWith((_) => _authenticatedAuth()),
            circleDetailProvider(_circleId).overrideWith(
              (_) => Future.value(_circle()),
            ),
            // Authenticated users make the screen watch members; mock it so
            // no unmocked provider (Firebase) is ever consulted.
            circleMembersProvider(_circleId)
                .overrideWith((_) => Future.value(const <CircleMember>[])),
            groupChatControllerProvider(_circleId).overrideWith((_) => chat),
          ],
          child: MaterialApp(
            home: Directionality(
              textDirection: direction,
              child: CircleDetailScreen(circleId: _circleId),
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();

      // The chat tile sits below the fold in the details ListView.
      await tester.ensureVisible(find.byKey(const Key('openCircleChat')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('openCircleChat')));
      await tester.pumpAndSettle();

      expect(find.byType(GroupChatScreen), findsOneWidget);
      expect(find.text(_circleName), findsOneWidget);
      expect(find.text('نص'), findsOneWidget);
      // Composer presence at widget level; its Semantics labels are covered
      // exhaustively (both directions) in the composer validation test —
      // pushed-route semantics flushing is unreliable under the fake clock.
      expect(find.byType(TextField), findsOneWidget);
    }
  });
}

/// Pumps the group chat screen with a real controller over fake transports.
/// Returns the controller for state assertions; disposal is owned by the
/// ProviderScope, so the test never disposes it a second time.
///
/// [settle] is false for states with a continuously animating indicator
/// (loading), where `pumpAndSettle` would time out on the spinner.
///
/// A placeholder tree is pumped first: consecutive pumps of an identical
/// widget structure would otherwise reuse the element tree, skip initState,
/// and keep projecting the previous controller's state.
Future<GroupChatController> _pumpChat(
  WidgetTester tester, {
  required TextDirection direction,
  required _FakeChatApi api,
  _FakeChatRealtimeClient? realtime,
  bool settle = true,
  GroupChatController? controller,
}) async {
  final chatRealtime = realtime ?? _FakeChatRealtimeClient();
  final chat = controller ??
      GroupChatController(
        api,
        () async =>
            (token: 'token', sessionId: 'backend-session', userId: _meId),
        realtime: chatRealtime,
      );
  await tester.pumpWidget(const SizedBox.shrink());
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        authControllerProvider.overrideWith((_) => _authenticatedAuth()),
        groupChatControllerProvider(_circleId).overrideWith((_) => chat),
      ],
      child: MaterialApp(
        home: Directionality(
          textDirection: direction,
          child: GroupChatScreen(
            circleId: _circleId,
            circleName: _circleName,
          ),
        ),
      ),
    ),
  );
  if (settle) {
    await tester.pumpAndSettle();
  } else {
    await tester.pump();
  }
  return chat;
}

/// Locates the editable text of the composer field labeled via Semantics.
Finder _editableWithin(String label) => find.descendant(
      of: find.bySemanticsLabel(label),
      matching: find.byType(EditableText),
    );

/// Records disposal so tests can prove the provider lifecycle releases the
/// per-circle controller when the screen goes away.
class _DisposalTrackingChat extends GroupChatController {
  _DisposalTrackingChat(super.api, super.credentials,
      {required super.realtime});

  bool disposed = false;

  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }
}

CircleResponse _circle() => CircleResponse(
      id: _circleId,
      name: _circleName,
      inviteCode: 'HLQ-7X2K',
      inviteLink: 'https://halaqaty.app/join/HLQ-7X2K',
      createdAt: DateTime.utc(2026, 8, 1),
    );

class _ListCall {
  const _ListCall({this.before});
  final String? before;
}

/// Fake REST boundary: canned pages, captured sends, injectable failures.
class _FakeChatApi extends ChatApiClient {
  _FakeChatApi() : super(Dio());

  final pages = <ChatMessagePage>[];
  final listCalls = <_ListCall>[];
  Object? listFailure;

  /// When set, the next list call waits on this completer (loading state).
  Completer<ChatMessagePage>? pendingPage;

  final sentIdempotencyKeys = <String>[];
  final sentContents = <String>[];
  ChatMessage? nextSendResult;
  Object? sendFailure;
  String? deletedCircleMessageId;

  /// When set, the next send call waits on this completer (in-flight send).
  Completer<ChatMessage>? pendingSend;

  @override
  Future<ChatMessagePage> listMessages({
    required String token,
    required String sessionId,
    required String circleId,
    int? limit,
    String? before,
  }) async {
    listCalls.add(_ListCall(before: before));
    if (listFailure != null) throw listFailure!;
    final pending = pendingPage;
    if (pending != null) {
      pendingPage = null;
      return pending.future;
    }
    if (pages.isEmpty) {
      throw StateError('No canned page for list call');
    }
    return pages.removeAt(0);
  }

  @override
  Future<ChatMessage> sendTextMessage({
    required String token,
    required String sessionId,
    required String circleId,
    required String content,
    required String idempotencyKey,
    String? replyToId,
  }) async {
    sentIdempotencyKeys.add(idempotencyKey);
    sentContents.add(content);
    if (sendFailure != null) throw sendFailure!;
    final pending = pendingSend;
    if (pending != null) {
      pendingSend = null;
      return pending.future;
    }
    return nextSendResult!;
  }

  @override
  Future<void> deleteCircleMessage({
    required String token,
    required String sessionId,
    required String circleId,
    required String messageId,
  }) async {
    deletedCircleMessageId = messageId;
  }
}

/// Fake realtime boundary: the controller subscribes to the broadcast stream.
class _FakeChatRealtimeClient
    implements ChatRealtimeClient, ChatRealtimePresenceClient {
  final StreamController<ChatRealtimeEvent> _events =
      StreamController<ChatRealtimeEvent>.broadcast(sync: true);

  int circleChatEventsCalls = 0;
  final typing = <bool>[];

  void emit(ChatRealtimeEvent event) => _events.add(event);

  @override
  Stream<ChatRealtimeEvent> circleChatEvents(
    String circleId, {
    required String token,
    required String backendSessionId,
  }) {
    circleChatEventsCalls++;
    return _events.stream;
  }

  @override
  Stream<ChatRealtimeEvent> directChatEvents(String peerId,
          {required String token, required String backendSessionId}) =>
      const Stream.empty();

  @override
  Future<void> sendTyping(
      {String? circleId, String? dmPeerId, required bool isTyping}) async {
    typing.add(isTyping);
  }

  @override
  Future<void> dispose() => _events.close();
}

/// Arabic-first labels pinned for the T034 group chat screen; the LTR column
/// mirrors the bilingual pattern of `session_ui_labels.dart`.
class _ChatLabels {
  const _ChatLabels(this.rtl);

  final bool rtl;

  String get loading =>
      rtl ? 'جارٍ تحميل المحادثة...' : 'Loading chat history...';
  String get empty => rtl
      ? 'لا توجد رسائل بعد؛ ابدأ المحادثة'
      : 'No messages yet; start the conversation';
  String get historyError =>
      rtl ? 'تعذر تحميل المحادثة' : 'Could not load chat history';
  String get retry => rtl ? 'إعادة المحاولة' : 'Retry';
  String get loadOlder => rtl ? 'تحميل الرسائل الأقدم' : 'Load older messages';
  String get composerHint => rtl ? 'اكتب رسالة' : 'Write a message';
  String get send => rtl ? 'إرسال' : 'Send';
  String get tooLong => rtl
      ? 'الرسالة طويلة جدًا؛ الحد الأقصى 4000 حرف'
      : 'Message is too long; limit is 4000 characters';
  String get statusPending => rtl ? 'قيد الإرسال' : 'Sending';
  String get statusSent => rtl ? 'أُرسلت' : 'Sent';
  String get statusDelivered => rtl ? 'تم التسليم' : 'Delivered';
  String get statusRead => rtl ? 'تمت القراءة' : 'Read';
  String get memberFallback => rtl ? 'عضو' : 'Member';
  String get sendFailed => rtl ? 'فشل الإرسال' : 'Send failed';
  String get editDraft => rtl ? 'تعديل الرسالة' : 'Edit message';
  String get discardDraft => rtl ? 'تجاهل الرسالة' : 'Discard message';

  String counter(int length) => '$length/4000';
}

/// In-memory secure storage so terminal drafts persist like production.
class _MemorySecureStorage extends FlutterSecureStorage {
  final values = <String, String>{};

  @override
  Future<String?> read(
          {required String key,
          IOSOptions? iOptions,
          AndroidOptions? aOptions,
          WebOptions? webOptions,
          MacOsOptions? mOptions,
          LinuxOptions? lOptions,
          WindowsOptions? wOptions}) async =>
      values[key];

  @override
  Future<void> write(
      {required String key,
      required String? value,
      IOSOptions? iOptions,
      AndroidOptions? aOptions,
      WebOptions? webOptions,
      MacOsOptions? mOptions,
      LinuxOptions? lOptions,
      WindowsOptions? wOptions}) async {
    if (value == null) {
      values.remove(key);
    } else {
      values[key] = value;
    }
  }

  @override
  Future<void> delete(
      {required String key,
      IOSOptions? iOptions,
      AndroidOptions? aOptions,
      WebOptions? webOptions,
      MacOsOptions? mOptions,
      LinuxOptions? lOptions,
      WindowsOptions? wOptions}) async {
    values.remove(key);
  }
}
