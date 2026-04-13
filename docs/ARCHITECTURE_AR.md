# حِلْقَتي — المعمارية التقنية للنظام

> **الإصدار:** 1.0 | **الحالة:** مرحلة التخطيط | **آخر تحديث:** 2026

**الوثائق ذات الصلة:** [ARCHITECTURE.md](ARCHITECTURE.md) · [PLAN_AR.md](PLAN_AR.md) · [DEPLOYMENT_AR.md](DEPLOYMENT_AR.md)

---

## جدول المحتويات

1. [مخطط النظام الشامل](#1-مخطط-النظام-الشامل)
2. [بروتوكولات الاتصال](#2-بروتوكولات-الاتصال)
3. [تكامل LiveKit مع Flutter](#3-تكامل-livekit-مع-flutter)
4. [مخطط قاعدة البيانات](#4-مخطط-قاعدة-البيانات)
5. [تخطيط نقاط API](#5-تخطيط-نقاط-api)
6. [اعتبارات الأمان](#6-اعتبارات-الأمان)

---

## 1. مخطط النظام الشامل

```
╔═══════════════════════════════════════════════════════════════════╗
║                       طبقة العميل                                 ║
║                                                                   ║
║   ┌─────────────────────────────────────────────────────────┐    ║
║   │        تطبيق Flutter (iOS / Android / الويب)             │    ║
║   │                                                          │    ║
║   │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────┐  │    ║
║   │  │ واجهة    │  │ واجهة   │  │ واجهة   │  │ واجهة │  │    ║
║   │  │ المصادقة │  │الحلقات  │  │المحادثة │  │الجلسة │  │    ║
║   │  └──────────┘  └──────────┘  └──────────┘  └────────┘  │    ║
║   │                                                          │    ║
║   │  الحزم: livekit_client · firebase_auth · fcm ·          │    ║
║   │          flutter_local_notifications                     │    ║
║   └─────────────────────────────────────────────────────────┘    ║
╚══════════╦═══════════════════╦═══════════════════╦═══════════════╝
           ║                   ║                   ║
     HTTPS/REST           WebSocket            WebRTC
   (مصادقة، CRUD)       (محادثات، طابور،    (صوت/فيديو
                         حضور، إشعارات)    عبر LiveKit)
           ║                   ║                   ║
╔══════════╩═══════════════════╩═══════════════════╩═══════════════╗
║                      طبقة الخادم الخلفي                          ║
║                                                                   ║
║  ┌─────────────────────────────────────────────────────────┐     ║
║  │                    خادم Go الخلفي                        │     ║
║  │                                                          │     ║
║  │  ┌─────────────────┐    ┌──────────────────────────┐    │     ║
║  │  │   واجهة REST     │    │     مركز WebSocket        │    │     ║
║  │  │   (Gin / Echo)   │    │                          │    │     ║
║  │  │                 │    │  ┌──────────────────┐    │    │     ║
║  │  │ /api/v1/auth    │    │  │ معالج المحادثات │    │    │     ║
║  │  │ /api/v1/circles │    │  ├──────────────────┤    │    │     ║
║  │  │ /api/v1/sessions│    │  │ معالج الطابور   │    │    │     ║
║  │  │ /api/v1/queue   │    │  ├──────────────────┤    │    │     ║
║  │  │ /api/v1/messages│    │  │ معالج الحضور    │    │    │     ║
║  │  │ /api/v1/progress│    │  ├──────────────────┤    │    │     ║
║  │  │ /api/v1/schedule│    │  │ موزع الإشعارات  │    │    │     ║
║  │  └────────┬─────────┘   └──────────┬───────────┘    │    │     ║
║  │           │                        │                 │     ║
║  │  ┌────────▼────────────────────────▼───────┐        │     ║
║  │  │           مدير LiveKit                   │        │     ║
║  │  │   (إنشاء الغرف، توليد الرموز)            │        │     ║
║  │  └────────┬────────────────────────────────┘        │     ║
║  └───────────┼─────────────────────────────────────────┘     ║
╚═════════════╦╩══════════════════════════════════════════════════╝
              ║
     ┌────────╩────────────────────────────────────────────────┐
     │                   طبقة البيانات والخدمات                 │
     │                                                           │
     │  ┌─────────────┐  ┌─────────────┐  ┌────────────────┐   │
     │  │  PostgreSQL  │  │    MinIO     │  │  LiveKit SFU   │   │
     │  │ (قاعدة الب.) │  │ (ملفات)     │  │(خادم WebRTC)  │   │
     │  └─────────────┘  └─────────────┘  └────────────────┘   │
     │                                                           │
     │  ┌─────────────────────────┐  ┌──────────────────────┐   │
     │  │       Firebase          │  │      Cloudflare      │   │
     │  │  Auth (الهوية)          │  │   (DNS + SSL/TLS)    │   │
     │  │  FCM (إشعارات دفع)      │  │                      │   │
     │  └─────────────────────────┘  └──────────────────────┘   │
     └───────────────────────────────────────────────────────────┘
```

---

## 2. بروتوكولات الاتصال

### 2.1 HTTPS / REST API

**يُستخدَم لـ:** جميع عمليات CRUD الاعتيادية، المصادقة، رفع الملفات، الإعدادات.

- نموذج طلب-استجابة عديم الحالة
- رأس المصادقة: `Authorization: Bearer <token>`
- أجسام الطلب/الاستجابة بتنسيق JSON
- رموز HTTP القياسية
- مُعمَّق: `/api/v1/...`

**متى يُستخدَم REST (لا WebSocket):**
- إنشاء/قراءة/تحديث/حذف الموارد (حلقات، جلسات، مستخدمون، سجلات تقدم)
- رفع الملفات (صور، رسائل صوتية)
- تبادل رموز المصادقة
- استرجاع البيانات الضخمة (التاريخ، التقارير)

### 2.2 WebSocket (الوقت الفعلي)

**يُستخدَم لـ:** الاتصال الفوري الذي يتطلب تأخيراً منخفضاً وإمكانية دفع من الخادم.

**دورة حياة اتصال WebSocket:**
```
العميل يتصل → WS /ws?token=<jwt>
    ↓
الخادم يُصادق على JWT
    ↓
الخادم يُسجِّل العميل في المركز (بمعرف المستخدم)
    ↓
العميل يشترك في الغرف (معرفات الحلقات والجلسات)
    ↓
رسائل ثنائية الاتجاه:
  العميل → الخادم: رسالة محادثة، إجراء طابور، مؤشر كتابة
  الخادم → العميل: رسالة محادثة، تحديث طابور، تغيير حضور، إشعار
    ↓
العميل ينقطع → المركز يُزيل التسجيل → بث تحديث الحضور
```

**تنسيق رسائل WebSocket:**
```json
{
  "type": "queue.student_status_changed",
  "session_id": "sess_123",
  "data": {
    "entry_id": "entry_456",
    "student_id": "user_789",
    "status": "reciting",
    "position": 3
  },
  "timestamp": "2026-03-15T10:30:00Z"
}
```

**أنواع الرسائل:**
| النوع | الاتجاه | الوصف |
|-------|---------|-------|
| `chat.message` | ثنائي | رسالة جديدة في الحلقة أو الرسائل المباشرة |
| `chat.typing` | ثنائي | مؤشر الكتابة |
| `chat.read` | E→X | تأشير الرسائل كمقروءة |
| `queue.updated` | X→E | تحديث كامل لحالة الطابور |
| `queue.student_status_changed` | X→E | تغيير حالة طالب واحد |
| `queue.round_started` | X→E | بدء جولة جديدة |
| `queue.reset` | X→E | إعادة ضبط الطابور |
| `presence.online` | X→E | مستخدم اتصل |
| `presence.offline` | X→E | مستخدم انقطع |
| `notification.in_app` | X→E | إشعار داخل التطبيق |
| `session.started` | X→E | المعلم بدأ الجلسة |
| `session.ended` | X→E | الجلسة انتهت |

### 2.3 WebRTC عبر LiveKit

**يُستخدَم لـ:** بث الصوت والفيديو في الجلسات المباشرة.

- LiveKit هو Selective Forwarding Unit (SFU) — يستقبل بث كل مشارك ويُعيد توجيهه للجميع دون دمج
- أكثر قابلية للتوسع من WebRTC النظير للنظير (الذي لا يتوسع لأكثر من ~4 مشاركين)
- عميل Flutter يستخدم حزمة `livekit_client` (الرسمية)
- خادم Go يستخدم `livekit-server-sdk-go` لإدارة الغرف وتوليد الرموز

### 2.4 Firebase Cloud Messaging (FCM)

**يُستخدَم لـ:** الإشعارات الفورية عند إغلاق التطبيق أو الخروج منه.

**التدفق:**
```
حدث يحدث (مثلاً: المعلم يبدأ الجلسة)
       ↓
خادم Go يكتشف الحدث
       ↓
خادم Go يسترجع رموز FCM للجهاز من قاعدة البيانات
       ↓
خادم Go → Firebase FCM API (HTTP POST)
       ↓
FCM → جهاز المستخدم (iOS APNs أو Android FCM)
       ↓
الجهاز يعرض الإشعار حتى لو كان التطبيق مغلقاً
```

---

## 3. تكامل LiveKit مع Flutter

### 3.1 تدفق التكامل الكامل

```
┌──────────────────────────────────────────────────────────────────┐
│                   تدفق تكامل LiveKit الكامل                      │
│                                                                   │
│  الخطوة 1: المعلم يبدأ الجلسة                                    │
│  ═════════════════════════════                                    │
│                                                                   │
│  Flutter UI              خادم Go               LiveKit Server   │
│     │                       │                       │            │
│     │  POST /sessions/{id}/start                    │            │
│     │──────────────────────►│                       │            │
│     │                       │  CreateRoom(name)     │            │
│     │                       │──────────────────────►│            │
│     │                       │  غرفة أُنشئت ✓        │            │
│     │                       │◄──────────────────────│            │
│     │  { token, livekit_url }│                      │            │
│     │◄──────────────────────│                       │            │
│                                                                   │
│  الخطوة 2: Flutter يتصل بـ LiveKit                               │
│  ══════════════════════════════                                   │
│                                                                   │
│  Flutter (livekit_client)                      LiveKit SFU      │
│     │                                               │             │
│     │  room.connect(url, token, RoomOptions{...})   │             │
│     │──────────────────────────────────────────────►│             │
│     │  مصافحة WebRTC (ICE, DTLS, SRTP)             │             │
│     │◄─────────────────────────────────────────────►│             │
│     │  متصل ✓                                       │             │
│     │◄──────────────────────────────────────────────│             │
│                                                                   │
│  الخطوة 3: المشاركون الآخرون ينضمون                             │
│  ══════════════════════════════════                               │
│                                                                   │
│  Flutter الطالب          خادم Go               LiveKit Server   │
│     │                       │                       │            │
│     │  GET /sessions/{id}/token                     │            │
│     │──────────────────────►│                       │            │
│     │  توليد JWT (roomName, uid)                    │            │
│     │  (الطالب: إذن الانضمام فقط، ليس المشرف)      │            │
│     │  { token }            │                       │            │
│     │◄──────────────────────│                       │            │
│     │  room.connect(url, token)                     │            │
│     │──────────────────────────────────────────────►│            │
│     │  متصل؛ يستقبل بثوث الصوت                     │            │
│     │◄──────────────────────────────────────────────│            │
│                                                                   │
│  الخطوة 4: تحكم المعلم (كتم، إخراج)                            │
│  ════════════════════════════════                                 │
│                                                                   │
│  Flutter UI              خادم Go               LiveKit Server   │
│     │  POST /sessions/{id}/                 │            │       │
│     │    mute/{participant_id}              │            │       │
│     │──────────────────────►│              │            │       │
│     │                       │  MutePublishedTrack (Admin API)   │
│     │                       │──────────────────────────────────►│
│     │                       │  المشارك مكتوم                    │
└──────────────────────────────────────────────────────────────────┘
```

### 3.2 نمط توليد الرمز في خادم Go

```go
// باستخدام github.com/livekit/server-sdk-go

func generateLiveKitToken(
    apiKey, apiSecret, roomName, identity, name string,
    isTeacher bool,
) (string, error) {
    
    at := auth.NewAccessToken(apiKey, apiSecret)
    
    grant := &auth.VideoGrant{
        RoomJoin:       true,
        Room:           roomName,
        CanPublish:     true,       // جميعهم ينشرون صوتهم
        CanSubscribe:   true,
        CanPublishData: isTeacher,  // فقط المعلم يُرسِل رسائل البيانات
    }
    
    // المعلم يحصل على صلاحيات إضافية
    if isTeacher {
        grant.RoomAdmin = true  // يمكنه كتم الآخرين وإخراجهم
    }
    
    at.AddGrant(grant).
        SetIdentity(identity). // معرف المستخدم
        SetName(name).         // اسم العرض
        SetValidFor(4 * time.Hour)
    
    return at.ToJWT()
}
```

### 3.3 نمط اتصال Flutter بالغرفة

```dart
// باستخدام حزمة livekit_client

Future<Room> connectToSession({
  required String livekitUrl,
  required String token,
}) async {
  final room = Room();
  
  // إعدادات صوت مُحسَّنة للتلاوة القرآنية
  final roomOptions = RoomOptions(
    defaultAudioPublishOptions: const AudioPublishOptions(
      name: 'recitation',
      audioBitrate: 48000,     // 48kbps كحد أدنى
    ),
    defaultVideoPublishOptions: const VideoPublishOptions(
      simulcast: true,         // جودة تكيفية للفيديو
    ),
    adaptiveStream: true,      // ضبط الجودة تلقائياً حسب النطاق الترددي
  );
  
  await room.connect(livekitUrl, token, roomOptions: roomOptions);
  
  return room;
}
```

### 3.4 إعداد الصوت (حرج)

```dart
// تعطيل إلغاء الضوضاء والتحكم التلقائي في الكسب للتلاوة القرآنية
// يجب ضبطه قبل الاتصال بالغرفة

await Hardware.instance.setPreferSpeakerOutput(true);

// معالجة الصوت على مستوى المنصة
final audioConstraints = {
  'echoCancellation': true,    // مفعَّل — يمنع الصدى
  'noiseSuppression': false,   // مُعطَّل — يحافظ على دقة المخارج والتجويد
  'autoGainControl': false,    // مُعطَّل — صوت تسميع ثابت
};
```

---

## 4. مخطط قاعدة البيانات

### 4.1 مخطط العلاقات (ASCII)

```
┌───────────────┐     ┌───────────────────┐     ┌───────────────┐
│  المستخدمون   │     │   أعضاء الحلقة   │     │   الحلقات    │
│───────────────│     │───────────────────│     │───────────────│
│ id (PK)       │────►│ user_id (FK)      │◄────│ id (PK)       │
│ name          │     │ circle_id (FK)    │     │ name          │
│ email         │     │ role              │     │ description   │
│ phone         │     │ joined_at         │     │ teacher_id(FK)│
│ avatar_url    │     └───────────────────┘     │ invite_code   │
│ fcm_token     │                               │ max_members   │
│ preferred_lang│     ┌───────────────────┐     │ privacy       │
│ created_at    │────►│   روابط الأولياء  │     │ gender_spec   │
│ updated_at    │     │───────────────────│     │ created_at    │
└──────┬────────┘     │ parent_user_id(FK)│     └───────┬───────┘
       │              │ student_user_id(FK│             │
       │              │ created_at        │     ┌───────▼───────┐
       │              └───────────────────┘     │   الجداول    │
       │                                        │───────────────│
       │              ┌───────────────────┐     │ id (PK)       │
       │              │   الإشعارات      │     │ circle_id (FK)│
       │◄─────────────│───────────────────│     │ day_of_week   │
       │              │ id (PK)           │     │ start_time    │
       │              │ user_id (FK)      │     │ end_time      │
       │              │ type              │     │ timezone      │
       │              │ title             │     │ is_active     │
       │              │ body              │     └───────────────┘
       │              │ data_json         │
       │              │ is_read           │     ┌───────────────┐
       │              │ created_at        │     │   الجلسات    │
       │              └───────────────────┘     │───────────────│
       │                                        │ id (PK)       │
       │              ┌───────────────────┐     │ circle_id (FK)│
       └─────────────►│    الرسائل       │     │ title         │
                      │───────────────────│◄────│ scheduled_at  │
                      │ id (PK)           │     │ actual_start  │
                      │ circle_id (FK)    │     │ actual_end    │
                      │ sender_id (FK)    │     │ status        │
                      │ content           │     │ lk_room_name  │
                      │ type              │     └───────┬───────┘
                      │ reply_to_id (FK)  │             │
                      │ is_pinned         │     ┌───────▼───────┐
                      │ created_at        │     │ حضور الجلسة  │
                      └───────────────────┘     │───────────────│
                                                │ session_id(FK)│
                      ┌───────────────────┐     │ user_id (FK)  │
                      │  قراءات الرسائل  │     │ joined_at     │
                      │───────────────────│     │ left_at       │
                      │ message_id (FK)   │     │ status        │
                      │ user_id (FK)      │     └───────────────┘
                      │ read_at           │
                      └───────────────────┘
                                                ┌───────────────┐
                      ┌───────────────────┐     │  طابور       │
                      │  سجل التحفيظ     │     │  التسميع     │
                      │───────────────────│     │───────────────│
                      │ id (PK)           │     │ id (PK)       │
                      │ student_id (FK)   │     │ session_id(FK)│
                      │ circle_id (FK)    │     │ round_number  │
                      │ surah_name        │     │ round_type    │
                      │ from_ayah         │     │ surah_name    │
                      │ to_ayah           │     │ from_ayah     │
                      │ type              │     │ to_ayah       │
                      │ grade             │     │ created_at    │
                      │ notes             │     └───────┬───────┘
                      │ session_id (FK)   │             │
                      │ date              │     ┌───────▼───────┐
                      └───────────────────┘     │  إدخالات     │
                                                │  الطابور     │
                                                │───────────────│
                                                │ id (PK)       │
                                                │ queue_id (FK) │
                                                │ student_id(FK)│
                                                │ position      │
                                                │ status        │
                                                │ grade         │
                                                │ teacher_notes │
                                                │ started_at    │
                                                │ completed_at  │
                                                └───────────────┘
```

### 4.2 تعريفات الجداول الرئيسية

#### جدول `users` (المستخدمون)
| العمود | النوع | القيود | الوصف |
|--------|-------|--------|-------|
| id | UUID | PK | معرف فريد |
| firebase_uid | VARCHAR(128) | UNIQUE NOT NULL | معرف Firebase Auth |
| name | VARCHAR(100) | NOT NULL | اسم العرض |
| email | VARCHAR(255) | UNIQUE | البريد الإلكتروني |
| phone | VARCHAR(20) | UNIQUE | رقم الهاتف مع رمز الدولة |
| avatar_url | TEXT | | رابط كائن MinIO |
| preferred_lang | VARCHAR(10) | DEFAULT 'ar' | رمز اللغة ISO 639-1 |
| fcm_token | TEXT | | رمز FCM للجهاز الحالي |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |
| updated_at | TIMESTAMPTZ | DEFAULT NOW() | |

#### جدول `circles` (الحلقات)
| العمود | النوع | القيود | الوصف |
|--------|-------|--------|-------|
| id | UUID | PK | |
| name | VARCHAR(100) | NOT NULL | اسم الحلقة |
| description | TEXT | | وصف الحلقة |
| rules | TEXT | | قواعد الحلقة |
| teacher_id | UUID | FK → users.id | مالك الحلقة |
| invite_code | VARCHAR(20) | UNIQUE NOT NULL | كود الانضمام |
| max_members | INTEGER | DEFAULT 50 | الحد الأقصى للأعضاء |
| privacy | VARCHAR(20) | CHECK IN ('public','private') | |
| gender_spec | VARCHAR(20) | CHECK IN ('male','female','mixed','unspecified') | |
| is_archived | BOOLEAN | DEFAULT FALSE | |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |

#### جدول `recitation_queue` (طابور التسميع)
| العمود | النوع | القيود | الوصف |
|--------|-------|--------|-------|
| id | UUID | PK | |
| session_id | UUID | FK → sessions.id NOT NULL | |
| round_number | INTEGER | NOT NULL | رقم الجولة |
| round_type | VARCHAR(30) | CHECK IN ('new_memorization','revision','old_revision','test') | نوع الجولة |
| surah_name | VARCHAR(100) | | اسم السورة |
| from_ayah | INTEGER | | آية البداية |
| to_ayah | INTEGER | | آية النهاية |
| is_active | BOOLEAN | DEFAULT TRUE | طابور واحد نشط لكل جلسة |
| created_at | TIMESTAMPTZ | DEFAULT NOW() | |

#### جدول `recitation_queue_entries` (إدخالات الطابور)
| العمود | النوع | القيود | الوصف |
|--------|-------|--------|-------|
| id | UUID | PK | |
| queue_id | UUID | FK → recitation_queue.id NOT NULL | |
| student_id | UUID | FK → users.id NOT NULL | |
| position | INTEGER | NOT NULL | الترتيب في الطابور |
| status | VARCHAR(20) | CHECK IN ('waiting','reciting','completed','skipped') | |
| grade | VARCHAR(30) | CHECK IN ('excellent','very_good','good','acceptable','needs_review','repeat') | |
| teacher_notes | TEXT | | ملاحظات المعلم |
| started_at | TIMESTAMPTZ | | وقت بدء التسميع |
| completed_at | TIMESTAMPTZ | | وقت الانتهاء/التقييم |

---

## 5. تخطيط نقاط API

### عنوان الأساس: `https://api.halaqaty.app/api/v1`

### المصادقة: جميع نقاط النهاية تتطلب `Authorization: Bearer <firebase-jwt>` ما عدا `/auth/*`

---

### `/auth` (المصادقة)
| الطريقة | المسار | الوصف |
|---------|--------|-------|
| POST | `/auth/register` | إنشاء ملف المستخدم بعد التسجيل في Firebase |
| POST | `/auth/fcm-token` | تحديث رمز FCM للجهاز |
| GET | `/auth/me` | جلب ملف المستخدم الحالي |
| PUT | `/auth/me` | تحديث الملف الشخصي |
| DELETE | `/auth/me` | حذف الحساب والبيانات |

### `/circles` (الحلقات)
| الطريقة | المسار | الوصف |
|---------|--------|-------|
| GET | `/circles` | قائمة حلقاتي (معلم + طالب) |
| POST | `/circles` | إنشاء حلقة جديدة |
| GET | `/circles/{id}` | تفاصيل حلقة |
| PUT | `/circles/{id}` | تحديث إعدادات الحلقة |
| DELETE | `/circles/{id}` | حذف الحلقة (المعلم فقط) |
| POST | `/circles/{id}/archive` | أرشفة الحلقة |
| POST | `/circles/join` | الانضمام بكود دعوة |
| POST | `/circles/{id}/leave` | مغادرة الحلقة |
| GET | `/circles/{id}/members` | قائمة الأعضاء مع الأدوار |
| PUT | `/circles/{id}/members/{userId}/role` | تحديث دور العضو |
| DELETE | `/circles/{id}/members/{userId}` | إخراج عضو من الحلقة |
| POST | `/circles/{id}/invite-code/refresh` | توليد كود دعوة جديد |

### `/sessions` (الجلسات)
| الطريقة | المسار | الوصف |
|---------|--------|-------|
| GET | `/circles/{id}/sessions` | قائمة جلسات الحلقة |
| POST | `/circles/{id}/sessions` | إنشاء جلسة |
| GET | `/sessions/{id}` | تفاصيل جلسة |
| POST | `/sessions/{id}/start` | المعلم يبدأ الجلسة (ينشئ غرفة LiveKit) |
| POST | `/sessions/{id}/end` | المعلم ينهي الجلسة |
| GET | `/sessions/{id}/token` | جلب رمز JWT للجلسة |
| POST | `/sessions/{id}/participants/{userId}/mute` | كتم مشارك |
| POST | `/sessions/{id}/participants/{userId}/remove` | إخراج مشارك |
| POST | `/sessions/{id}/lock` | قفل الجلسة |

### `/queue` (الطابور)
| الطريقة | المسار | الوصف |
|---------|--------|-------|
| GET | `/sessions/{id}/queue` | جلب حالة الطابور الحالية |
| POST | `/sessions/{id}/queue/rounds` | بدء جولة جديدة |
| POST | `/sessions/{id}/queue/reset` | إعادة ضبط الطابور |
| PUT | `/sessions/{id}/queue/entries/{entryId}/status` | تحديث حالة طالب في الطابور |
| PUT | `/sessions/{id}/queue/entries/{entryId}/grade` | تقييم تسميع طالب |
| PUT | `/sessions/{id}/queue/order` | إعادة ترتيب الطابور |
| POST | `/sessions/{id}/queue/entries` | إضافة طالب للطابور (متأخر) |
| DELETE | `/sessions/{id}/queue/entries/{entryId}` | إزالة طالب من الطابور |

### `/messages` (الرسائل)
| الطريقة | المسار | الوصف |
|---------|--------|-------|
| GET | `/circles/{id}/messages` | قائمة رسائل الحلقة (مُقسَّمة) |
| POST | `/circles/{id}/messages` | إرسال رسالة |
| DELETE | `/circles/{id}/messages/{msgId}` | حذف رسالة |
| POST | `/circles/{id}/messages/{msgId}/pin` | تثبيت رسالة |
| DELETE | `/circles/{id}/messages/{msgId}/pin` | إلغاء تثبيت |
| POST | `/circles/{id}/messages/{msgId}/read` | تأشير مقروء |
| GET | `/dm/{userId}` | قائمة محادثة مباشرة |
| POST | `/dm/{userId}` | إرسال رسالة مباشرة |

### `/progress` (التقدم)
| الطريقة | المسار | الوصف |
|---------|--------|-------|
| GET | `/circles/{id}/progress` | تقدم جميع الطلاب في الحلقة |
| GET | `/circles/{id}/progress/{userId}` | تقدم طالب محدد |
| GET | `/progress/me` | تقدمي الشخصي عبر جميع الحلقات |

---

## 6. اعتبارات الأمان

### 6.1 المصادقة والتفويض

- **الهوية:** Firebase Auth تُصدِر JWTs؛ خادم Go يتحقق منها في كل طلب
- **التفويض:** قائم على الأدوار لكل حلقة. بعد التحقق من JWT، يتحقق الخادم من جدول `circle_members`
- **دورة حياة الرمز:** رموز Firebase تنتهي صلاحيتها بعد ساعة؛ SDK يُجدِّدها آلياً
- **رموز LiveKit:** يُولِّدها خادم Go حصرياً؛ المدى محدود (إذن الانضمام فقط) للطلاب

### 6.2 أمان غرف LiveKit

- كل جلسة تُولِّد اسم غرفة LiveKit فريداً (مبني على UUID)
- أسماء الغرف غير قابلة للتخمين
- كل مشارك يحتاج JWT من خادم Go للانضمام — لا وصول مجهول
- JWT المعلم يتضمن `RoomAdmin: true` (يمكنه الكتم والإخراج)
- JWT الطالب يتضمن `CanPublish: true` لكن ليس `RoomAdmin`
- تُحذف الغرفة من خادم LiveKit عند انتهاء الجلسة

### 6.3 تحديد المعدل

- REST API: محدودة بعنوان IP ومعرف المستخدم
- WebSocket: حد أقصى 3 اتصالات نشطة للمستخدم الواحد
- إرسال الرسائل: 30 رسالة/دقيقة لكل مستخدم في كل حلقة
- رفع الملفات: 10 عمليات رفع/ساعة لكل مستخدم

### 6.4 التحقق من المدخلات

- جميع مدخلات API تُتحقَّق وتُعقَّم من جانب الخادم
- أرقام الآيات تُتحقَّق من صحتها مقابل أطوال السور المعروفة
- التحقق من نوع الملف (MIME type، ليس الامتداد فقط)
- الأحجام القصوى للملفات تُطبَّق من جانب الخادم
- منع SQL Injection عبر الاستعلامات المُحدَّدة المعاملات (Go + pgx)
- منع XSS: محتوى الرسائل مُخزَّن كنص عادي؛ HTML مُهرَّب عند العرض

### 6.5 خصوصية البيانات

- الرسائل الصوتية والتسجيلات مُخزَّنة في MinIO مع سياسات تحكم وصول
- روابط الملفات موقَّعة مؤقتاً وتنتهي صلاحيتها بعد 7 أيام
- البيانات الشخصية لا تُعاد في APIs المرئية للمجموعة
- تسجيلات الجلسات تتطلب موافقة صريحة من المعلم

### 6.6 أمان النقل

- HTTPS مُطبَّق في كل مكان (TLS 1.2+، Cloudflare يدير الشهادات)
- اتصالات WebSocket عبر WSS (TLS)
- بثوث WebRTC مشفرة بـ DTLS/SRTP (مدمج في بروتوكول WebRTC)
- رؤوس HSTS مُضبوطة

---

*هذه الوثيقة هي المرجع التقني الرئيسي. راجع [ARCHITECTURE.md](ARCHITECTURE.md) للنسخة الإنجليزية.*

*راجع [DEPLOYMENT_AR.md](DEPLOYMENT_AR.md) لتفاصيل البنية التحتية والنشر.*
