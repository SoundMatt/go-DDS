# Low-Level Requirements (LLR)
## go-DDS

**Document ID:** LLR-001  
**Version:** 1.0  
**Date:** 2026-06-09  
**Standard:** DO-178C §5.2, ISO 26262-6 §7.2  
**Parent document:** `.fusa-reqs.json` (High-Level Requirements / HLRs)

---

## 1. Introduction

LLRs describe **how** the architecture implements each HLR. They are derived
from the architecture described in `CONTRIBUTING.md` and `cert/SDP.md §5`.
Each LLR references its parent HLR and identifies the source file(s) and
test(s) that satisfy it.

LLRs are numbered `LLR-<package>-NNN`.

---

## 2. DDS interface package (`dds`)

### LLR-DDS-001 — Participant creation

**Parent HLR:** REQ-DDS-001  
**Source:** `dds.go` (interface definition); `mock/mock.go:278 New()`  
**Test:** `mock/mock_test.go TestNew`

The `New()` function in each backend shall:
1. Accept a `dds.Domain` and variadic options
2. Return `(dds.Participant, error)`
3. Return `dds.ErrClosed` wrapped in an `fmt.Errorf` if the domain is already
   closed or unavailable
4. Be safe to call concurrently from multiple goroutines

### LLR-DDS-002 — Publisher lifecycle

**Parent HLR:** REQ-DDS-002  
**Source:** `mock/mock.go:321 NewPublisher()`, `mock/mock.go:460 Write()`  
**Test:** `mock/mock_test.go TestPublisher*`

1. `NewPublisher(topic, qos)` shall register the publisher in the broker's
   topic table under an exclusive lock
2. `Write(payload)` shall deliver `payload` to all matching subscribers
   atomically with respect to concurrent `Close()` calls
3. After `Close()`, `Write()` shall return `dds.ErrClosed`
4. The publisher's channel shall be drained before `Close()` returns

### LLR-DDS-003 — Subscriber delivery channel

**Parent HLR:** REQ-DDS-003  
**Source:** `mock/mock.go:503 subscriber`, `mock/mock.go:512 C()`  
**Test:** `mock/mock_test.go TestSubscriber*`

1. `C()` shall return a `<-chan dds.Sample` buffered to at least 64 elements
2. Samples shall arrive in the order they were published by a single publisher
3. If the channel is full and `BackPressurePolicy` is `DropNewest`, the
   incoming sample shall be silently dropped (not block the publisher)
4. After `Close()`, the channel shall be closed; subsequent reads drain remaining
   samples then return the zero value

---

## 3. Safety package (`safety`)

### LLR-SAFETY-001 — DeterministicQueue enqueue

**Parent HLR:** REQ-SAFETY-001, REQ-SAFETY-002  
**Source:** `safety/queue.go Enqueue()`  
**Test:** `safety/queue_test.go TestDeterministicQueue_Enqueue`

1. `Enqueue()` shall append the item to the ring buffer in O(1) time with no
   heap allocation after construction
2. If the queue is full and the back-pressure policy is `DropOldest`, the
   oldest element shall be overwritten; `ErrQueueFull` shall not be returned
3. If the back-pressure policy is `DropNewest`, `ErrQueueFull` shall be returned
   immediately without modifying the queue
4. `Enqueue()` shall be safe to call concurrently with `Dequeue()`

### LLR-SAFETY-002 — DeterministicQueue dequeue

**Parent HLR:** REQ-SAFETY-001, REQ-SAFETY-003  
**Source:** `safety/queue.go Dequeue()`  
**Test:** `safety/queue_test.go TestDeterministicQueue_Dequeue`

1. `Dequeue()` shall return the oldest item in O(1) time with no heap allocation
2. If the queue is empty, `Dequeue()` shall return the zero value and `false`
3. `Dequeue()` shall be safe to call concurrently with `Enqueue()`

