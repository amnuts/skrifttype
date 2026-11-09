-- A collection's "active" flag is user intent, independent of whether it
-- currently has any members (an empty collection can be switched on and
-- then populated). Its members' rows in `activations` follow this flag.
ALTER TABLE collections ADD COLUMN active INTEGER NOT NULL DEFAULT 0;
