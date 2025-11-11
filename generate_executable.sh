#!/bin/bash


# generate binary per macos platform
OOS=darwin GOARCH=arm64 go build -o atomic_generator_arm64
OOS=darwin GOARCH=amd64 go build -o atomic_generator_amd64

# generate cross platform binary
lipo -create -output generate_workflow atomic_generator_amd64 atomic_generator_arm64

# delete temp binaries
rm atomic_generator_arm64
rm atomic_generator_amd64
