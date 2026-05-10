-- 000026b_seed_document_requirements
--
-- Seed the FL staff document requirement set. Cited from
-- active/product/florida-inspection-coverage.md. role='all' means every
-- staff role; specific roles override (e.g. 'driver' for the driver
-- physical). Sort order groups related rows together on the detail page.
--
-- Note: seeds are idempotent — we use INSERT OR IGNORE on a deterministic
-- ID per row so re-running the migration cannot duplicate rows. (Tests
-- run migrations on a fresh DB so this is belt-and-suspenders, but
-- production deploys re-apply the latest schema.)

INSERT OR IGNORE INTO document_requirements
    (id, entity_type, role, doc_kind, label, frequency, grace_period_days, authority, sort_order, state_code)
VALUES
    -- Background screening + clearinghouse cluster -----------------------------
    ('FL_STAFF_BG_INITIAL',       'staff', 'all',      'background_check',         'Background screening',                'every_n_years:5',  0,   'Handbook § 5.1; F.S. § 435.04',                  10, 'FL'),
    ('FL_STAFF_BG_FORM_5131',     'staff', 'all',      'bg_screening_form',        'Background screening form (CF-FSP 5131)', 'every_n_years:5', 0, 'Handbook § 7.4.1',                              11, 'FL'),
    ('FL_STAFF_BG_ATTEST_1649A',  'staff', 'all',      'attestation_good_moral',   'Attestation of good moral character (CF-FSP 1649A)', 'every_n_years:5', 0, 'Handbook § 7.4.1',                  12, 'FL'),

    -- Training cluster ---------------------------------------------------------
    ('FL_STAFF_INTRO_40HR',       'staff', 'all',      'intro_40hr_training',      '40-hour introductory training',       'once',             0,   'Handbook § 4.1, § 4.2.1',                       20, 'FL'),
    ('FL_STAFF_INSERVICE_10HR',   'staff', 'all',      'inservice_10hr',           '10-hour annual in-service training',  'annual',           0,   'Handbook § 4.2.7 (CF-FSP 5268)',                21, 'FL'),
    ('FL_STAFF_LITERACY_5HR',     'staff', 'all',      'early_literacy_training',  'Early literacy training (5 hrs)',     'once',             0,   'Handbook § 4.2.2',                              22, 'FL'),
    ('FL_STAFF_FIRE_EXT',         'staff', 'all',      'fire_extinguisher_training','Fire extinguisher training',        'once',             30,  'Handbook § 4.2.5',                              23, 'FL'),
    ('FL_STAFF_SAFE_SLEEP',       'staff', 'all',      'safe_sleep_training',      'Safe sleep / shaken baby training',   'once',             30,  'Handbook § 4.2.3',                              24, 'FL'),

    -- CPR + first aid ----------------------------------------------------------
    ('FL_STAFF_CPR',              'staff', 'all',      'cpr_cert',                 'Pediatric CPR',                       'every_n_years:3',  0,   'Handbook § 4.2.4; § 2.5.1',                     30, 'FL'),
    ('FL_STAFF_FIRST_AID',        'staff', 'all',      'first_aid_cert',           'First aid',                           'every_n_years:3',  0,   'Handbook § 4.2.4',                              31, 'FL'),

    -- Personnel file basics ----------------------------------------------------
    ('FL_STAFF_APPLICATION',      'staff', 'all',      'employment_application',   'Employment application',              'once',             0,   'Handbook § 7.4; F.S. § 402.3055(1)(b)',         40, 'FL'),
    ('FL_STAFF_I9',               'staff', 'all',      'i9',                       'I-9 employment eligibility',          'once',             0,   'Federal Form I-9 (USCIS)',                      41, 'FL'),
    ('FL_STAFF_W4',               'staff', 'all',      'w4',                       'W-4 tax withholding',                 'once',             0,   'IRS Form W-4',                                  42, 'FL'),
    ('FL_STAFF_REPORTER_5337',    'staff', 'all',      'mandated_reporter_5337',   'Mandated reporter form (CF-FSP 5337)', 'annual',          0,   'Handbook § 7.4',                                43, 'FL'),
    ('FL_STAFF_RECORD_ACK',       'staff', 'all',      'personnel_record_ack',     'Personnel record acknowledgement',    'once',             0,   'Handbook § 7.4',                                44, 'FL'),

    -- Director-only ------------------------------------------------------------
    ('FL_STAFF_DIRECTOR_CRED',    'staff', 'director', 'director_credential',      'Director credential',                 'every_n_years:5',  0,   'Handbook § 4.7 (CF-FSP 5290)',                  50, 'FL'),

    -- Driver-only --------------------------------------------------------------
    ('FL_STAFF_DRIVER_LICENSE',   'staff', 'driver',   'drivers_license',          'Florida driver''s license',           'every_n_years:8',  0,   'Handbook § 2.5.1; § 7.B item 34',               60, 'FL'),
    ('FL_STAFF_DRIVER_PHYSICAL',  'staff', 'driver',   'driver_physical',          'Driver physical examination',         'annual',           0,   'Handbook § 2.5.1',                              61, 'FL'),
    ('FL_STAFF_DRIVER_MVR',       'staff', 'driver',   'driver_mvr',               'Motor vehicle record (MVR)',          'annual',           0,   'Common practice (verify with Handbook § 7)',    62, 'FL'),
    ('FL_STAFF_DRIVER_TRAIN',     'staff', 'driver',   'transportation_training',  'Transportation training',             'once',             0,   'Handbook § 4.2.6',                              63, 'FL'),

    -- Cook-only ----------------------------------------------------------------
    ('FL_STAFF_FOOD_HANDLER',     'staff', 'cook',     'food_handler_cert',        'Food handler certification',          'every_n_years:3',  0,   'Handbook § 3.9 (verify with Handbook)',         70, 'FL'),

    -- Volunteer-only -----------------------------------------------------------
    ('FL_STAFF_VOLUNTEER_ACK',    'staff', 'volunteer','volunteer_ack_5217',       'Volunteer acknowledgement (CF-FSP 5217)', 'once',         0,   'Handbook § 7.4',                                80, 'FL');
