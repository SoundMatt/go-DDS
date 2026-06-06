# go-DDS

A Go library for DDS (Data Distribution Service) publish/subscribe, designed for vehicle signal transport.

The API is defined as a stable Go interface. Implementations are swappable:

| Package | Description | Requires |
|---|---|---|
| `mock` | In-process, pure Go. Default for development and testing. | Nothing |
| `cyclone` | CycloneDDS via CGo. Real DDS domain (multi-process, multi-host). | `libcyclonedds-dev` + `-tags cyclone` |
| `rtps` _(planned)_ | Pure-Go RTPS wire protocol. Zero native dependencies, real DDS. | Nothing |

## Quick start

```go
import (
    dds "github.com/SoundMatt/go-DDS"
    "github.com/SoundMatt/go-DDS/mock"
)

p, _ := mock.New(dds.Domain(0))
defer p.Close()

pub, _ := p.NewPublisher("/VIN001/Vehicle", dds.DefaultQoS)
sub, _ := p.NewSubscriber("/VIN001/Vehicle", dds.DefaultQoS)

pub.Write([]byte(`{"replyTopic":"client/123","request":{"action":"get","path":"Vehicle.Speed"}}`))

sample := <-sub.C()
fmt.Println(string(sample.Payload))
```

## Using CycloneDDS (production)

```bash
# Install CycloneDDS
apt-get install -y libcyclonedds-dev    # Debian/Ubuntu
brew install cyclonedds                 # macOS

# Build with real DDS
go build -tags cyclone ./...
```

```go
import (
    dds "github.com/SoundMatt/go-DDS"
    "github.com/SoundMatt/go-DDS/cyclone"
)

p, err := cyclone.New(dds.Domain(0))
```

## Wire protocol (VISS over DDS)

The topic naming convention mirrors VISS over MQTT:

- **Request topic**: `/<VIN>/Vehicle` — the vehicle subscribes here
- **Payload**: `{"replyTopic":"<unique>","request":{...VISS JSON...}}`  
- **Reply topic**: `<unique>` — the client subscribes here for the response

This envelope is identical to the MQTT protocol so the server-side routing
logic in `vissv2server/mqttMgr` and `vissv2server/ddsMgr` is symmetric.

## Roadmap

- [x] Go interface definition (`dds.Participant`, `Publisher`, `Subscriber`)
- [x] In-process mock (development and testing)
- [x] CycloneDDS CGo implementation (production, multi-host)
- [ ] Pure-Go RTPS/UDP implementation (phase 2 — no CGo, any platform)
- [ ] Reliable QoS with retransmission
- [ ] DDS-Security (DTLS / auth plugins)
