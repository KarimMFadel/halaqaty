# Documentation Synchronization Guide | دليل مزامنة الوثائق

> Bilingual policy for Halaqaty docs

---

## English

### Current Policy (Updated)

Arabic docs are now centralized under:

- `docs/management/arabic/`

Arabic coverage is **business/product-facing only**.
Technical deep-dive docs are maintained in English only.

### Document map

| Type | English | Arabic |
|---|---|---|
| Project intro | [`README.md`](../../../README.md) | [`README_AR.md`](README_AR.md) |
| Product requirements | [`PRD.md`](../product/PRD.md) | [`PRD_AR.md`](PRD_AR.md) |
| Master plan (business) | [`PROJECT_PLAN.md`](../planning/PROJECT_PLAN.md) | [`PLAN_AR.md`](PLAN_AR.md) |
| Feature board | [`FEATURES.md`](../product/FEATURES.md) | [`FEATURES_AR.md`](FEATURES_AR.md) |
| User journeys | [`JOURNEY.md`](../product/JOURNEY.md) | [`JOURNEY_AR.md`](JOURNEY_AR.md) |
| Roles and business permissions | [`ROLES.md`](../product/ROLES.md) | [`ROLES_AR.md`](ROLES_AR.md) |
| Frozen MVP decisions | [`MVP_DECISION_REGISTER.md`](../product/MVP_DECISION_REGISTER.md) | [`DECISIONS_AR.md`](DECISIONS_AR.md) |
| Market and competitor research | [`QURAN_MEMORIZATION_COMPETITOR_ANALYSIS.md`](../business/QURAN_MEMORIZATION_COMPETITOR_ANALYSIS.md) | [`COMPETITOR_ANALYSIS_AR.md`](COMPETITOR_ANALYSIS_AR.md) |

### Source-of-truth rules

- **Business/product decisions:** English primary, Arabic adapted for user-facing clarity.
- **Technical implementation:** English only.
- **Arabic terminology:** natural Quran/Islamic wording for user-facing concepts.

### Update rules

1. If you change `PRD.md`, `PROJECT_PLAN.md`, `FEATURES.md`, `JOURNEY.md`, `ROLES.md`, `MVP_DECISION_REGISTER.md`, or the competitor analysis, update its Arabic mirror in `docs/management/arabic/`.
2. If you change technical docs (`ARCHITECTURE.md`, `DEPLOYMENT.md`), no Arabic mirror is required.
3. Summarize business intent in Arabic; do not copy technical implementation details, APIs, infrastructure, or operational runbooks.
4. Keep links valid after any file move.

---

## العربية

### السياسة الحالية (محدّثة)

تم تجميع الملفات العربية تحت:

- `docs/management/arabic/`

التغطية العربية تركز على **المحتوى التجاري/المنتجي** فقط،
أما التفاصيل التقنية العميقة فتبقى في الوثائق الإنجليزية.

### خريطة المستندات

| النوع | الإنجليزية | العربية |
|---|---|---|
| تعريف المشروع | [`README.md`](../../../README.md) | [`README_AR.md`](README_AR.md) |
| وثيقة متطلبات المنتج | [`PRD.md`](../product/PRD.md) | [`PRD_AR.md`](PRD_AR.md) |
| الخطة الرئيسية (تجارية) | [`PROJECT_PLAN.md`](../planning/PROJECT_PLAN.md) | [`PLAN_AR.md`](PLAN_AR.md) |
| لوحة المميزات | [`FEATURES.md`](../product/FEATURES.md) | [`FEATURES_AR.md`](FEATURES_AR.md) |
| رحلات المستخدمين | [`JOURNEY.md`](../product/JOURNEY.md) | [`JOURNEY_AR.md`](JOURNEY_AR.md) |
| الأدوار والصلاحيات التجارية | [`ROLES.md`](../product/ROLES.md) | [`ROLES_AR.md`](ROLES_AR.md) |
| قرارات MVP المحسومة | [`MVP_DECISION_REGISTER.md`](../product/MVP_DECISION_REGISTER.md) | [`DECISIONS_AR.md`](DECISIONS_AR.md) |
| السوق وتحليل المنافسين | [`QURAN_MEMORIZATION_COMPETITOR_ANALYSIS.md`](../business/QURAN_MEMORIZATION_COMPETITOR_ANALYSIS.md) | [`COMPETITOR_ANALYSIS_AR.md`](COMPETITOR_ANALYSIS_AR.md) |

### قواعد المزامنة

1. عند تعديل `PRD.md` أو `PROJECT_PLAN.md` أو `FEATURES.md` أو `JOURNEY.md` أو `ROLES.md` أو `MVP_DECISION_REGISTER.md` أو تحليل المنافسين، يجب تحديث مرآته العربية في `docs/management/arabic/`.
2. عند تعديل الوثائق التقنية (`ARCHITECTURE.md` و `DEPLOYMENT.md`) لا حاجة لمرآة عربية.
3. لخّص المقصد التجاري بالعربية، ولا تنسخ تفاصيل التنفيذ أو واجهات البرمجة أو البنية التحتية أو أدلة التشغيل.
4. تأكد من سلامة الروابط بعد أي نقل للملفات.

