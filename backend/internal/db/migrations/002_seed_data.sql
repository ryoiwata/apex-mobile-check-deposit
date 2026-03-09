-- Correspondents
INSERT INTO correspondents (id, name, omnibus_account_id) VALUES
    ('CORR-SOFI',  'SoFi',    'OMNI-SOFI-001'),
    ('CORR-WBL',   'Webull',  'OMNI-WBL-001'),
    ('CORR-CASH',  'CashApp', 'OMNI-CASH-001')
ON CONFLICT (id) DO NOTHING;

-- Investor accounts — suffixes map to vendor stub scenarios
INSERT INTO accounts (id, correspondent_id, account_type, status) VALUES
    ('ACC-SOFI-1001', 'CORR-SOFI', 'individual', 'active'),  -- IQA blur
    ('ACC-SOFI-1002', 'CORR-SOFI', 'individual', 'active'),  -- IQA glare
    ('ACC-SOFI-1003', 'CORR-SOFI', 'individual', 'active'),  -- MICR failure
    ('ACC-SOFI-1004', 'CORR-SOFI', 'individual', 'active'),  -- duplicate
    ('ACC-SOFI-1005', 'CORR-SOFI', 'individual', 'active'),  -- amount mismatch
    ('ACC-SOFI-1006', 'CORR-SOFI', 'individual', 'active'),  -- clean pass
    ('ACC-SOFI-0000', 'CORR-SOFI', 'individual', 'active'),  -- basic pass
    ('ACC-RETIRE-001','CORR-WBL',  'retirement', 'active')   -- contribution type test
ON CONFLICT (id) DO NOTHING;
