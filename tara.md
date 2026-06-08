# Threat Analysis and Risk Assessment (TARA)

**Module:** github.com/SoundMatt/go-DDS  
**Generated:** 2026-06-08T21:18:02Z  
**Standard:** ISO 21434 Chapter 9  

| ID | Asset | Threat | STRIDE | CWE | Vector | Likelihood | Impact | SL | Control | Residual Risk |
|---|---|---|---|---|---|---|---|---|---|---|
| TARA-001 | main.go | Predictable random values enable authentication bypass or token forgery | S/T | CWE-330 | Network | Low | Medium | 2 | Use crypto/rand | Low after remediation |
| TARA-002 | fault.go | Predictable random values enable authentication bypass or token forgery | S/T | CWE-330 | Network | Low | Medium | 2 | Use crypto/rand | Low after remediation |
| TARA-003 | spdp.go | Predictable random values enable authentication bypass or token forgery | S/T | CWE-330 | Network | Low | Medium | 2 | Use crypto/rand | Low after remediation |
| TARA-004 | cyclone.go | Unsafe pointer usage causes undefined behaviour or memory corruption | E | CWE-242 | Local | Medium | High | 3 | Remove unsafe usage; use safe Go idioms | Medium — requires code redesign |
| TARA-005 | traffic_linux.go | Unsafe pointer usage causes undefined behaviour or memory corruption | E | CWE-242 | Local | Medium | High | 3 | Remove unsafe usage; use safe Go idioms | Medium — requires code redesign |
| TARA-006 | testutil_test.go | Command injection from variable input enables arbitrary command execution | E/R | CWE-78 | Network | Medium | High | 3 | Use exec.Command with fixed command and sanitised args | Low after remediation |
| TARA-007 | wan.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-008 | cdr.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-009 | cdr.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-010 | cdr.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-011 | cdr.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-012 | cdr.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-013 | cdr.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-014 | cdr.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-015 | cdr.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-016 | mock.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-017 | mock_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-018 | mock_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-019 | pool_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-020 | pool_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-021 | fault_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-022 | fault_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-023 | fault_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-024 | fault_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-025 | fault_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-026 | cdr.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-027 | cdr.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-028 | cdr.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-029 | cdr.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-030 | cdr.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-031 | cdr.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-032 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-033 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-034 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-035 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-036 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-037 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-038 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-039 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-040 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-041 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-042 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-043 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-044 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-045 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-046 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-047 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-048 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-049 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-050 | fragment.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-051 | guid.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-052 | guid.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-053 | guid.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-054 | guid.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-055 | guid.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-056 | guid.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-057 | guid.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-058 | guid.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-059 | guid.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-060 | guid.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-061 | guid.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-062 | locator.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-063 | locator.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-064 | locator.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-065 | locator.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-066 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-067 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-068 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-069 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-070 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-071 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-072 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-073 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-074 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-075 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-076 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-077 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-078 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-079 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-080 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-081 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-082 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-083 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-084 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-085 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-086 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-087 | message.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-088 | packet_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-089 | packet_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-090 | packet_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-091 | packet_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-092 | packet_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-093 | packet_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-094 | packet_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-095 | packet_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-096 | packet_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-097 | participant.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-098 | persist.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-099 | reliable.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-100 | rtps_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-101 | rtps_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-102 | wire_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-103 | wire_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-104 | wire_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-105 | wire_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-106 | e2e.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-107 | e2e_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-108 | e2e_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-109 | cert.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-110 | cert.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-111 | cert_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-112 | cert_test.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-113 | shmem.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-114 | shmem.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-115 | diagnostics.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-116 | taprio.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-117 | taprio_linux.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-118 | taprio_linux.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-119 | taprio_linux.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-120 | taprio_linux.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-121 | taprio_linux.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-122 | taprio_linux.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-123 | taprio_linux.go | Integer narrowing conversion causes silent data truncation | T/D | CWE-190 | Local | Low | Medium | 1 | Add range check before conversion | Low after remediation |
| TARA-124 | tsn_test.go | World-readable/writable file allows unauthorised data access or tampering | I/T | CWE-732 | Local | Medium | Medium | 2 | Create file with mode 0640 or stricter | Low after remediation |
| TARA-125 | tsn_test.go | World-readable/writable file allows unauthorised data access or tampering | I/T | CWE-732 | Local | Medium | Medium | 2 | Create file with mode 0640 or stricter | Low after remediation |
| TARA-126 | tsn_test.go | World-readable/writable file allows unauthorised data access or tampering | I/T | CWE-732 | Local | Medium | Medium | 2 | Create file with mode 0640 or stricter | Low after remediation |
