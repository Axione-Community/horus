% horus-agent(1)

NAME
====

**horus-agent** - Performs snmp and ping job requests from the dispatcher and posts the results on the selected data backends.

SYNOPSIS
========

| **horus-agent** \[**-h**|**-v**] \[**-B** _value_] \[**-b** _value_] \[**-d** _level_] \[**--fping-max-procs** _value_]
|                 \[**--fping-packet-count** _count_] \[**--influx-db** _value_]
|                 \[**--influx-host** _value_] \[**--influx-password** _value_]
|                 \[**--influx-retries** _value_] \[**--influx-rp** _value_]
|                 \[**--influx-timeout** _value_] \[**--influx-user** _value_] \[**-j** _count_]
|                 \[**-k** _host1,host2,..._] \[**--kafka-partition** _value_] 
|                 \[**--kafka-topic** _value_] \[**--log** _dir_] \[**-m** percent] \[**--mock**]
|                 \[**-n** _host1,host2,..._] \[**--nats-name** _value_]
|                 \[**--nats-reconnect-delay** _seconds_] \[**--nats-subject** _value_]
|                 \[**-p** _port_] \[**-P** _url1,url2,..._] \[**--prom-max-age** _sec_]
|                 \[**--prom-sweep-frequency** _sec_] \[**-s** _sec_] \[**-t** _msec_]

DESCRIPTION
===========

The agent receives job requests from the dispatcher over http. If it has remaining capacity, it accepts and queues the job. The job is an json document containing all
information about the device to poll, the metrics to retrieve and the backends where to send the results.

At the end of a polling job, the agent posts the results to Kafka, NATS or InfluxDB and keeps them in memory for Prometheus scraping. It also sends back a report to the dispatcher
with the polling duration and error if any. Ping results (min, max, avg, loss) are kept in memory for Prometheus scraping only and no report is sent back to the agent.

The result posted to Kafka is a big json document containing the aggregated poll results for each device. You can use **horus-query(1)** to get the same data on stdout.

The Prometheus metrics are named using the `<measure name>_<metric name>` pattern, for example: `sysInfo_sysUpTime` and they have the following default labels: id, host,
vendor, model and category of the polled device. The snmp metrics are pushed in batch using prometheus remote-write API; the `/pingmetrics` and `/metrics` endpoints are still available
for scrapping ping metrics and agent monitoring data.

Options
=======

General options
---------------

-b, --snmp-buf-size=value

:   UDP receive buffer size for snmp replies (in bytes) (default: 6000)

-d, --debug

:   Specifies the debug level from 1 to 3. Defaults to 0 (disabled).

-h, --help

:   Prints a help message.

-j, --snmp-jobs

:   Specifies the snmp polling job capacity of this agent. Defaults to 1; when set to 0, snmp polling is disabled.

    --log

:   Specifies the directory where the log files are written. The files are created and rotated by the glog lib (https://github.com/vma/glog).
    If not set, logs are written to stderr.

-m, --max-mem-load=percent

:    Max memory usage allowed before rejecting new jobs (default: 90%)

    --mock

:   Runs the agent in mock mod for snmp requests.

-p, --port

:   Specifies the listen port of the API web server. Defaults to 8080.

-s, --stat-frequency

:   Specifies the frequency in seconds at which gather and log agent stats (memory usage, ongoing polls, prometheus stats.) Disabled if set to 0 (default.)

-t, --inter-poll-delay

:   Specifies the time to wait in ms between each snmp poll request. It is used to smoothe the load and avoid spikes. Defaults to 100ms.

-v, --version

:   Prints the current version and build date.

Ping related options
--------------------

    --fping-max-procs

:   Specifies the max ping requests capacity for this agent (based on max simultaneous fping processes). Defaults to 5.

    --fping-packet-count

:   Specifies the number of ping requests sent to each host. Defaults to 15.


InfluxDB related options
------------------------

    --influx-host

:   Specifies the influxDB host address. Push to influxDB disabled if empty (default). The subsequent options are needed only if this one is set.

    --influx-db

:   Specifies the influxDB database.

    --influx-user

:   Specifies the influxDB user login.

    --influx-password

:    Specifies the influxDB user password.

    --influx-retries

:   Specifies the influxDB write retry count in case of error. Defaults to 2.

    --influx-rp

:   Specifies the influxDB retention policy for the pushed data. Defaults to "autogen".

    --influx-timeout

:   Specifies the influxDB write timeout in seconds. Defaults to 5s.


Kafka related options
---------------------

-k, --kafka-hosts

:   Specifies the Kafka brokers host names or IP addresses list. Push to Kafka is disabled if empty (default).

    --kafka-partition

:   Specifies the Kafka write partition to use. Defaults to 0.

    --kafka-topic

:   Specifies the Kafka topic to use for the snmp results.


NATS related options
--------------------

-n, --nats-hosts

:   NATS hosts list (push to NATS disabled if empty)

    --nats-name

:   NATS connection name

    --nats-subject

:  NATS subject for snmp results

    --nats-reconnect-delay

:  Delay in seconds before reconnecting to the NATS server on lost connection



Prometheus related options
--------------------------

-B, --prom-batch-size=value

:   Number of timeseries to accumulate before a remote write (default: 5000)

-D, --prom-push-deadline=sec

:   Max time to wait before remote write even if buffer is not full (default: 120)

-P, --prom-endpoints=url1,url2,...

:   Prometheus endpoint list for remote write

    --prom-max-age

:   Specifies the maximum time in second to keep Prometheus samples in memory. If set to 0 (default), Prometheus collectors are disabled.

    --prom-sweep-frequency

:   Specifies the cleaning frequency in second of old Prometheus samples. Defaults to 120s.

-T, --prom-timeout=sec

:   Prometheus write timeout (default: 2)


BUGS
====

See GitHub Issues: <https://github.com/sipsolutions/horus/issues>

AUTHOR
======

Valli A. Vallimamod <vma@sip.solutions>

SEE ALSO
========

**horus-dispatcher(1)**, **horus-query(1)**, **horus-walk(1)**
