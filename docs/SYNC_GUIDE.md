# Documentation Synchronization Guide | دليل مزامنة الوثائق

> **Bilingual Document | وثيقة ثنائية اللغة**

---

## English Section

### Overview

All Halaqaty documentation exists in two parallel versions:
- **English** (`.md` files, e.g., `PLAN.md`)
- **Arabic** (`_AR.md` files, e.g., `PLAN_AR.md`)

This guide explains the synchronization policy and responsibilities for keeping both versions aligned.

### Document Pairs

| English Document | Arabic Document | Description |
|-----------------|----------------|-------------|
| [`../README.md`](../README.md) | [`README_AR.md`](README_AR.md) | Project introduction |
| [`PLAN.md`](PLAN.md) | [`PLAN_AR.md`](PLAN_AR.md) | Master project plan |
| [`FEATURES.md`](FEATURES.md) | [`FEATURES_AR.md`](FEATURES_AR.md) | Feature specifications |
| [`ARCHITECTURE.md`](ARCHITECTURE.md) | [`ARCHITECTURE_AR.md`](ARCHITECTURE_AR.md) | Technical architecture |
| [`DEPLOYMENT.md`](DEPLOYMENT.md) | [`DEPLOYMENT_AR.md`](DEPLOYMENT_AR.md) | Deployment strategy |
| [`SYNC_GUIDE.md`](SYNC_GUIDE.md) | *(this file is bilingual)* | This guide |

### Source of Truth Rules

| Content Type | Source of Truth | Reason |
|-------------|----------------|--------|
| Technical terms (API names, library names, code) | 🇬🇧 English | Technical precision; code is in English |
| User-facing Quran/Islamic terminology | 🇸🇦 Arabic | Arabic is the authentic source for Quranic terms |
| Feature decisions and status | 🇬🇧 English | Primary language for team decisions |
| UI text for Arabic users | 🇸🇦 Arabic | Arabic speakers are the primary users |
| Code examples and configuration | 🇬🇧 English only | No need to duplicate code in Arabic |

### Update Policy

**When you update an English document:**
1. Make your changes to the English `.md` file
2. Open the corresponding `_AR.md` file
3. Apply the equivalent changes in Arabic
4. Commit both files together in the same Git commit
5. PR description must mention which document pairs were updated

**When you update an Arabic document:**
1. Make your changes to the Arabic `_AR.md` file
2. Open the corresponding English `.md` file
3. Apply the equivalent changes in English
4. Commit both files together in the same Git commit

**Commit message convention:**
```
docs: update [PLAN|FEATURES|ARCHITECTURE|DEPLOYMENT] (EN+AR)
```

### Translation Quality Standards

- **Do NOT use machine translation** (Google Translate, DeepL) for Arabic documents
- Arabic must be natural, professional Modern Standard Arabic (فصحى معاصرة)
- Use authentic Islamic/Quranic terminology where applicable
- Technical terms (LiveKit, Docker, WebSocket, etc.) remain in English within Arabic text
- Numbers in Arabic documents may use Western numerals (1, 2, 3) or Eastern Arabic numerals (١، ٢، ٣) — be consistent within a document
- All Arabic text must be RTL-compatible

### What Doesn't Need to be Mirrored

The following do NOT need Arabic mirrors:
- Code files (`.go`, `.dart`, `.yaml`)
- Git-related files (`.gitignore`, `LICENSE`)
- This `SYNC_GUIDE.md` (it is itself bilingual)
- `CONTRIBUTING.md` (when created — English only for contributor guidelines)

### Handling Terminology Differences

Some concepts have different primary terminology between languages:

| English | Arabic | Notes |
|---------|--------|-------|
| Recitation Queue | طابور التسميع / نظام ترتيب التسميع | "طابور" feels more natural |
| Circle | حلقة | Standard term used universally |
| Teacher / Reciter | مُقرئ / محفظ | Both used; context-dependent |
| Student | طالب | Standard |
| Supervisor | مُشرف | Standard |
| Session | جلسة | Standard |
| Grade | درجة | Standard |
| New Memorization | حفظ جديد | Standard |
| Revision | مراجعة | Standard |

---

## القسم العربي

### نظرة عامة

جميع وثائق حِلْقَتي موجودة في نسختين متوازيتين:
- **الإنجليزية** (ملفات `.md`، مثال: `PLAN.md`)
- **العربية** (ملفات `_AR.md`، مثال: `PLAN_AR.md`)

هذا الدليل يشرح سياسة المزامنة والمسؤوليات للحفاظ على توافق النسختين.

### أزواج الوثائق

