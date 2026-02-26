-- +goose Up
-- Rootfs Support Schema

-- Base Layers table: Global shared image layers extracted to JuiceFS
CREATE TABLE IF NOT EXISTS base_layers (
    id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL,
    image_ref TEXT NOT NULL,          -- Original image reference (e.g., "python:3.11")
    image_digest TEXT,                 -- Image digest after extraction
    layer_path TEXT NOT NULL,          -- JuiceFS path for the extracted layer
    size_bytes BIGINT NOT NULL DEFAULT 0,

    -- Extraction status
    status TEXT NOT NULL DEFAULT 'pending',  -- pending, extracting, ready, failed
    extracted_at TIMESTAMPTZ,
    last_error TEXT,

    -- Access tracking
    last_accessed_at TIMESTAMPTZ,
    ref_count INTEGER NOT NULL DEFAULT 0,    -- Number of sandboxes using this layer

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One layer per team per image reference
    UNIQUE(team_id, image_ref)
);

CREATE INDEX IF NOT EXISTS idx_base_layers_team_id ON base_layers(team_id);
CREATE INDEX IF NOT EXISTS idx_base_layers_status ON base_layers(status);
CREATE INDEX IF NOT EXISTS idx_base_layers_ref_count ON base_layers(ref_count);

-- Apply updated_at trigger for base_layers
DROP TRIGGER IF EXISTS update_base_layers_updated_at ON base_layers;
CREATE TRIGGER update_base_layers_updated_at
    BEFORE UPDATE ON base_layers
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Rootfs Snapshots table: Point-in-time snapshots of sandbox upper layers
CREATE TABLE IF NOT EXISTS rootfs_snapshots (
    id TEXT PRIMARY KEY,
    sandbox_id TEXT NOT NULL,
    team_id TEXT NOT NULL,

    -- Layer references
    base_layer_id TEXT NOT NULL REFERENCES base_layers(id),
    upper_volume_id TEXT NOT NULL,     -- Volume ID containing the upperdir snapshot

    -- JuiceFS metadata
    root_inode BIGINT NOT NULL,        -- Snapshot root directory inode
    source_inode BIGINT NOT NULL,      -- Source upperdir root inode at snapshot time

    -- Metadata
    name TEXT,
    description TEXT,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    metadata JSONB,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_rootfs_snapshots_sandbox_id ON rootfs_snapshots(sandbox_id);
CREATE INDEX IF NOT EXISTS idx_rootfs_snapshots_team_id ON rootfs_snapshots(team_id);
CREATE INDEX IF NOT EXISTS idx_rootfs_snapshots_base_layer_id ON rootfs_snapshots(base_layer_id);
CREATE INDEX IF NOT EXISTS idx_rootfs_snapshots_expires_at ON rootfs_snapshots(expires_at);

-- Sandbox Rootfs table: Rootfs configuration for each sandbox
CREATE TABLE IF NOT EXISTS sandbox_rootfs (
    sandbox_id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL,

    -- Layer configuration
    base_layer_id TEXT NOT NULL REFERENCES base_layers(id),
    upper_volume_id TEXT NOT NULL,     -- Volume for upperdir

    -- Overlay paths (relative to JuiceFS root)
    upper_path TEXT NOT NULL,          -- Path to upperdir
    work_path TEXT NOT NULL,           -- Path to overlay workdir

    -- Snapshot tracking
    current_snapshot_id TEXT REFERENCES rootfs_snapshots(id),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sandbox_rootfs_team_id ON sandbox_rootfs(team_id);
CREATE INDEX IF NOT EXISTS idx_sandbox_rootfs_base_layer_id ON sandbox_rootfs(base_layer_id);

-- Apply updated_at trigger for sandbox_rootfs
DROP TRIGGER IF EXISTS update_sandbox_rootfs_updated_at ON sandbox_rootfs;
CREATE TRIGGER update_sandbox_rootfs_updated_at
    BEFORE UPDATE ON sandbox_rootfs
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TRIGGER IF EXISTS update_sandbox_rootfs_updated_at ON sandbox_rootfs;
DROP INDEX IF EXISTS idx_sandbox_rootfs_base_layer_id;
DROP INDEX IF EXISTS idx_sandbox_rootfs_team_id;
DROP TABLE IF EXISTS sandbox_rootfs;

DROP INDEX IF EXISTS idx_rootfs_snapshots_expires_at;
DROP INDEX IF EXISTS idx_rootfs_snapshots_base_layer_id;
DROP INDEX IF EXISTS idx_rootfs_snapshots_team_id;
DROP INDEX IF EXISTS idx_rootfs_snapshots_sandbox_id;
DROP TABLE IF EXISTS rootfs_snapshots;

DROP TRIGGER IF EXISTS update_base_layers_updated_at ON base_layers;
DROP INDEX IF EXISTS idx_base_layers_ref_count;
DROP INDEX IF EXISTS idx_base_layers_status;
DROP INDEX IF EXISTS idx_base_layers_team_id;
DROP TABLE IF EXISTS base_layers;
