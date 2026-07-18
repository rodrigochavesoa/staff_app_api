-- Migration Down: Auth, Users & Plans update rollback

DROP INDEX IF EXISTS idx_users_username;
DROP INDEX IF EXISTS idx_users_email;
