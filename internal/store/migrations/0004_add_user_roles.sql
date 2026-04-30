-- Add role + must_change_password to users.
-- role is a four-tier enum (guest < user < mod < admin); existing rows
-- pick up DEFAULT 'user' which preserves prior behavior.
-- must_change_password is set to 1 when seeding the bootstrapped admin
-- account on first run; the password-change screen clears it on success.

ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user'
    CHECK (role IN ('guest','user','mod','admin'));
ALTER TABLE users ADD COLUMN must_change_password INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_users_role ON users(role);
