CREATE OR REPLACE MACRO hk_referrer(ref) AS (
    CASE
        WHEN ref IS NULL OR ref = '' THEN '(Direct)'
        WHEN ref LIKE 'http%' THEN regexp_extract(ref, 'https?://([^/]+)', 1)
        ELSE ref
    END
);

CREATE OR REPLACE MACRO hk_device(viewport_width) AS (
    CASE
        WHEN viewport_width < 576 THEN 'Mobile'
        WHEN viewport_width < 992 THEN 'Tablet'
        ELSE 'Desktop'
    END
);

CREATE OR REPLACE MACRO hk_country(country_code) AS (
    COALESCE(NULLIF(country_code, ''), '(Unknown)')
);
