# F-008 User-Targeted Realtime Contract

Add one server-to-client event named `notification.updated` using the existing authenticated WebSocket envelope. It is user-targeted through the existing `realtime.Hub.SendToUsers` authorization path; it is never broadcast to a circle topic.

```json
{
  "type": "notification.updated",
  "payload": {
    "notification_id": "optional UUID",
    "unread_count": 3
  },
  "timestamp": "UTC ISO 8601"
}
```

- `notification_id` is included when one notification is created or marked read; omit it for a bulk read-all/count invalidation.
- `unread_count` is the server-authoritative count after the committed transition.
- The event contains no title, body, message text, grade data, circle membership details, media data, token, or deep-link URL. The client fetches the authorized item from `GET /notifications`.
- The event updates passive history/count state; preference OFF suppresses foreground alert presentation but does not suppress this reconciliation event.
- Delivery may be duplicated or missed. Clients replace their displayed count with the supplied value and reconcile via REST on reconnect/resume/account switch; they do not increment from push receipt.
