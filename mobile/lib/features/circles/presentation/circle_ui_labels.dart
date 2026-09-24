const circleCancelLabel = 'إلغاء';
const circleMutationErrorLabel = 'تعذر إكمال الطلب. حاول مرة أخرى';

/// Localized display name for a circle language code so user-facing copy never
/// exposes the raw database value. Unknown codes fall back to the code itself.
String circleLanguageLabel(String code, bool rtl) => switch (code) {
      'ar' => rtl ? 'العربية' : 'Arabic',
      'en' => rtl ? 'الإنجليزية' : 'English',
      _ => code,
    };

/// Localized display name for a circle audience restriction. The single source
/// for the restriction→copy mapping (mirrors [CircleDetailLabels]); unknown
/// values render as "unspecified".
String circleAudienceLabel(String restriction, bool rtl) =>
    switch (restriction) {
      'male' => rtl ? CircleDetailLabels.maleAr : CircleDetailLabels.maleEn,
      'female' =>
        rtl ? CircleDetailLabels.femaleAr : CircleDetailLabels.femaleEn,
      'mixed' => rtl ? CircleDetailLabels.mixedAr : CircleDetailLabels.mixedEn,
      _ => rtl
          ? CircleDetailLabels.unspecifiedAr
          : CircleDetailLabels.unspecifiedEn,
    };

abstract final class CircleDetailLabels {
  static const titleAr = 'تفاصيل الحلقة';
  static const titleEn = 'Circle details';
  static const archivedAr = 'هذه الحلقة مؤرشفة ومتاحة للقراءة فقط';
  static const archivedEn = 'This circle is archived and read-only.';
  static const capacityAr = 'السعة القصوى';
  static const capacityEn = 'Maximum capacity';
  static const visibilityAr = 'نوع الحلقة';
  static const visibilityEn = 'Circle visibility';
  static const audienceAr = 'الفئة المستهدفة';
  static const audienceEn = 'Audience';
  static const languageAr = 'اللغة';
  static const languageEn = 'Language';
  static const rulesAr = 'قواعد الحلقة';
  static const rulesEn = 'Circle rules';
  static const membersAr = 'الأعضاء';
  static const membersEn = 'Members';
  static const chatAr = 'المحادثة';
  static const chatEn = 'Chat';
  static const scheduleAr = 'المواعيد';
  static const scheduleEn = 'Schedule';
  static const manageAr = 'إدارة الحلقة';
  static const manageEn = 'Manage circle';
  static const archiveAr = 'أرشفة الحلقة';
  static const archiveEn = 'Archive circle';
  static const loadErrorAr = 'تعذر تحميل تفاصيل الحلقة';
  static const loadErrorEn = 'Could not load circle details';
  static const goneAr = 'هذه الحلقة لم تعد متاحة';
  static const goneEn = 'This circle is no longer available';
  static const goneHelpAr = 'إعادة المحاولة غير متاحة. عُد إلى الحلقات.';
  static const goneHelpEn = 'Retry is unavailable. Return to circles.';
  static const exitAr = 'العودة إلى الحلقات';
  static const exitEn = 'Back to circles';
  static const retryAr = 'إعادة المحاولة';
  static const retryEn = 'Retry';
  static const privateAr = 'خاصة';
  static const privateEn = 'Private';
  static const publicAr = 'عامة';
  static const publicEn = 'Public';
  static const maleAr = 'ذكور';
  static const maleEn = 'Male';
  static const femaleAr = 'إناث';
  static const femaleEn = 'Female';
  static const mixedAr = 'مختلطة';
  static const mixedEn = 'Mixed';
  static const unspecifiedAr = 'غير محدد';
  static const unspecifiedEn = 'Unspecified';
}
