-- A connection grant names the account session that asked for it, so the relay
-- can end a controller when that session signs out or reaches its sign-in age.
-- The column holds the session's stored hash, never a usable cookie.

ALTER TABLE remote_connection_grants ADD COLUMN session_hash TEXT;
ALTER TABLE remote_connection_grants ADD COLUMN authenticated_at TEXT;
