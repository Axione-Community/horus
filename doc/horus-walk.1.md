% horus-walk(1)

NAME
====

**horus-walk** - Walk an indexed metric and prints the result to stdout.

SYNOPSIS
========

| **horus-walk** \[**-h**|**-v**] \[**-b** _level_] \[**-c** _value_] \[**-c** _value_] \[**-H** _value_]
|                 \[**-p** _value_] \[**-P** _pp1,..._] OID

DESCRIPTION
===========

**horus-query** is a test tool that uses Horus to walk an indexed OID and display the processed result

Options
-------

-b, --rx-buf-size

:   Defines the snmp RX buffer size (0: keep internal default value) (default: 0)

 -c, --community

:   Defines the snmp community (default: "public")

-d, --debug

:   Defines the debug level (default: 0)

-h, --help

:   Prints (this) help message

 -H, --host

:   The snmp device host

-p, --port

:   The snmp device port (default: 161)

-P, --post-processors=pp1,pp2,...

:   The list of post processors to apply to retrieved values

-v, --version

:   Print version and build date


BUGS
====

See GitHub Issues: <https://github.com/sipsolutions/horus/issues>

AUTHOR
======

Valli A. Vallimamod <vma@sip.solutions>

SEE ALSO
========

**horus-dispatcher(1)**, **horus-agent(1)**, **horus-query(1)**
