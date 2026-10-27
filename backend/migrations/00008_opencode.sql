-- +goose Up
-- opencode profiles: their credential is what `opencode auth export` prints,
-- imported into the card's agent home before each run.
ALTER TABLE agent_profiles DROP CONSTRAINT agent_profiles_kind_check;
ALTER TABLE agent_profiles ADD CONSTRAINT agent_profiles_kind_check CHECK (kind IN ('claude', 'opencode'));
ALTER TABLE agent_profiles DROP CONSTRAINT agent_profiles_credential_kind_check;
ALTER TABLE agent_profiles ADD CONSTRAINT agent_profiles_credential_kind_check
    CHECK (credential_kind IN ('oauth_token', 'api_key', 'opencode_auth'));

-- +goose Down
DELETE FROM agent_profiles WHERE kind = 'opencode';
ALTER TABLE agent_profiles DROP CONSTRAINT agent_profiles_credential_kind_check;
ALTER TABLE agent_profiles ADD CONSTRAINT agent_profiles_credential_kind_check
    CHECK (credential_kind IN ('oauth_token', 'api_key'));
ALTER TABLE agent_profiles DROP CONSTRAINT agent_profiles_kind_check;
ALTER TABLE agent_profiles ADD CONSTRAINT agent_profiles_kind_check CHECK (kind IN ('claude'));
