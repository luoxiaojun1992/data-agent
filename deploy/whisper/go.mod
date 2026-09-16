module data-agent-whisper

go 1.25.0

require (
	github.com/ggerganov/whisper.cpp/bindings/go v0.0.0
	golang.org/x/sync v0.21.0
)

// whisper.cpp is cloned into ./whisper.cpp by the Dockerfile build; locally a
// symlink is created (gitignored) so `go mod tidy` can resolve this path.
replace github.com/ggerganov/whisper.cpp/bindings/go => ./whisper.cpp/bindings/go
