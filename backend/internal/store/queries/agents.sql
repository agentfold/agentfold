-- name: ListAgentProfiles :many
SELECT * FROM agent_profiles ORDER BY name;

-- name: GetAgentProfile :one
SELECT * FROM agent_profiles WHERE id = $1;

-- name: GetAgentProfileByName :one
SELECT * FROM agent_profiles WHERE name = $1;

-- name: InsertAgentProfile :one
INSERT INTO agent_profiles (kind, name) VALUES ($1, $2) RETURNING *;

-- name: RenameAgentProfile :one
UPDATE agent_profiles SET name = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: SetAgentCredential :one
UPDATE agent_profiles SET credential = $2, credential_kind = $3, updated_at = now() WHERE id = $1 RETURNING *;

-- name: DeleteAgentProfile :execrows
DELETE FROM agent_profiles WHERE id = $1;

-- name: ListMcpServers :many
SELECT * FROM mcp_servers ORDER BY name;

-- name: GetMcpServer :one
SELECT * FROM mcp_servers WHERE id = $1;

-- name: InsertMcpServer :one
INSERT INTO mcp_servers (name, transport, config, secrets) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: UpdateMcpServer :one
UPDATE mcp_servers
SET name = $2, transport = $3, config = $4, secrets = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteMcpServer :execrows
DELETE FROM mcp_servers WHERE id = $1;