### LLR-SAFETY-003 — E2E CRC framing

**Parent HLR:** REQ-SAFETY-007, REQ-SAFETY-008  
**Source:** `safety/e2e.go wrapE2E()`, `safety/e2e.go unwrapE2E()`  
**Test:** `safety/e2e_test.go TestE2E_CRC*`

1. `wrapE2E()` shall prepend a 6-byte header: magic (2B `0xE2E0`), sequence
   number (2B big-endian uint16), CRC-16/CCITT (2B big-endian uint16 of payload)
2. `unwrapE2E()` shall verify the magic bytes, recompute the CRC over the
   payload, and return `safety.ErrCRCMismatch` if the check fails
3. CRC computation shall use the CRC-16/CCITT polynomial (0x1021, init 0xFFFF)
4. Sequence number shall increment monotonically and wrap at 65535→0

### LLR-SAFETY-004 — Sequence gap detection

**Parent HLR:** REQ-SAFETY-009, REQ-SAFETY-010  
**Source:** `safety/e2e.go E2ESubscriber.pump()`  
**Test:** `safety/e2e_test.go TestE2E_SequenceGap`

1. The subscriber pump shall track the last received sequence number per topic
2. A gap is detected when `received_seqno != (last_seqno + 1) mod 65536`
3. On gap detection, `SafetyEventKindSequenceGap` shall be emitted on the
   safety events channel with the expected and received sequence numbers
4. The gap detection shall tolerate the first sample (no previous seqno)

### LLR-SAFETY-005 — Rate monitor polling

**Parent HLR:** REQ-SAFETY-018, REQ-SAFETY-019  
**Source:** `safety/rate.go RateMonitor`  
**Test:** `safety/rate_test.go TestRateMonitor*`

1. `Start(ctx)` shall launch a single background goroutine; subsequent calls
   before `Stop()` shall be no-ops
2. The goroutine shall call the metrics provider at the configured interval
3. For each event kind, if `(delta_count / interval_seconds) > threshold`, the
   alert callback shall be invoked exactly once per polling interval that the
   threshold is exceeded
4. When `ctx` is cancelled or `Stop()` is called, the goroutine shall exit
   within one polling interval

---

## 4. RTPS package (`rtps`)

### LLR-RTPS-001 — SPDP announcement

**Parent HLR:** REQ-RTPS-001  
**Source:** `rtps/spdp.go spdpLoop()`  
**Test:** `rtps/rtps_test.go TestSPDP*`

1. The participant shall multicast an SPDP DATA submessage to the well-known
   SPDP multicast group (`239.255.0.1:7400`) at the configured announcement
   interval (default: 1s)
2. The SPDP payload shall contain: GUID prefix, default unicast locator,
   metatraffic unicast locator, lease duration
3. Remote participants shall be evicted from the discovery table after
   `lease_duration * 2` without a received SPDP

### LLR-RTPS-002 — Reliable delivery heartbeat

**Parent HLR:** REQ-RTPS-005  
**Source:** `rtps/reliable.go heartbeatLoop()`  
**Test:** `rtps/rtps_test.go TestReliable*`

1. A writer in Reliable QoS mode shall send a HEARTBEAT submessage at the
   configured heartbeat period (default: 200ms) to each matched reader
2. The HEARTBEAT shall contain `firstSN` and `lastSN` of the in-memory history
3. On receipt of an ACKNACK for a sequence number within `[firstSN, lastSN]`,
   the writer shall retransmit the requested samples within one heartbeat period
4. `heartbeatLoop` shall terminate when the `done` channel is closed

### LLR-RTPS-003 — Fragment reassembly

**Parent HLR:** REQ-RTPS-007  
**Source:** `rtps/fragment.go`  
**Test:** `rtps/packet_test.go TestFragment*`

1. The fragment assembler shall accept DATA_FRAG submessages with the same
   sequence number and reassemble them in order of fragment number
