# Security policy

## Supported versions

Until v1.0.0, only the latest tagged pre-release and the main branch receive
security fixes. A version support table will be published with v1.0.0.

## Reporting a vulnerability

Do not open a public issue for a suspected vulnerability.

Use GitHub private vulnerability reporting when it is enabled for this
repository. If it is unavailable, contact the maintainer through the private
contact method listed on the maintainer's GitHub profile.

Include:

- affected version or commit;
- impact and expected attack scenario;
- minimal reproduction;
- suggested mitigation, if known; and
- whether the issue is already public.

You should receive an acknowledgement within seven days. Coordinated
disclosure timing will depend on severity and release complexity.

## Security boundaries

This library depends on the application to:

- authenticate to and secure Redis;
- configure Redis TLS and network controls;
- select an appropriate fail-open or fail-closed policy;
- configure trusted proxy networks correctly;
- choose identities that cannot be cheaply rotated by attackers; and
- avoid logging secret identity values.

See [proxy security](docs/proxy-security.md) for forwarding-header guidance.
