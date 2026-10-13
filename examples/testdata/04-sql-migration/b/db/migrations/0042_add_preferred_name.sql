-- Adds a preferred name, backfilled for the subject who asked for it.
ALTER TABLE subjects ADD COLUMN preferred_name text;

UPDATE subjects
SET preferred_name = 'Jane'
WHERE email = 'jane.okafor@example.com'
  AND account_number = 41000032;
