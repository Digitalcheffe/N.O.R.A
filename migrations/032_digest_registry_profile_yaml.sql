-- Store the source file path of the originating app profile template.
ALTER TABLE digest_registry ADD COLUMN profile_source TEXT NOT NULL DEFAULT '';
