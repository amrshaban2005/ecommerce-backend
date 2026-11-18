CREATE TABLE users (
 id UUID PRIMARY KEY,
 full_name TEXT NOT NULL CHECK (length(btrim(full_name)) > 0),
 email TEXT NOT NULL,
 password_hash TEXT NOT NULL CHECK (length(password_hash) > 0),
 role TEXT NOT NULL DEFAULT 'Customer' CHECK (role IN ('Customer','Administrator')),
 CONSTRAINT users_email_key UNIQUE (email)
);
