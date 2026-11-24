package rbac

const invitationNotificationEligibleQuery = `
SELECT EXISTS (
 SELECT 1 FROM circle_invitations i
 JOIN circles c ON c.id = i.circle_id
 JOIN users u ON u.id = i.recipient_user_id
 JOIN users inviter ON inviter.id = i.inviter_user_id
 JOIN circle_members m ON m.circle_id = i.circle_id AND m.user_id = i.inviter_user_id
 WHERE i.id = $1 AND i.recipient_user_id = $2 AND i.state = 'pending'
   AND c.is_archived = FALSE AND u.deleted_at IS NULL AND inviter.deleted_at IS NULL
   AND (m.role IN ('teacher', 'supervisor') OR (m.role = 'student' AND i.role = 'student'))
)`
