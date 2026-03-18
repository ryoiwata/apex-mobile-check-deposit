-- Track executed migrations
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    VARCHAR(50) PRIMARY KEY,
    applied_at TIMESTAMPTZ DEFAULT NOW()
);

-- Correspondents (broker-dealers)
CREATE TABLE IF NOT EXISTS correspondents (
    id                 VARCHAR(50) PRIMARY KEY,
    name               VARCHAR(100) NOT NULL,
    omnibus_account_id VARCHAR(50) NOT NULL,
    created_at         TIMESTAMPTZ DEFAULT NOW()
);

-- Investor accounts
CREATE TABLE IF NOT EXISTS accounts (
    id               VARCHAR(50) PRIMARY KEY,
    correspondent_id VARCHAR(50) NOT NULL REFERENCES correspondents(id),
    account_type     VARCHAR(20) NOT NULL CHECK (account_type IN ('individual','joint','retirement','ira_traditional','ira_roth')),
    status           VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','closed')),
    created_at       TIMESTAMPTZ DEFAULT NOW()
);

-- Transfers: central entity
CREATE TABLE IF NOT EXISTS transfers (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id             VARCHAR(50) NOT NULL,
    amount_cents           BIGINT NOT NULL CHECK (amount_cents > 0),
    declared_amount_cents  BIGINT NOT NULL CHECK (declared_amount_cents > 0),
    status                 VARCHAR(20) NOT NULL DEFAULT 'requested'
                               CHECK (status IN (
                                   'requested','validating','analyzing','approved',
                                   'funds_posted','completed','rejected','returned'
                               )),
    flagged                BOOLEAN NOT NULL DEFAULT FALSE,
    flag_reason            VARCHAR(100),
    contribution_type      VARCHAR(20),
    vendor_transaction_id  VARCHAR(100),
    micr_routing           VARCHAR(9),
    micr_account           VARCHAR(20),
    micr_serial            VARCHAR(20),
    micr_confidence        DECIMAL(3,2),
    ocr_amount_cents       BIGINT,
    front_image_ref        VARCHAR(500),
    back_image_ref         VARCHAR(500),
    settlement_batch_id    UUID,
    return_reason          VARCHAR(100),
    created_at             TIMESTAMPTZ DEFAULT NOW(),
    updated_at             TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_transfers_status  ON transfers(status);
CREATE INDEX IF NOT EXISTS idx_transfers_account ON transfers(account_id);
CREATE INDEX IF NOT EXISTS idx_transfers_created ON transfers(created_at);
CREATE INDEX IF NOT EXISTS idx_transfers_batch   ON transfers(settlement_batch_id);

-- Ledger entries: append-only
CREATE TABLE IF NOT EXISTS ledger_entries (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transfer_id           UUID NOT NULL REFERENCES transfers(id),
    to_account_id         VARCHAR(50) NOT NULL,
    from_account_id       VARCHAR(50) NOT NULL,
    type                  VARCHAR(20) NOT NULL DEFAULT 'MOVEMENT',
    sub_type              VARCHAR(20) NOT NULL,
    transfer_type         VARCHAR(20) NOT NULL DEFAULT 'CHECK',
    currency              VARCHAR(3) NOT NULL DEFAULT 'USD',
    amount_cents          BIGINT NOT NULL CHECK (amount_cents > 0),
    memo                  VARCHAR(50) DEFAULT 'FREE',
    source_application_id UUID,
    created_at            TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ledger_transfer   ON ledger_entries(transfer_id);
CREATE INDEX IF NOT EXISTS idx_ledger_to_account ON ledger_entries(to_account_id);

-- State transition audit trail
CREATE TABLE IF NOT EXISTS state_transitions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transfer_id UUID NOT NULL REFERENCES transfers(id),
    from_state  VARCHAR(20) NOT NULL,
    to_state    VARCHAR(20) NOT NULL,
    triggered_by VARCHAR(50),
    metadata    JSONB,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_state_transfer ON state_transitions(transfer_id);

-- Operator audit log
CREATE TABLE IF NOT EXISTS audit_logs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operator_id VARCHAR(50) NOT NULL,
    action      VARCHAR(20) NOT NULL CHECK (action IN ('approve','reject','override')),
    transfer_id UUID NOT NULL REFERENCES transfers(id),
    notes       TEXT,
    metadata    JSONB,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_transfer ON audit_logs(transfer_id);
CREATE INDEX IF NOT EXISTS idx_audit_operator ON audit_logs(operator_id);

-- Settlement batches
CREATE TABLE IF NOT EXISTS settlement_batches (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_date         DATE NOT NULL,
    file_path          VARCHAR(500),
    deposit_count      INTEGER NOT NULL DEFAULT 0,
    total_amount_cents BIGINT NOT NULL DEFAULT 0,
    status             VARCHAR(20) NOT NULL DEFAULT 'pending'
                           CHECK (status IN ('pending','submitted','acknowledged')),
    bank_reference     VARCHAR(100),
    created_at         TIMESTAMPTZ DEFAULT NOW()
);
