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
    owner_id    TEXT NOT NULL,
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

-- Phase 2 tables

CREATE TABLE IF NOT EXISTS genres (
    id          TEXT PRIMARY KEY,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    deleted_at  TEXT,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    description TEXT,
    parent_id   TEXT REFERENCES genres(id),
    path        TEXT NOT NULL,
    depth       INTEGER NOT NULL DEFAULT 0,
    sort_order  INTEGER NOT NULL DEFAULT 0,
    color       TEXT,
    icon        TEXT,
    is_system   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS book_genres (
    book_id     TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    genre_id    TEXT NOT NULL REFERENCES genres(id),
    PRIMARY KEY (book_id, genre_id)
);
CREATE INDEX IF NOT EXISTS idx_book_genres_genre ON book_genres(genre_id);

CREATE TABLE IF NOT EXISTS tags (
    id          TEXT PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS book_tags (
    book_id     TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    tag_id      TEXT NOT NULL REFERENCES tags(id),
    created_at  TEXT NOT NULL,
    PRIMARY KEY (book_id, tag_id)
);
CREATE INDEX IF NOT EXISTS idx_book_tags_tag ON book_tags(tag_id);

CREATE TABLE IF NOT EXISTS collections (
    id                TEXT PRIMARY KEY,
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    library_id        TEXT NOT NULL REFERENCES libraries(id),
    owner_id          TEXT NOT NULL,
    name              TEXT NOT NULL,
    is_inbox          INTEGER NOT NULL DEFAULT 0,
    is_global_access  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS collection_books (
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    book_id       TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    PRIMARY KEY (collection_id, book_id)
);
CREATE INDEX IF NOT EXISTS idx_collection_books_book ON collection_books(book_id);

CREATE TABLE IF NOT EXISTS collection_shares (
    id                    TEXT PRIMARY KEY,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL,
    deleted_at            TEXT,
    collection_id         TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    shared_with_user_id   TEXT NOT NULL REFERENCES users(id),
    shared_by_user_id     TEXT NOT NULL REFERENCES users(id),
    permission            TEXT NOT NULL DEFAULT 'read'
);
CREATE INDEX IF NOT EXISTS idx_collection_shares_user ON collection_shares(shared_with_user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_collection_shares_collection ON collection_shares(collection_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS shelves (
    id          TEXT PRIMARY KEY,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    owner_id    TEXT NOT NULL REFERENCES users(id),
    name        TEXT NOT NULL,
    description TEXT,
    color       TEXT,
    icon        TEXT
);

CREATE TABLE IF NOT EXISTS shelf_books (
    shelf_id    TEXT NOT NULL REFERENCES shelves(id) ON DELETE CASCADE,
    book_id     TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    sort_order  INTEGER NOT NULL,
    PRIMARY KEY (shelf_id, book_id)
);
CREATE INDEX IF NOT EXISTS idx_shelf_books_book ON shelf_books(book_id);

CREATE TABLE IF NOT EXISTS listening_events (
    id                  TEXT PRIMARY KEY,
    user_id             TEXT NOT NULL REFERENCES users(id),
    book_id             TEXT NOT NULL REFERENCES books(id),
    start_position_ms   INTEGER NOT NULL,
    end_position_ms     INTEGER NOT NULL,
    started_at          TEXT NOT NULL,
    ended_at            TEXT NOT NULL,
    playback_speed      REAL NOT NULL,
    device_id           TEXT NOT NULL,
    device_name         TEXT,
    source              TEXT NOT NULL,
    duration_ms         INTEGER NOT NULL,
    created_at          TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_le_user_ended ON listening_events(user_id, ended_at);
CREATE INDEX IF NOT EXISTS idx_le_book ON listening_events(book_id);
CREATE INDEX IF NOT EXISTS idx_le_user_book ON listening_events(user_id, book_id);

CREATE TABLE IF NOT EXISTS playback_state (
    user_id                 TEXT NOT NULL REFERENCES users(id),
    book_id                 TEXT NOT NULL REFERENCES books(id),
    current_position_ms     INTEGER NOT NULL DEFAULT 0,
    is_finished             INTEGER NOT NULL DEFAULT 0,
    finished_at             TEXT,
    started_at              TEXT NOT NULL,
    last_played_at          TEXT NOT NULL,
    total_listen_time_ms    INTEGER NOT NULL DEFAULT 0,
    updated_at              TEXT NOT NULL,
    PRIMARY KEY (user_id, book_id)
);
CREATE INDEX IF NOT EXISTS idx_ps_user_last_played ON playback_state(user_id, last_played_at DESC)
    WHERE is_finished = 0;

CREATE TABLE IF NOT EXISTS book_preferences (
    user_id                         TEXT NOT NULL REFERENCES users(id),
    book_id                         TEXT NOT NULL REFERENCES books(id),
    playback_speed                  REAL,
    skip_forward_sec                INTEGER,
    hide_from_continue_listening    INTEGER NOT NULL DEFAULT 0,
    updated_at                      TEXT NOT NULL,
    PRIMARY KEY (user_id, book_id)
);

CREATE TABLE IF NOT EXISTS book_reading_sessions (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id),
    book_id         TEXT NOT NULL REFERENCES books(id),
    started_at      TEXT NOT NULL,
    finished_at     TEXT,
    is_completed    INTEGER NOT NULL DEFAULT 0,
    final_progress  REAL NOT NULL DEFAULT 0,
    listen_time_ms  INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_brs_user_book ON book_reading_sessions(user_id, book_id);

CREATE TABLE IF NOT EXISTS invites (
    id          TEXT PRIMARY KEY,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    deleted_at  TEXT,
    code        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL DEFAULT '',
    email       TEXT NOT NULL DEFAULT '',
    role        TEXT NOT NULL DEFAULT 'member',
    created_by  TEXT NOT NULL REFERENCES users(id),
    expires_at  TEXT NOT NULL,
    claimed_at  TEXT,
    claimed_by  TEXT REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS user_profiles (
    user_id       TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    avatar_type   TEXT NOT NULL DEFAULT 'auto',
    avatar_value  TEXT NOT NULL DEFAULT '',
    tagline       TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS user_settings (
    user_id                     TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    default_playback_speed      REAL NOT NULL DEFAULT 1.0,
    default_skip_forward_sec    INTEGER NOT NULL DEFAULT 30,
    default_skip_backward_sec   INTEGER NOT NULL DEFAULT 10,
    default_sleep_timer_min     INTEGER,
    shake_to_reset_sleep_timer  INTEGER NOT NULL DEFAULT 0,
    updated_at                  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS instance (
    key     TEXT PRIMARY KEY,
    value   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS server_settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS genre_aliases (
    id          TEXT PRIMARY KEY,
    raw_value   TEXT NOT NULL,
    raw_lower   TEXT NOT NULL,
    genre_ids   TEXT NOT NULL DEFAULT '[]',
    created_by  TEXT NOT NULL REFERENCES users(id),
    created_at  TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_genre_aliases_raw ON genre_aliases(raw_lower);

CREATE TABLE IF NOT EXISTS unmapped_genres (
    raw_value   TEXT NOT NULL,
    raw_slug    TEXT NOT NULL PRIMARY KEY,
    book_count  INTEGER NOT NULL DEFAULT 1,
    first_seen  TEXT NOT NULL,
    book_ids    TEXT NOT NULL DEFAULT '[]'
);

-- Phase 3 tables

CREATE TABLE IF NOT EXISTS activities (
    id                  TEXT PRIMARY KEY,
    user_id             TEXT NOT NULL REFERENCES users(id),
    type                TEXT NOT NULL,
    created_at          TEXT NOT NULL,
    user_display_name   TEXT NOT NULL DEFAULT '',
    user_avatar_color   TEXT NOT NULL DEFAULT '',
    user_avatar_type    TEXT NOT NULL DEFAULT '',
    user_avatar_value   TEXT NOT NULL DEFAULT '',
    book_id             TEXT,
    book_title          TEXT,
    book_author_name    TEXT,
    book_cover_path     TEXT,
    is_reread           INTEGER NOT NULL DEFAULT 0,
    duration_ms         INTEGER NOT NULL DEFAULT 0,
    milestone_value     INTEGER NOT NULL DEFAULT 0,
    milestone_unit      TEXT,
    lens_id             TEXT,
    lens_name           TEXT
);
CREATE INDEX IF NOT EXISTS idx_activities_user ON activities(user_id);
CREATE INDEX IF NOT EXISTS idx_activities_created ON activities(created_at DESC);

CREATE TABLE IF NOT EXISTS user_stats (
    user_id             TEXT PRIMARY KEY REFERENCES users(id),
    total_listen_ms     INTEGER NOT NULL DEFAULT 0,
    books_finished      INTEGER NOT NULL DEFAULT 0,
    current_streak      INTEGER NOT NULL DEFAULT 0,
    longest_streak      INTEGER NOT NULL DEFAULT 0,
    last_listened_date  TEXT,
    updated_at          TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS user_milestone_states (
    user_id                  TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    last_streak_days         INTEGER NOT NULL DEFAULT 0,
    last_listen_hours_total  INTEGER NOT NULL DEFAULT 0,
    updated_at               TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS transcode_jobs (
    id              TEXT PRIMARY KEY,
    book_id         TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    audio_file_id   TEXT NOT NULL,
    source_path     TEXT NOT NULL,
    source_codec    TEXT NOT NULL,
    source_hash     TEXT NOT NULL DEFAULT '',
    output_path     TEXT NOT NULL DEFAULT '',
    output_codec    TEXT NOT NULL DEFAULT 'aac',
    output_size     INTEGER NOT NULL DEFAULT 0,
    variant         TEXT NOT NULL DEFAULT 'stereo',
    status          TEXT NOT NULL DEFAULT 'pending',
    progress        INTEGER NOT NULL DEFAULT 0,
    priority        INTEGER NOT NULL DEFAULT 1,
    error           TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    started_at      TEXT,
    completed_at    TEXT
);
CREATE INDEX IF NOT EXISTS idx_transcode_jobs_book ON transcode_jobs(book_id);
CREATE INDEX IF NOT EXISTS idx_transcode_jobs_audio_file ON transcode_jobs(audio_file_id);
CREATE INDEX IF NOT EXISTS idx_transcode_jobs_status ON transcode_jobs(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_transcode_jobs_audio_variant ON transcode_jobs(audio_file_id, variant);

CREATE TABLE IF NOT EXISTS abs_imports (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    backup_path         TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'active',
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    completed_at        TEXT,
    total_users         INTEGER NOT NULL DEFAULT 0,
    total_books         INTEGER NOT NULL DEFAULT 0,
    total_sessions      INTEGER NOT NULL DEFAULT 0,
    users_mapped        INTEGER NOT NULL DEFAULT 0,
    books_mapped        INTEGER NOT NULL DEFAULT 0,
    sessions_imported   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS abs_import_users (
    import_id       TEXT NOT NULL REFERENCES abs_imports(id) ON DELETE CASCADE,
    abs_user_id     TEXT NOT NULL,
    abs_username    TEXT NOT NULL DEFAULT '',
    abs_email       TEXT NOT NULL DEFAULT '',
    listenup_id     TEXT,
    lu_email        TEXT,
    lu_display_name TEXT,
    mapped_at       TEXT,
    session_count   INTEGER NOT NULL DEFAULT 0,
    total_listen_ms INTEGER NOT NULL DEFAULT 0,
    confidence      TEXT NOT NULL DEFAULT 'none',
    match_reason    TEXT NOT NULL DEFAULT '',
    suggestions     TEXT NOT NULL DEFAULT '[]',
    PRIMARY KEY (import_id, abs_user_id)
);

CREATE TABLE IF NOT EXISTS abs_import_books (
    import_id       TEXT NOT NULL REFERENCES abs_imports(id) ON DELETE CASCADE,
    abs_media_id    TEXT NOT NULL,
    abs_title       TEXT NOT NULL DEFAULT '',
    abs_author      TEXT NOT NULL DEFAULT '',
    abs_duration_ms INTEGER NOT NULL DEFAULT 0,
    abs_asin        TEXT NOT NULL DEFAULT '',
    abs_isbn        TEXT NOT NULL DEFAULT '',
    listenup_id     TEXT,
    lu_title        TEXT,
    lu_author       TEXT,
    mapped_at       TEXT,
    session_count   INTEGER NOT NULL DEFAULT 0,
    confidence      TEXT NOT NULL DEFAULT 'none',
    match_reason    TEXT NOT NULL DEFAULT '',
    suggestions     TEXT NOT NULL DEFAULT '[]',
    PRIMARY KEY (import_id, abs_media_id)
);

CREATE TABLE IF NOT EXISTS abs_import_sessions (
    import_id       TEXT NOT NULL REFERENCES abs_imports(id) ON DELETE CASCADE,
    abs_session_id  TEXT NOT NULL,
    abs_user_id     TEXT NOT NULL,
    abs_media_id    TEXT NOT NULL,
    start_time      TEXT NOT NULL,
    duration        INTEGER NOT NULL DEFAULT 0,
    start_position  INTEGER NOT NULL DEFAULT 0,
    end_position    INTEGER NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'pending_user',
    imported_at     TEXT,
    skip_reason     TEXT,
    PRIMARY KEY (import_id, abs_session_id)
);
CREATE INDEX IF NOT EXISTS idx_abs_import_sessions_user ON abs_import_sessions(import_id, abs_user_id);
CREATE INDEX IF NOT EXISTS idx_abs_import_sessions_media ON abs_import_sessions(import_id, abs_media_id);

CREATE TABLE IF NOT EXISTS abs_import_progress (
    import_id       TEXT NOT NULL REFERENCES abs_imports(id) ON DELETE CASCADE,
    abs_user_id     TEXT NOT NULL,
    abs_media_id    TEXT NOT NULL,
    current_time    INTEGER NOT NULL DEFAULT 0,
    duration        INTEGER NOT NULL DEFAULT 0,
    progress        REAL NOT NULL DEFAULT 0,
    is_finished     INTEGER NOT NULL DEFAULT 0,
    finished_at     TEXT,
    last_update     TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending_user',
    imported_at     TEXT,
    PRIMARY KEY (import_id, abs_user_id, abs_media_id)
);

CREATE TABLE IF NOT EXISTS audible_cache_books (
    region      TEXT NOT NULL,
    asin        TEXT NOT NULL,
    data        TEXT NOT NULL,
    fetched_at  TEXT NOT NULL,
    PRIMARY KEY (region, asin)
);

CREATE TABLE IF NOT EXISTS audible_cache_chapters (
    region      TEXT NOT NULL,
    asin        TEXT NOT NULL,
    data        TEXT NOT NULL,
    fetched_at  TEXT NOT NULL,
    PRIMARY KEY (region, asin)
);

CREATE TABLE IF NOT EXISTS audible_cache_search (
    region      TEXT NOT NULL,
    query       TEXT NOT NULL,
    data        TEXT NOT NULL,
    fetched_at  TEXT NOT NULL,
    PRIMARY KEY (region, query)
);
