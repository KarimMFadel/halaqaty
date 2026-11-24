import 'package:flutter_test/flutter_test.dart';
import 'package:halaqaty_mobile/features/notifications/presentation/notification_copy.dart';

void main() {
  const fallbackTitle = 'Server generic title';
  const fallbackBody = 'Server generic body';

  test('the full contract type matrix resolves Arabic-first copy', () {
    for (final type in const [
      'session_reminder',
      'group_message',
      'private_message',
      'grade_posted',
      'circle_invitation',
      'queue_next_soon',
      'queue_your_turn',
      'student_joined_session',
      'student_joined_circle',
      'circle_archived',
      'session_started',
      'session_ended',
    ]) {
      final copy = resolveNotificationCopy(
        type: type,
        sourceRefType: 'session',
        fallbackTitle: fallbackTitle,
        fallbackBody: fallbackBody,
        isArabic: true,
      );
      expect(copy.title, isNotEmpty, reason: 'type $type has Arabic copy');
      expect(copy.body, isNotEmpty);
      expect(copy.title, isNot(fallbackTitle),
          reason: 'Arabic display copy is primary, not the server fallback');
    }
  });

  test('specific Arabic copy is keyed by type (sample values)', () {
    expect(
      resolveNotificationCopy(
        type: 'queue_your_turn',
        sourceRefType: 'queue',
        fallbackTitle: fallbackTitle,
        fallbackBody: fallbackBody,
        isArabic: true,
      ).title,
      'جاء دورك في التلاوة',
    );
    expect(
      resolveNotificationCopy(
        type: 'student_joined_circle',
        sourceRefType: 'circle',
        fallbackTitle: fallbackTitle,
        fallbackBody: fallbackBody,
        isArabic: true,
      ).title,
      'انضم عضو جديد إلى حلقتك',
    );
  });

  test('English rendering uses the English fallback strings', () {
    final copy = resolveNotificationCopy(
      type: 'session_started',
      sourceRefType: 'session',
      fallbackTitle: fallbackTitle,
      fallbackBody: fallbackBody,
      isArabic: false,
    );
    expect(copy.title, 'The session started');
    expect(copy.body, 'The circle session has started; you can join.');
  });

  test(
    'unknown/future types fall back to the server title/body verbatim',
    () {
      final copy = resolveNotificationCopy(
        type: 'future_event',
        sourceRefType: 'session',
        fallbackTitle: fallbackTitle,
        fallbackBody: fallbackBody,
        isArabic: true,
      );
      expect(copy.title, fallbackTitle);
      expect(copy.body, fallbackBody);
    },
  );
}
