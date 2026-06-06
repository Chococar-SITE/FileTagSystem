-- Schema v2 — FTS5 full-text index over field-value labels + aliases (§5.5).
-- The trigram tokenizer gives substring matching (incl. CJK) without the
-- full-table LIKE scan on the core search path. rowid mirrors field_values.id;
-- text = the value plus all of its aliases. Triggers keep it in sync.

CREATE VIRTUAL TABLE field_value_fts USING fts5(text, tokenize = 'trigram');

INSERT INTO field_value_fts(rowid, text)
SELECT fv.id,
       fv.value || ' ' || COALESCE(
         (SELECT group_concat(a.alias, ' ') FROM field_value_aliases a WHERE a.field_value_id = fv.id), '')
FROM field_values fv;

-- New value: index its label (aliases are added later via their own trigger).
CREATE TRIGGER fv_fts_ai AFTER INSERT ON field_values BEGIN
  INSERT INTO field_value_fts(rowid, text) VALUES (new.id, new.value);
END;

-- Value renamed: rebuild label + current aliases.
CREATE TRIGGER fv_fts_au AFTER UPDATE OF value ON field_values BEGIN
  DELETE FROM field_value_fts WHERE rowid = old.id;
  INSERT INTO field_value_fts(rowid, text) VALUES (new.id,
    new.value || ' ' || COALESCE(
      (SELECT group_concat(a.alias, ' ') FROM field_value_aliases a WHERE a.field_value_id = new.id), ''));
END;

-- Value (and its subtree) deleted: drop the index row.
CREATE TRIGGER fv_fts_ad AFTER DELETE ON field_values BEGIN
  DELETE FROM field_value_fts WHERE rowid = old.id;
END;

-- Alias added: rebuild the owning value's text.
CREATE TRIGGER fva_fts_ai AFTER INSERT ON field_value_aliases BEGIN
  DELETE FROM field_value_fts WHERE rowid = new.field_value_id;
  INSERT INTO field_value_fts(rowid, text)
    SELECT fv.id, fv.value || ' ' || COALESCE(
      (SELECT group_concat(a.alias, ' ') FROM field_value_aliases a WHERE a.field_value_id = fv.id), '')
    FROM field_values fv WHERE fv.id = new.field_value_id;
END;

-- Alias reparented (e.g. tag merge): rebuild both old and new owners.
CREATE TRIGGER fva_fts_au AFTER UPDATE OF field_value_id ON field_value_aliases BEGIN
  DELETE FROM field_value_fts WHERE rowid IN (old.field_value_id, new.field_value_id);
  INSERT INTO field_value_fts(rowid, text)
    SELECT fv.id, fv.value || ' ' || COALESCE(
      (SELECT group_concat(a.alias, ' ') FROM field_value_aliases a WHERE a.field_value_id = fv.id), '')
    FROM field_values fv WHERE fv.id IN (old.field_value_id, new.field_value_id);
END;

-- Alias removed: rebuild the owning value's text (if it still exists).
CREATE TRIGGER fva_fts_ad AFTER DELETE ON field_value_aliases BEGIN
  DELETE FROM field_value_fts WHERE rowid = old.field_value_id;
  INSERT INTO field_value_fts(rowid, text)
    SELECT fv.id, fv.value || ' ' || COALESCE(
      (SELECT group_concat(a.alias, ' ') FROM field_value_aliases a WHERE a.field_value_id = fv.id), '')
    FROM field_values fv WHERE fv.id = old.field_value_id;
END;
