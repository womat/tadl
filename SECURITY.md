# Security

tadl runs on a Raspberry Pi in a home network, reads the DL-Bus of a heating controller and serves the values through an HTTPS API and a web page. Reports of security issues are taken seriously.

## Reporting a vulnerability

Please **do not open a public issue**. Report it privately through GitHub instead:
**Security → Report a vulnerability** ([direct link](https://github.com/womat/tadl/security/advisories/new)).

Helpful details:

- the affected version (`tadl --version`, or `GET /version` on the device)
- steps to reproduce
- what an attacker could achieve with it

You will usually get an answer within a week. tadl is a spare-time project, so no fixed response time can be
promised.

## Supported versions

Security fixes are made for the latest release only.
