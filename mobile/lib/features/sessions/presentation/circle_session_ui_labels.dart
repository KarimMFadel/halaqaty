/// Paired Arabic/English labels for the circle detail sessions section
/// (Arabic-first, mirroring `CircleDetailLabels`).
abstract final class CircleSessionUiLabels {
  static const sectionTitleAr = 'الجلسات';
  static const sectionTitleEn = 'Sessions';
  static const createAr = 'إنشاء جلسة';
  static const createEn = 'Create session';
  static const scheduledAr = 'جلسة مجدولة';
  static const scheduledEn = 'Scheduled session';
  static const activeAr = 'جلسة نشطة';
  static const activeEn = 'Active session';
  static const emptyTitleAr = 'لا توجد جلسات حالياً';
  static const emptyTitleEn = 'No active sessions right now';
  static const emptyManagerHintAr = 'أنشئ جلسة لينضم إليها الطلاب';
  static const emptyManagerHintEn = 'Create a session so students can join';
  static const emptyMemberHintAr = 'سيُنشئ المعلم أو المشرف الجلسة عند الحاجة';
  static const emptyMemberHintEn =
      'Your teacher or supervisor will create a session when needed';
  static const loadErrorAr = 'تعذر تحميل الجلسات';
  static const loadErrorEn = 'Could not load sessions';
  static const offlineAr =
      'لا يوجد اتصال حاليًا. تُعرض آخر قائمة جلسات تم تحميلها والإجراءات متوقفة مؤقتًا';
  static const offlineEn = 'You are offline. Showing the last loaded sessions; '
      'actions are paused';
  static const permissionAr = 'لا تملك صلاحية عرض جلسات هذه الحلقة';
  static const permissionEn =
      'You do not have permission to view this circle’s sessions';
  static const retryAr = 'إعادة المحاولة';
  static const retryEn = 'Retry';
  static const createFailedAr = 'تعذر إنشاء الجلسة. حاول مرة أخرى';
  static const createFailedEn = 'Could not create the session. Try again';
  static const archivedHintAr = 'الحلقة مؤرشفة؛ الجلسات متاحة للعرض فقط';
  static const archivedHintEn =
      'This circle is archived; sessions are read-only';

  static String participantsAr(int count) {
    if (count == 0) return 'لا مشاركين';
    if (count == 1) return 'مشارك واحد';
    if (count == 2) return 'مشاركان';
    if (count <= 10) return '$count مشاركين';
    return '$count مشاركاً';
  }

  static String participantsEn(int count) =>
      count == 1 ? '1 participant' : '$count participants';
}
