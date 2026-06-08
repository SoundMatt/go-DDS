# command-response

Request-reply over DDS using `rpc.Requester` and `rpc.Replier` (OMG DDS-RPC pattern).

**Pattern:** typed RPC built on two topics (`<service>/request`, `<service>/reply`) with correlation IDs for in-flight request matching.

## Run

```sh
go run .
```

## What it shows

| Concept | Where |
|---|---|
| `rpc.NewRequester[Req, Rep]` | client-side typed RPC |
| `rpc.NewReplier[Req, Rep]` | server-side typed RPC |
| `Replier.Requests()` channel | iterates inbound requests |
| `Replier.Reply` | sends a correlated reply |
| `dds.ReliableQoS` | required for RPC — ensures no lost requests or replies |

## Expected output

```
sent: hello       got: HELLO       from: echo-server-1
sent: world       got: WORLD       from: echo-server-1
sent: go-dds      got: GO-DDS      from: echo-server-1
sent: rpc         got: RPC         from: echo-server-1
sent: works       got: WORKS       from: echo-server-1
```
