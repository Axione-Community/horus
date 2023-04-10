% horus-dispatcher(1)

NAME
====

**horus-dispatcher** - Dispatchs snmp and ping jobs to agents.

SYNOPSIS
========

| **horus-dispatcher** \[**-h**|**-v**] \[**-c** _url_] \[**-C** url] \[**-d** _level_] \[**--device-max-lock-time** _seconds_]
|                      \[**-g** _seconds_] \[**-H** _host1:port1,host2:port2,..._]
|                      \[**-i** _address_] \[**-k** _seconds_] \[**-l** _value_] \[**--log** _dir_]
|                      \[**-m** _value_] \[**--max-load-delta** _value_]
|                      \[**--ping-batch-count** _value_]
|                      \[**--polling-interval-tolerance** _second_] \[**-p** _port_]
|                      \[**-q** _seconds_] \[**-u** _seconds_] \[**-w** _sec_]

DESCRIPTION
===========

The dispatcher queries periodically the `devices` table for devices whose `last_polled_at` value is past its `polling_frequency` and whose `is_polling` flag is not set. Then
it retrieves the snmp metrics for each resulting device and builds a json that is sent sequentially over http to all available agents until accepted (the agent replies with a code 202).
If no agent accepts the job, it is discarded (it will be resent on the next round). Otherwise, the device's `is_polling` flag is set and `last_polled_at` is set to current time.

Upon completion of the polling requests, the agent sends a report to the dispatcher. If there was a polling error, it is saved to the reports table for subsequent inspection.

Ping requests are dispatched in the same way except there is no report and the metrics are saved to Prometheus only.

The in-memory agent list is kept up to date from db and each agent is checked regurarly to get its status and load. Dead agents are discarded until they are back again.

A pg adivsory lock is requested at startup and held by the first launched process to ensure that only one instance is active. Any other started instance becomes only active after the first one stops.

Options
=======

-c, --dsn

:   Specifies the postgres db connection DSN like `postgres://horus:secret@localhost/horus`.

-C, --lock-dsn

:   Specifies the postgres db DSN to use for advisory locks. Must be different from main DSN.

-d, --debug

:   Specifies the debug level from 1 to 3. Defaults to 0 (disabled).

    --device-max-lock-time

:   Force unlock devices locked longer than this delay (default: 600)

-g, --db-ping-freq

:   Specifies the db query frequency in seconds for new available ping jobs. Defaults to 10s; when set to 0, ping queries are disabled.

-H, --cluster-hosts

:   Lists all hosts of the dispatcher cluster

-h, --help

:   Prints a help message.

-i, --ip

:   Specifies the web server local listen IP for devices API and end job reports from agents. Defaults to the system's first ip address.
    Must be non-zero as it is used for the report url given to the agents.

-k, --agent-keepalive-freq

:   Specifies the agent keep-alive requests frequency in seconds. Defaults to 30s.

-l, --lock-id

:   Defines postgres advisory lock id to ensure single running process. First started process acquires the locks and becomes master. This behaviour is disabled
    by default or when the lock ID is set to 0.

    --log

:   Specifies the directory where the log files are written. The files are created and rotated by the glog lib (https://github.com/vma/glog).
    If not set, logs are written to stderr.

-m, --db-max-snmp-jobs

:   Defines  maximum number of snmp jobs to retrieve from db at each query (default: 200)

    --max-load-delta

:   Specifies the max load delta allowed between agents before moving a device to another agent. The load of an agent is defined as the ratio of
    the current queued and ongoing jobs over total agent's capacity. We do a load based balancing but for better memory usage, we try to stick
    a device to the same agent as log as possible even if it is not the least loaded. Defaults to 0.1.

    --ping-batch-count

:   Specifies the number of hosts to query per agent's fping process. Defaults to 100.

-p, --port

:   Specifies the listen port of the API web server. Defaults to 8080.

     --polling-interval-tolerance=second

:   Specifies the tolerance interval to start a new polling job before its due time. Defaults to 1s.

-q, --db-snmp-freq

:   Specifies the check frequency in seconds for new available snmp polling jobs in database. Defaults to 30s; when set to 0, snmp queries are disabled.

-u, --device-unlock-freq

:   Specifies the frequency in seconds for the device unlocker goroutine. On each keep-alive, the agents return to the dispatcher their ongoing requests.
    The device unlocker automatically resets the device's `is_polling` flag if this device is not currently polled by any agent. Defaults to 600s.

-v, --version

:   Prints the current version and build date.

 -w, --load-avg-window=sec

:   SNMP load avg calculation window (default: 30)

BUGS
====

See GitHub Issues: <https://github.com/sipsolutions/horus/issues>

AUTHOR
======

Valli A. Vallimamod <vma@sip.solutions>

SEE ALSO
========

**horus-agent(1)**, **horus-query(1)**, **horus-walk(1)**
