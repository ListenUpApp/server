CREATE TABLE IF NOT EXISTS users (
    id              TEXT PRIMARY KEY,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    deleted_at      TEXT,
    email           TEXT NOT NULL,
    email_lower     TEXT NOT NULL UNIQUE,
    password_hash   TEXT,
    is_root         INTEGER NOT NULL DEFAULT 0,
    role            TEXT NOT NULL DEFAULT 'member',
    status          TEXT NOT NULL DEFAULT 'active',
    invited_by      TEXT REFERENCES users(id),
    approved_by     TEXT REFERENCES users(id),
    approved_at     TEXT,
    display_name    TEXT NOT NULL DEFAULT '',
    first_name      TEXT NOT NULL DEFAULT '',
    last_name       TEXT NOT NULL DEFAULT '',
    last_login_at   TEXT NOT NULL,
    can_download    INTEGER NOT NULL DEFAULT 1,
    can_share       INTEGER NOT NULL DEFAULT 1,
    avatar_type     TEXT,
    avatar_color    TEXT
);

CREATE TABLE IF NOT EXISTS sessions (
    id                  TEXT PRIMARY KEY,
    user_id             TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash  TEXT,
    expires_at          TEXT NOT NULL,
    created_at          TEXT NOT NULL,
    last_seen_at        TEXT NOT NULL,
    ip_address          TEXT,
    device_type         TEXT,
    platform            TEXT,
    platform_version    TEXT,
    client_name         TEXT,
    client_version      TEXT,
    client_build        TEXT,
    device_name         TEXT,
    device_model        TEXT,
    browser_name        TEXT,
    browser_version     TEXT
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS libraries (
    id          TEXT PRIMARY KEY,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    owner_id    TEXT NOT NULL REFERENCES users(id),
    name        TEXT NOT NULL,
    scan_paths  TEXT NOT NULL DEFAULT '[]',
    skip_inbox  INTEGER NOT NULL DEFAULT 0,
    access_mode TEXT NOT NULL DEFAULT 'open'
);

CREATE TABLE IF NOT EXISTS books (
    id              TEXT PRIMARY KEY,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    deleted_at      TEXT,
    scanned_at      TEXT NOT NULL,
    isbn            TEXT,
    title           TEXT NOT NULL,
    subtitle        TEXT,
    path            TEXT NOT NULL UNIQUE,
    description     TEXT,
    publisher       TEXT,
    publish_year    TEXT,
    language        TEXT,
    asin            TEXT,
    audible_region  TEXT,
    total_duration  INTEGER NOT NULL DEFAULT 0,
    total_size      INTEGER NOT NULL DEFAULT 0,
    abridged        INTEGER NOT NULL DEFAULT 0,
    cover_path      TEXT,
    cover_filename  TEXT,
    cover_format    TEXT,
    cover_size      INTEGER,
    cover_inode     INTEGER,
    cover_mod_time  INTEGER,
    cover_blur_hash TEXT,
    staged_collection_ids TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX IF NOT EXISTS idx_books_updated ON books(updated_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_books_title ON books(title COLLATE NOCASE) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS book_audio_files (
    id          TEXT NOT NULL,
    book_id     TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    path        TEXT NOT NULL,
    filename    TEXT NOT NULL,
    format      TEXT NOT NULL,
    codec       TEXT,
    size        INTEGER NOT NULL,
    duration    INTEGER NOT NULL,
    bitrate     INTEGER,
    inode       INTEGER NOT NULL,
    mod_time    INTEGER NOT NULL,
    sort_order  INTEGER NOT NULL,
    PRIMARY KEY (book_id, id)
);
CREATE INDEX IF NOT EXISTS idx_book_audio_files_inode ON book_audio_files(inode);

CREATE TABLE IF NOT EXISTS book_chapters (
    book_id         TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    idx             INTEGER NOT NULL,
    title           TEXT NOT NULL,
    audio_file_id   TEXT,
    start_time      INTEGER NOT NULL,
    end_time        INTEGER NOT NULL,
    PRIMARY KEY (book_id, idx)
);

CREATE TABLE IF NOT EXISTS contributors (
    id              TEXT PRIMARY KEY,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    deleted_at      TEXT,
    name            TEXT NOT NULL,
    sort_name       TEXT,
    biography       TEXT,
    image_url       TEXT,
    image_blur_hash TEXT,
    asin            TEXT,
    aliases         TEXT NOT NULL DEFAULT '[]',
    website         TEXT,
    birth_date      TEXT,
    death_date      TEXT
);
CREATE INDEX IF NOT EXISTS idx_contributors_updated ON contributors(updated_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_contributors_name ON contributors(sort_name COLLATE NOCASE) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS book_contributors (
    book_id         TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    contributor_id  TEXT NOT NULL REFERENCES contributors(id),
    roles           TEXT NOT NULL DEFAULT '[]',
    credited_as     TEXT,
    PRIMARY KEY (book_id, contributor_id)
);
CREATE INDEX IF NOT EXISTS idx_book_contributors_contributor ON book_contributors(contributor_id);

CREATE TABLE IF NOT EXISTS series (
    id              TEXT PRIMARY KEY,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    deleted_at      TEXT,
    name            TEXT NOT NULL,
    description     TEXT,
    asin            TEXT,
    cover_path      TEXT,
    cover_filename  TEXT,
    cover_format    TEXT,
    cover_size      INTEGER,
    cover_inode     INTEGER,
    cover_mod_time  INTEGER,
    cover_blur_hash TEXT
);
CREATE INDEX IF NOT EXISTS idx_series_updated ON series(updated_at) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS book_series (
    book_id     TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    series_id   TEXT NOT NULL REFERENCES series(id),
    sequence    TEXT,
    PRIMARY KEY (book_id, series_id)
);
