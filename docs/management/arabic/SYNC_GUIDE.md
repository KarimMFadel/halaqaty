# Documentation Synchronization Guide | دليل مزامنة الوثائق

> Bilingual policy for Halaqaty docs

---

## English

### Current Policy (Updated)

Arabic docs are now centralized under:

- `docs/arabic/`

Arabic coverage is **business/product-facing only**.
Technical deep-dive docs are maintained in English only.

### Document map

| Type | English | Arabic |
|---|---|---|
| Project intro | [`README.md`](../README.md) | [`docs/arabic/README_AR.md`](arabic/README_AR.md) |
| Product requirements | [`PRD.md`](PRD.md) | [`docs/arabic/PRD_AR.md`](arabic/PRD_AR.md) |
| Master plan (business) | [`PLAN.md`](PLAN.md) | [`docs/arabic/PLAN_AR.md`](arabic/PLAN_AR.md) |
| Feature board | [`FEATURES.md`](FEATURES.md) | [`docs/arabic/FEATURES_AR.md`](arabic/FEATURES_AR.md) |
| Technical architecture | [`ARCHITECTURE.md`](ARCHITECTURE.md) | — |
| Deployment strategy | [`DEPLOYMENT.md`](DEPLOYMENT.md) | — |

### Source-of-truth rules

- **Business/product decisions:** English primary, Arabic adapted for user-facing clarity.
- **Technical implementation:** English only.
- **Arabic terminology:** natural Quran/Islamic wording for user-facing concepts.

### Update rules

1. If you change `PRD.md`, `PLAN.md`, or `FEATURES.md`, update Arabic mirrors in `docs/arabic/`.
2. If you change technical docs (`ARCHITECTURE.md`, `DEPLOYMENT.md`), no Arabic mirror is required.
3. Keep links valid after any file move.

---

## العربية

### السياسة الحالية (محدّثة)

تم تجميع الملفات العربية تحت:

- `docs/arabic/`

التغطية العربية تركز على **المحتوى التجاري/المنتجي** فقط،
أما التفاصيل التقنية العميقة فتبقى في الوثائق الإنجليزية.

### خريطة المستندات

| النوع | الإنجليزية | العربية |
|---|---|---|
| تعريف المشروع | [`README.md`](../README.md) | [`docs/arabic/README_AR.md`](arabic/README_AR.md) |
| وثيقة متطلبات المنتج | [`PRD.md`](PRD.md) | [`docs/arabic/PRD_AR.md`](arabic/PRD_AR.md) |
| الخطة الرئيسية (تجارية) | [`PLAN.md`](PLAN.md) | [`docs/arabic/PLAN_AR.md`](arabic/PLAN_AR.md) |
| لوحة المميزات | [`FEATURES.md`](FEATURES.md) | [`docs/arabic/FEATURES_AR.md`](arabic/FEATURES_AR.md) |
| المعمارية التقنية | [`ARCHITECTURE.md`](ARCHITECTURE.md) | — |
| استراتيجية النشر | [`DEPLOYMENT.md`](DEPLOYMENT.md) | — |

### قواعد المزامنة

1. عند تعديل `PRD.md` أو `PLAN.md` أو `FEATURES.md` يجب تحديث النسخ العربية في `docs/arabic/`.
2. عند تعديل الوثائق التقنية (`ARCHITECTURE.md` و `DEPLOYMENT.md`) لا حاجة لمرآة عربية.
3. تأكد من سلامة الروابط بعد أي نقل للملفات.
