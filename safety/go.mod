module github.com/SoundMatt/go-DDS/safety

go 1.25.0

require github.com/SoundMatt/go-DDS v0.53.0

require (
	github.com/SoundMatt/RELAY v1.11.1 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

// In-repo development: build/test against the sibling root module's working
// tree instead of a published tag. See ROADMAP.md, "Architecture Initiative"
// (#71), Phase A.
replace github.com/SoundMatt/go-DDS => ../
