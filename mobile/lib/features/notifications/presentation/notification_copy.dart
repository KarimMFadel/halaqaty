/// Display-copy resolver for notification history rows.
///
/// Decision (Karim, 2026-10-25, MVP register): notification copy is
/// displayed Arabic-first on the client, keyed off the notification `type`
/// plus `source_ref.type`; the server `title`/`body` are generic
/// lock-screen-safe FALLBACKS only (also used for unknown or future types
/// so the list stays honest instead of blank).
///
/// Today every event type in the contract enum maps to exactly one
/// `source_ref.type` (per specs/008-notification-system contracts and the
/// backend projections), so the table is type-keyed. `sourceRefType` is
/// kept in the resolution signature deliberately: if a future type ever
/// spans sources, the pair key plugs in without changing callers.
///
/// No ARB/l10n framework exists yet (F-019 defers one): this typed resolver
/// must migrate to generated .arb strings when that framework lands. The
/// keys below are the wire `type` values straight from openapi.yaml, so the
/// migration stays mechanical.
library;

/// Resolved display copy for one history row.
class NotificationCopy {
  const NotificationCopy({required this.title, required this.body});

  final String title;
  final String body;
}

/// Arabic-first copy keyed by wire `type`; English fallback strings in
/// the parallel table below.
const _arabicByType = {
  'session_reminder': (
    'تذكير بجلسة قادمة',
    'ستبدأ الجلسة قريباً حسب جدول الحلقة.'
  ),
  'group_message': (
    'رسالة جديدة في مجموعة',
    'توجد رسالة جديدة في محادثة الحلقة.'
  ),
  'private_message': ('رسالة خاصة جديدة', 'توجد رسالة خاصة جديدة في محادثتك.'),
  'grade_posted': ('نُشرت نتيجة التلاوة', 'أصبحت نتائج تلاوتك متاحة للاطلاع.'),
  'circle_invitation': ('دعوة لحلقة', 'تصل إليك دعوة للانضمام إلى حلقة.'),
  'queue_next_soon': (
    'اقترب دورك في التلاوة',
    'أنت التالي في دور التلاوة؛ استعد.'
  ),
  'queue_your_turn': (
    'جاء دورك في التلاوة',
    'اختيارك أنت للتلاوة في هذا الدور؛ كن مستعداً.'
  ),
  'student_joined_session': (
    'انضم عضو إلى الجلسة',
    'انضم أحد الطلاب إلى جلسة نشطة.'
  ),
  'student_joined_circle': (
    'انضم عضو جديد إلى حلقتك',
    'انضم طالب إلى الحلقة التي تديرها.'
  ),
  'circle_archived': (
    'أُرشفت الحلقة',
    'تم أرشفة إحدى حلقاتك وتوقفت عن النشاط.'
  ),
  'session_started': ('بدأت الجلسة', 'بدأت جلسة الحلقة الآن؛ يمكنك الانضمام.'),
  'session_ended': ('انتهت الجلسة', 'انتهت جلسة الحلقة.'),
};

const _englishByType = {
  'session_reminder': (
    'Upcoming session reminder',
    'The circle session starts soon per the schedule.'
  ),
  'group_message': (
    'New group message',
    'A new message arrived in a circle conversation.'
  ),
  'private_message': (
    'New private message',
    'A new private message arrived in your conversation.'
  ),
  'grade_posted': (
    'Evaluation posted',
    'Your recitation evaluation is now available.'
  ),
  'circle_invitation': (
    'Circle invitation',
    'You were invited to join a circle.'
  ),
  'queue_next_soon': (
    'Your turn is coming',
    'You are next in the recitation queue; get ready.'
  ),
  'queue_your_turn': ('Your turn!', 'You are the selected reciter this round.'),
  'student_joined_session': (
    'A member joined the session',
    'A student joined an active session.'
  ),
  'student_joined_circle': (
    'A member joined your circle',
    'A student joined the circle you manage.'
  ),
  'circle_archived': (
    'Circle archived',
    'One of your circles was archived and became inactive.'
  ),
  'session_started': (
    'The session started',
    'The circle session has started; you can join.'
  ),
  'session_ended': ('The session ended', 'The circle session has ended.'),
};

/// Resolves display copy for one history row.
///
/// [type] is the wire value; unknown values (future events) render the
/// server's generic `fallbackTitle`/`fallbackBody` verbatim.
NotificationCopy resolveNotificationCopy({
  required String type,
  required String sourceRefType,
  required String fallbackTitle,
  required String fallbackBody,
  required bool isArabic,
}) {
  final typed = isArabic ? _arabicByType[type] : _englishByType[type];
  if (typed == null) {
    return NotificationCopy(title: fallbackTitle, body: fallbackBody);
  }
  return NotificationCopy(title: typed.$1, body: typed.$2);
}
