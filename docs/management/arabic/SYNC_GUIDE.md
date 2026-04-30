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
| Project intro | [`README.md`](../../../README.md) | [`docs/management/README_AR.md`](README_AR.md) |
| Product requirements | [`PRD.md`](../product/PRD.md) | [`docs/management/PRD_AR.md`](PRD_AR.md) |
| Master plan (business) | [`PROJECT_PLAN.md`](../planning/PROJECT_PLAN.md) | [`docs/management/PLAN_AR.md`](PLAN_AR.md) |
| Feature board | [`FEATURES.md`](../product/FEATURES.md) | [`docs/management/FEATURES_AR.md`](FEATURES_AR.md) |
| Technical architecture | [`ARCHITECTURE.md`](../../engineering/architecture/ARCHITECTURE.md) | — |
| Deployment strategy | [`DEPLOYMENT.md`](../../engineering/deployment/DEPLOYMENT.md) | — |

### Source-of-truth rules

- **Business/product decisions:** English primary, Arabic adapted for user-facing clarity.
- **Technical implementation:** English only.
- **Arabic terminology:** natural Quran/Islamic wording for user-facing concepts.

### Update rules

1. If you change `PRD.md`, `PROJECT_PLAN.md`, or `FEATURES.md`, update Arabic mirrors in `docs/management/arabic/`.
2. If you change technical docs (`ARCHITECTURE.md`, `DEPLOYMENT.md`), no Arabic mirror is required.
3. Keep links valid after any file move.

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
| تعريف المشروع | [`README.md`](../../../README.md) | [`docs/management/README_AR.md`](README_AR.md) |
| وثيقة متطلبات المنتج | [`PRD.md`](../product/PRD.md) | [`docs/management/PRD_AR.md`](PRD_AR.md) |
| الخطة الرئيسية (تجارية) | [`PROJECT_PLAN.md`](../planning/PROJECT_PLAN.md) | [`docs/management/PLAN_AR.md`](PLAN_AR.md) |
| لوحة المميزات | [`FEATURES.md`](../product/FEATURES.md) | [`docs/management/FEATURES_AR.md`](FEATURES_AR.md) |
| المعمارية التقنية | [`ARCHITECTURE.md`](../../engineering/architecture/ARCHITECTURE.md) | — |
| استراتيجية النشر | [`DEPLOYMENT.md`](../../engineering/deployment/DEPLOYMENT.md) | — |

### قواعد المزامنة

1. عند تعديل `PRD.md` أو `PROJECT_PLAN.md` أو `FEATURES.md` يجب تحديث النسخ العربية في `docs/management/arabic/`.
2. عند تعديل الوثائق التقنية (`ARCHITECTURE.md` و `DEPLOYMENT.md`) لا حاجة لمرآة عربية.
3. تأكد من سلامة الروابط بعد أي نقل للملفات.

