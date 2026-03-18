-- Correspondents
INSERT INTO correspondents (id, name, omnibus_account_id) VALUES
    ('CORR-SOFI',  'SoFi',    'OMNI-SOFI-001'),
    ('CORR-WBL',   'Webull',  'OMNI-WBL-001'),
    ('CORR-CASH',  'CashApp', 'OMNI-CASH-001')
ON CONFLICT (id) DO NOTHING;

-- Investor accounts — account type and status drive business rules;
-- vendor stub behavior is controlled via the explicit scenario dropdown.
INSERT INTO accounts (id, correspondent_id, account_type, status) VALUES
    ('ACC-SOFI-1001',  'CORR-SOFI', 'individual',    'active'),    -- default individual account
    ('ACC-SOFI-1002',  'CORR-SOFI', 'joint',         'active'),    -- joint account type
    ('ACC-RETIRE-001', 'CORR-SOFI', 'ira_traditional','active'),   -- traditional IRA (contribution type)
    ('ACC-RETIRE-002', 'CORR-SOFI', 'ira_roth',      'active'),    -- Roth IRA (contribution type)
    ('ACC-SUSPENDED',  'CORR-SOFI', 'individual',    'suspended')  -- suspended → account eligibility rejection
ON CONFLICT (id) DO NOTHING;
