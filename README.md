# Pair-API ![version](https://img.shields.io/badge/version-0.2.2-blue)
**Test APIs for the native Pair applications**

## Overview
This repository contains API implementations written using Rust and Go. They are not interchangeable. The Rust API is, in essence, an earlier version of the Go API. Their respective features are detailed below. I suggest not attempting to run these APIs directly, but reading their source code and applying their structures and functions for yourself. These APIs were designed for an earlier prototype of the Pair app that utilized internet connectivity, but now the Pair app is designed to be fully offline.

### Rust
- gRPC server
- Protobufs as medium for data
- Redis used as experimental database
- JWT verification & validation

### Go
- gRPC server
- Protobufs as medium for data
- MongoDB
- JWT verification & validation
- Auth0 communication
- Extensive error checking
- Applicable server response codes
- Simple logging

## Getting Started
### Rust
Navigate to `rust` directory \
Execute `cargo run server`

### Go
Navigate to `go/pair` directory \
Execute `docker compose up`

## Disclaimer
These APIs require the protobuf compiler (protoc) and certification PEMs to work correctly. I do not guarantee you will be able to execute them on your own device without reading the actual Rust and Go code to figure out what is required. Proficient Docker knowledge is especially recommended when spinning up the Go API. Also, the Go API CA PEM and private key PEM should be stored in a go/pair/cert directory. The public key PEM should be Base64 encrypted so that it can be read from an .env file. Other .env variables must be set and an Auth0 account must be configured as well.

## License
GPL-3.0 License. See `LICENSE.txt` for more information.
