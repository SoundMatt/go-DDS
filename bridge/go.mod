module github.com/SoundMatt/go-DDS/bridge

go 1.25.0

require (
	github.com/SoundMatt/go-DDS v0.53.0
	google.golang.org/grpc v1.81.1
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/SoundMatt/RELAY/v2 v2.0.4 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

// In-repo development: build/test against the sibling root module's working
// tree instead of a published tag. See ROADMAP.md, "Architecture Initiative"
// (#71), Phase B.
replace github.com/SoundMatt/go-DDS => ../
