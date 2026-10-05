-- Completed totals imports, including days for which Google returned no rows.
-- Replaced in the same transaction as the facts; site cleanup follows site_id.
CREATE TABLE IF NOT EXISTS search_console_totals_days (
    site_id      UUID    NOT NULL,
    property_uri VARCHAR NOT NULL,
    data_state   VARCHAR NOT NULL,
    date         DATE    NOT NULL
);
