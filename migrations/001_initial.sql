CREATE TABLE IF NOT EXISTS destinations (
    id         SERIAL PRIMARY KEY,
    city       VARCHAR(255) NOT NULL,
    country    VARCHAR(255),
    data       JSONB NOT NULL DEFAULT '{}',
    fetched_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT destinations_city_unique UNIQUE (city)
);

CREATE INDEX IF NOT EXISTS destinations_data_gin ON destinations USING GIN (data);