2. Out-of-order fragments shall be buffered until all fragments arrive
3. A complete message is dispatched to the reader when all fragments are received
4. Fragment buffers for incomplete messages shall be evicted after 10s without
   completion, and `ErrFragmentTimeout` emitted

---

## 5. Security package (`security`)

### LLR-SEC-001 — HMAC-SHA-256 sign

**Parent HLR:** REQ-SECURITY-001  
**Source:** `security/hmac.go HMAC256Plugin.Sign()`  
**Test:** `security/security_test.go TestHMAC*`

1. `Sign(payload)` shall compute HMAC-SHA-256 over `payload` using the
   configured key and append the 32-byte tag to the payload
2. `Verify(payload)` shall split off the last 32 bytes, recompute HMAC, and
   return `security.ErrAuthFailed` if the tags do not match using
   `hmac.Equal()` (constant-time comparison)
3. Keys shorter than 32 bytes shall be rejected at construction with
   `security.ErrKeyTooShort`

### LLR-SEC-002 — AES-256-GCM encrypt/decrypt

**Parent HLR:** REQ-SECURITY-002  
**Source:** `security/aesgcm.go AESGCMPlugin`  
**Test:** `security/security_test.go TestAESGCM*`

1. Each call to `Encrypt()` shall generate a fresh 12-byte random nonce using
   `crypto/rand.Read()`
2. The nonce shall be prepended to the ciphertext in the output
3. `Decrypt()` shall extract the nonce from the first 12 bytes, then call
   `aead.Open()`; authentication failures return `security.ErrAuthFailed`
4. A nil or incorrect key shall return `security.ErrKeyInvalid` at construction

---

## 6. Traceability matrix

| LLR ID | Parent HLR | Source file | Test |
|---|---|---|---|
| LLR-DDS-001 | REQ-DDS-001 | `mock/mock.go:278` | `mock/mock_test.go:TestNew` |
| LLR-DDS-002 | REQ-DDS-002 | `mock/mock.go:321,460` | `mock/mock_test.go:TestPublisher*` |
| LLR-DDS-003 | REQ-DDS-003 | `mock/mock.go:503` | `mock/mock_test.go:TestSubscriber*` |
| LLR-SAFETY-001 | REQ-SAFETY-001/002 | `safety/queue.go` | `safety/queue_test.go:TestDeterministicQueue_Enqueue` |
| LLR-SAFETY-002 | REQ-SAFETY-001/003 | `safety/queue.go` | `safety/queue_test.go:TestDeterministicQueue_Dequeue` |
| LLR-SAFETY-003 | REQ-SAFETY-007/008 | `safety/e2e.go` | `safety/e2e_test.go:TestE2E_CRC*` |
| LLR-SAFETY-004 | REQ-SAFETY-009/010 | `safety/e2e.go` | `safety/e2e_test.go:TestE2E_SequenceGap` |
| LLR-SAFETY-005 | REQ-SAFETY-018/019 | `safety/rate.go` | `safety/rate_test.go:TestRateMonitor*` |
| LLR-RTPS-001 | REQ-RTPS-001 | `rtps/spdp.go` | `rtps/rtps_test.go:TestSPDP*` |
| LLR-RTPS-002 | REQ-RTPS-005 | `rtps/reliable.go` | `rtps/rtps_test.go:TestReliable*` |
| LLR-RTPS-003 | REQ-RTPS-007 | `rtps/fragment.go` | `rtps/packet_test.go:TestFragment*` |
| LLR-SEC-001 | REQ-SECURITY-001 | `security/hmac.go` | `security/security_test.go:TestHMAC*` |
| LLR-SEC-002 | REQ-SECURITY-002 | `security/aesgcm.go` | `security/security_test.go:TestAESGCM*` |

---

## 7. Revision history

| Version | Date | Author | Changes |
|---|---|---|---|
| 1.0 | 2026-06-09 | Matt Jones | Initial release |
