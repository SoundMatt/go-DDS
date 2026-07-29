module github.com/SoundMatt/go-DDS/tools

go 1.25.0

require (
	github.com/SoundMatt/RELAY v1.14.0
	github.com/SoundMatt/go-DDS v0.53.0
)

require google.golang.org/protobuf v1.36.11 // indirect

// In-repo development: build/test against the sibling root module's working
// tree instead of a published tag. See ROADMAP.md, "Architecture Initiative"
// (#71), Phase C.
replace github.com/SoundMatt/go-DDS => ../
