-- migrate:foreign-keys-off
-- A TrueType/OpenType collection (.ttc/.otc) holds several faces in one
-- file, so a font is identified by (path, face_index), not path alone.
-- SQLite cannot alter a column constraint, so rebuild `fonts` (the
-- documented 12-step recipe). The runner turns foreign keys off around
-- this migration so dropping `fonts` does not cascade into
-- collection_members, activations and font_tags, then runs
-- PRAGMA foreign_key_check before committing.

CREATE TABLE fonts_new (
    id              INTEGER PRIMARY KEY,
    path            TEXT    NOT NULL,
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

    FOREIGN KEY (watched_root_id) REFERENCES watched_roots(id) ON DELETE SET NULL,
    UNIQUE (path, face_index)
);

INSERT INTO fonts_new (
    id, path, file_size, file_mtime, sha256,
    family, subfamily, full_name, postscript_name, format, face_index,
    weight_class, width_class, italic, is_variable, classification,
    designer, foundry, license_text, version_string,
    gsub_features, gpos_features, variation_axes, unicode_ranges, unicode_codepoints,
    parse_error, source, watched_root_id, first_seen_at, last_seen_at, available
)
SELECT
    id, path, file_size, file_mtime, sha256,
    family, subfamily, full_name, postscript_name, format, face_index,
    weight_class, width_class, italic, is_variable, classification,
    designer, foundry, license_text, version_string,
    gsub_features, gpos_features, variation_axes, unicode_ranges, unicode_codepoints,
    parse_error, source, watched_root_id, first_seen_at, last_seen_at, available
FROM fonts;

DROP TRIGGER fonts_ai;
DROP TRIGGER fonts_ad;
DROP TRIGGER fonts_au;
DROP TABLE fonts_fts;
DROP TABLE fonts;
ALTER TABLE fonts_new RENAME TO fonts;

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

INSERT INTO fonts_fts(fonts_fts) VALUES('rebuild');
