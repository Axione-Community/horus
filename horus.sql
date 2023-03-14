CREATE TABLE agents (
    id serial PRIMARY KEY,
    ip_address character varying NOT NULL,
    port integer NOT NULL DEFAULT 80,
    active boolean NOT NULL DEFAULT false,
    is_alive boolean NOT NULL DEFAULT false,
    load real NOT NULL DEFAULT 0,
    last_checked_at timestamp with time zone,
    UNIQUE (ip_address, port)
);

CREATE TABLE profiles (
    id serial PRIMARY KEY,
    category character varying NOT NULL,
    vendor character varying NOT NULL,
    model character varying NOT NULL,
    UNIQUE(category, vendor, model)
);

CREATE TABLE devices (
    id serial PRIMARY KEY,
    active boolean NOT NULL DEFAULT true,
    hostname character varying NOT NULL,
    ip_address character varying,
    is_polling boolean NOT NULL DEFAULT false,
    last_pinged_at timestamp with time zone,
    last_polled_at timestamp with time zone,
    ping_frequency integer NOT NULL DEFAULT 0,
    polling_frequency integer NOT NULL DEFAULT 300,
    profile_id integer NOT NULL REFERENCES profiles(id) ON UPDATE CASCADE ON DELETE SET NULL,
    snmp_alternate_community character varying NOT NULL DEFAULT '',
    snmp_community character varying NOT NULL,
    snmp_connection_count integer NOT NULL DEFAULT 1,
    snmp_disable_bulk boolean NOT NULL DEFAULT false,
    snmp_port integer NOT NULL DEFAULT 161,
    snmp_retries integer NOT NULL DEFAULT 1,
    snmp_timeout integer NOT NULL DEFAULT 10,
    snmp_max_repetitions integer NOT NULL DEFAULT 50,
    snmp_version character varying NOT NULL DEFAULT '2c',
    snmpv3_auth_passwd character varying NOT NULL DEFAULT '',
    snmpv3_auth_proto character varying NOT NULL DEFAULT '',
    snmpv3_auth_user character varying NOT NULL DEFAULT '',
    snmpv3_privacy_passwd character varying NOT NULL DEFAULT '',
    snmpv3_privacy_proto character varying NOT NULL DEFAULT '',
    snmpv3_security_level character varying NOT NULL DEFAULT '',
    tags json NOT NULL DEFAULT '{}'::json,
    allowed_agent_ids []integer NOT NULL DEFAULT '{}'::integer[]
    UNIQUE (hostname, ip_address)
);

CREATE TABLE metrics (
    active boolean NOT NULL DEFAULT true,
    description text NOT NULL,
    export_as_label boolean NOT NULL DEFAULT false,
    exported_name character varying,
    id serial PRIMARY KEY,
    index_pattern character varying NOT NULL DEFAULT '',
    name character varying NOT NULL,
    oid character varying NOT NULL,
    polling_frequency integer NOT NULL DEFAULT 0,
    post_processors character varying[] DEFAULT '{}' NOT NULL,
    UNIQUE (oid, index_pattern)
);

CREATE TABLE measures (
    id serial PRIMARY KEY,
    description text NOT NULL,
    filter_metric_id integer REFERENCES metrics(id),
    filter_pattern character varying NOT NULL DEFAULT '',
    index_metric_id integer REFERENCES metrics(id),
    invert_filter_match boolean NOT NULL DEFAULT false,
    is_indexed boolean NOT NULL DEFAULT false,
    name character varying NOT NULL,
    to_influx boolean NOT NULL DEFAULT false,
    to_kafka boolean NOT NULL DEFAULT true,
    to_prometheus boolean NOT NULL DEFAULT true,
    to_nats boolean NOT NULL DEFAULT true,
    use_alternate_community boolean NOT NULL DEFAULT false,
    UNIQUE (name)
);

CREATE TABLE measure_metrics (
    id serial PRIMARY KEY,
    measure_id integer NOT NULL REFERENCES measures(id) ON UPDATE CASCADE ON DELETE CASCADE,
    metric_id integer NOT NULL REFERENCES metrics(id) ON UPDATE CASCADE ON DELETE CASCADE,
    UNIQUE (measure_id, metric_id)
);

CREATE TABLE metric_poll_times (
    id serial PRIMARY KEY,
    device_id integer NOT NULL REFERENCES devices(id) ON UPDATE CASCADE ON DELETE CASCADE,
    metric_id integer NOT NULL REFERENCES measures(id) ON UPDATE CASCADE ON DELETE CASCADE,
    last_polled_at timestamp with time zone NOT NULL,
    UNIQUE (device_id, metric_id)
);

CREATE TABLE profile_measures (
    id serial PRIMARY KEY,
    profile_id integer NOT NULL REFERENCES profiles(id) ON UPDATE CASCADE ON DELETE CASCADE,
    measure_id integer NOT NULL REFERENCES measures(id) ON UPDATE CASCADE ON DELETE CASCADE,
    UNIQUE (profile_id, measure_id)
);

ALTER TABLE agents OWNER TO horus;
ALTER TABLE profiles OWNER TO horus;
ALTER TABLE devices OWNER TO horus;
ALTER TABLE metrics OWNER TO horus;
ALTER TABLE measures OWNER TO horus;
ALTER TABLE measure_metrics OWNER TO horus;
ALTER TABLE metric_poll_times OWNER TO horus;
ALTER TABLE profile_measures OWNER TO horus;
