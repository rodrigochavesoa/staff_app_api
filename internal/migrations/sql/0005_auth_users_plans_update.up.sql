-- Migration Up: Auth, Users & Plans update

-- 1. Create indexes for user lookups
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
