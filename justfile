dev:
  overmind start -f Procfile.dev

compose:
  docker-compose up --build

docker:
  docker build . --platform=linux/amd64 -t reg.mindwork.cool/emoji-registry:latest

push:
  docker push reg.mindwork.cool/emoji-registry:latest

install-deps:
  go install github.com/air-verse/air@latest
  brew install tmux
  brew install overmind

# Regenerate *_templ.go from the .templ views under app/views.
generate:
  go generate ./...

# Regenerate the views, then keep them fresh while you edit .templ files.
generate-watch:
  go tool templ generate -path app/views -watch
