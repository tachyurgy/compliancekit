-- REQ-010: short-lived email-verification tokens sent at signup.
-- 24-hour expiry, single-use. Token is hashed (sha256) at rest; the raw
-- token is in the email link only. Mirrors password_resets shape.
CREATE TABLE email_verifications (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TEXT NOT NULL,
    used_at     TEXT,
    ip          TEXT,
    user_agent  TEXT,
    created_at  TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_email_verifications_user    ON email_verifications(user_id);
CREATE INDEX idx_email_verifications_expires ON email_verifications(expires_at) WHERE used_at IS NULL;
