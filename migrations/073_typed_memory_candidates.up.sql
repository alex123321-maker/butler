-- Typed projection is selected by trusted consumer code; model/owner HTTP claim
-- bodies do not choose a memory class. Owner decisions remain mandatory.
ALTER TABLE memory_claims ADD COLUMN memory_type TEXT NOT NULL DEFAULT 'profile'
 CHECK(memory_type IN ('profile','episodic'));
ALTER TABLE memory_claims ADD COLUMN episode_id BIGINT REFERENCES memory_episodes(id) ON DELETE SET NULL;
ALTER TABLE memory_claims ADD CONSTRAINT memory_claim_single_projection CHECK(profile_id IS NULL OR episode_id IS NULL);
-- Exact accepted episode text is useful before optional, separately admitted
-- indexing. Do not fabricate an embedding to satisfy a storage constraint.
ALTER TABLE memory_episodes ALTER COLUMN embedding DROP NOT NULL;