| الوثيقة الإنجليزية | الوثيقة العربية | الوصف |
|------------------|---------------|-------|
| [`../README.md`](../README.md) | [`README_AR.md`](README_AR.md) | مقدمة المشروع |
| [`PLAN.md`](PLAN.md) | [`PLAN_AR.md`](PLAN_AR.md) | الخطة الرئيسية للمشروع |
| [`FEATURES.md`](FEATURES.md) | [`FEATURES_AR.md`](FEATURES_AR.md) | مواصفات المميزات |
| [`ARCHITECTURE.md`](ARCHITECTURE.md) | [`ARCHITECTURE_AR.md`](ARCHITECTURE_AR.md) | المعمارية التقنية |
| [`DEPLOYMENT.md`](DEPLOYMENT.md) | [`DEPLOYMENT_AR.md`](DEPLOYMENT_AR.md) | استراتيجية النشر |
| [`SYNC_GUIDE.md`](SYNC_GUIDE.md) | *(هذا الملف ثنائي اللغة)* | هذا الدليل |

### قواعد مصدر الحقيقة

| نوع المحتوى | مصدر الحقيقة | السبب |
|------------|-------------|-------|
| المصطلحات التقنية (أسماء APIs، المكتبات، الكود) | 🇬🇧 الإنجليزية | الدقة التقنية؛ الكود بالإنجليزية |
| المصطلحات القرآنية/الإسلامية للمستخدمين | 🇸🇦 العربية | العربية المصدر الأصيل للمصطلحات القرآنية |
| قرارات المميزات وحالتها | 🇬🇧 الإنجليزية | اللغة الأساسية لقرارات الفريق |
| نصوص واجهة المستخدم للمستخدمين العرب | 🇸🇦 العربية | المستخدمون العرب هم الجمهور الأساسي |
| أمثلة الكود والإعدادات | 🇬🇧 الإنجليزية فقط | لا داعي لتكرار الكود بالعربية |

### سياسة التحديث

**عند تحديث وثيقة إنجليزية:**
1. أجرِ تغييراتك على ملف `.md` الإنجليزي
2. افتح ملف `_AR.md` المقابل
3. طبِّق التغييرات المكافئة بالعربية
4. أرسِل كلا الملفين معاً في نفس commit Git
5. وصف pull request يجب أن يذكر أزواج الوثائق المُحدَّثة

**عند تحديث وثيقة عربية:**
1. أجرِ تغييراتك على ملف `_AR.md` العربي
2. افتح ملف `.md` الإنجليزي المقابل
3. طبِّق التغييرات المكافئة بالإنجليزية
4. أرسِل كلا الملفين معاً في نفس commit Git

**اصطلاح رسائل commit:**
```
docs: update [PLAN|FEATURES|ARCHITECTURE|DEPLOYMENT] (EN+AR)
```

### معايير جودة الترجمة

- **لا تستخدم الترجمة الآلية** (Google Translate، DeepL) للوثائق العربية
- يجب أن تكون العربية طبيعية واحترافية بالفصحى المعاصرة
- استخدم المصطلحات الإسلامية/القرآنية الأصيلة عند الاقتضاء
- المصطلحات التقنية (LiveKit، Docker، WebSocket، إلخ) تبقى بالإنجليزية داخل النص العربي
- الأرقام يمكن أن تكون غربية (1، 2، 3) أو عربية مشرقية (١، ٢، ٣) — تحلَّ باتساق داخل الوثيقة الواحدة
- جميع النصوص العربية يجب أن تكون متوافقة مع الكتابة من اليمين إلى اليسار (RTL)

### ما لا يحتاج إلى مرآة

لا تحتاج الوثائق التالية إلى نسخة عربية مقابلة:
- ملفات الكود (`.go`، `.dart`، `.yaml`)
- ملفات Git (`.gitignore`، `LICENSE`)
- هذا الملف `SYNC_GUIDE.md` (هو ثنائي اللغة بطبيعته)
- `CONTRIBUTING.md` (عند إنشائه — الإنجليزية فقط لإرشادات المساهمين)

### التعامل مع الاختلافات المصطلحية

بعض المفاهيم لها مصطلحات أساسية مختلفة بين اللغتين:

| الإنجليزية | العربية | ملاحظات |
|-----------|--------|---------|
| Recitation Queue | طابور التسميع / نظام ترتيب التسميع | "طابور" يبدو أكثر طبيعية |
| Circle | حلقة | المصطلح المعياري المستخدم عالمياً |
| Teacher / Reciter | مُقرئ / محفظ | كلاهما مستخدم؛ يعتمد على السياق |
| Student | طالب | معياري |
| Supervisor | مُشرف | معياري |
| Session | جلسة | معياري |
| Grade | درجة | معياري |
| New Memorization | حفظ جديد | معياري |
| Revision | مراجعة | معياري |

---

*هذه الوثيقة ثنائية اللغة بطبيعتها ولا تحتاج إلى ملف `_AR.md` مقابل.*

*This document is bilingual by nature and does not need a corresponding `_AR.md` file.*
