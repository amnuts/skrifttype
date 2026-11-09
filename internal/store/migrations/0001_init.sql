-- skrifttype v1 schema. Single migration; all tables share dependencies
-- so they land together. Future schema changes go in 0002_*.sql etc.

CREATE TABLE IF NOT EXISTS schema_version (
    version    INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL
);

CREATE TABLE watched_roots (
    id        INTEGER PRIMARY KEY,
    path      TEXT    NOT NULL UNIQUE,
    recursive INTEGER NOT NULL DEFAULT 1,
    enabled   INTEGER NOT NULL DEFAULT 1,
    added_at  INTEGER NOT NULL
);

CREATE TABLE fonts (
    id              INTEGER PRIMARY KEY,
    path            TEXT    NOT NULL UNIQUE,
    file_size       INTEGER NOT NULL,
    file_mtime      INTEGER NOT NULL,
    sha256          TEXT,

    family          TEXT    NOT NULL,
    subfamily       TEXT    NOT NULL,
    full_name       TEXT    NOT NULL,
    postscript_name TEXT,
    format          TEXT    NOT NULL,
    face_index      INTEGER NOT NULL DEFAULT 0,
    weight_class    INTEGER,
    width_class     INTEGER,
    italic          INTEGER NOT NULL DEFAULT 0,
    is_variable     INTEGER NOT NULL DEFAULT 0,
    classification  TEXT,

    designer        TEXT,
    foundry         TEXT,
    license_text    TEXT,
    version_string  TEXT,

    gsub_features      TEXT NOT NULL DEFAULT '[]',
    gpos_features      TEXT NOT NULL DEFAULT '[]',
    variation_axes     TEXT NOT NULL DEFAULT '[]',
    unicode_ranges     TEXT NOT NULL DEFAULT '[]',
    unicode_codepoints TEXT NOT NULL DEFAULT '[]',

    parse_error     TEXT,

    source          TEXT    NOT NULL,
    watched_root_id INTEGER,
    first_seen_at   INTEGER NOT NULL,
    last_seen_at    INTEGER NOT NULL,
    available       INTEGER NOT NULL DEFAULT 1,

    FOREIGN KEY (watched_root_id) REFERENCES watched_roots(id) ON DELETE SET NULL
);

CREATE INDEX idx_fonts_family         ON fonts(family);
CREATE INDEX idx_fonts_classification ON fonts(classification);
CREATE INDEX idx_fonts_source         ON fonts(source);
CREATE INDEX idx_fonts_postscript     ON fonts(postscript_name);

CREATE VIRTUAL TABLE fonts_fts USING fts5(
    family, subfamily, full_name, postscript_name, designer, foundry,
    content='fonts', content_rowid='id'
);

CREATE TRIGGER fonts_ai AFTER INSERT ON fonts BEGIN
    INSERT INTO fonts_fts (rowid, family, subfamily, full_name, postscript_name, designer, foundry)
    VALUES (new.id, new.family, new.subfamily, new.full_name, new.postscript_name, new.designer, new.foundry);
END;

CREATE TRIGGER fonts_ad AFTER DELETE ON fonts BEGIN
    INSERT INTO fonts_fts (fonts_fts, rowid, family, subfamily, full_name, postscript_name, designer, foundry)
    VALUES ('delete', old.id, old.family, old.subfamily, old.full_name, old.postscript_name, old.designer, old.foundry);
END;

CREATE TRIGGER fonts_au AFTER UPDATE ON fonts BEGIN
    INSERT INTO fonts_fts (fonts_fts, rowid, family, subfamily, full_name, postscript_name, designer, foundry)
    VALUES ('delete', old.id, old.family, old.subfamily, old.full_name, old.postscript_name, old.designer, old.foundry);
    INSERT INTO fonts_fts (rowid, family, subfamily, full_name, postscript_name, designer, foundry)
    VALUES (new.id, new.family, new.subfamily, new.full_name, new.postscript_name, new.designer, new.foundry);
END;

CREATE TABLE collections (
    id         INTEGER PRIMARY KEY,
    name       TEXT    NOT NULL UNIQUE,
    color      TEXT,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE TABLE collection_members (
    collection_id INTEGER NOT NULL,
    font_id       INTEGER NOT NULL,
    PRIMARY KEY (collection_id, font_id),
    FOREIGN KEY (collection_id) REFERENCES collections(id) ON DELETE CASCADE,
    FOREIGN KEY (font_id)       REFERENCES fonts(id)       ON DELETE CASCADE
);
CREATE INDEX idx_collection_members_font ON collection_members(font_id);

CREATE TABLE activations (
    font_id      INTEGER NOT NULL,
    source_kind  TEXT    NOT NULL,
    source_id    INTEGER,
    activated_at INTEGER NOT NULL,
    FOREIGN KEY (font_id) REFERENCES fonts(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_activations_unique ON activations(font_id, source_kind, COALESCE(source_id, 0));
CREATE INDEX idx_activations_source ON activations(source_kind, source_id);

CREATE TABLE tags (
    id   INTEGER PRIMARY KEY,
    name TEXT    NOT NULL UNIQUE
);

CREATE TABLE font_tags (
    font_id INTEGER NOT NULL,
    tag_id  INTEGER NOT NULL,
    PRIMARY KEY (font_id, tag_id),
    FOREIGN KEY (font_id) REFERENCES fonts(id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id)  REFERENCES tags(id)  ON DELETE CASCADE
);

CREATE TABLE preferences (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
