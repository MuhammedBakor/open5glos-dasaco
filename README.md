# **5GLoS**: A Scaling and Load Balancing for Free5GC

Open5glos is a Proof-of-Concept(PoC) application that serves as a load balancer proxy server for managing connections between AMF (Access and Mobility Management Function) and gNB (Next Generation Node B) nodes in a 5G network. This project facilitates the handling of NGAP (Next Generation Application Protocol) messages, enabling communication, and coordination between different network components, while maintaing even load distribution. Moreover, it provides a optimal solution for automated scaling AMF with unique AMF Id.

## Project Structure

## Directory Structure

```
5glos/
├── cmd/
│   └── proxy/
│       └── main.go           # Application entry point
├── internal/
│   ├── amf/
│   │   ├── amf.go           # AMF implementation: handling messages from/to AMF.
│   │   └── manager.go       # AMF manager for connection management.
│   ├── config/
│   │   ├── config.go        # Settings for deployment in Kubernetes cluster.
│   ├── gnb/
│   │   ├── gnb.go           # GNB implementation: handling messages from/to gNB(RAN).
│   │   └── manager.go       # GNB manager for connection management.
│   ├── ngap/
│   │   ├── connection.go    # NGAP connection handling: connection wrapper and read/send NGAP message.
│   │   └── server.go        # SCTP proxy server. 
│   ├── service/
│   │   └── service.go       # Main service orchestrator.
│   ├── ue/
│   │   └── context.go       # UE context management.
│   └── utils/
│       ├── kubernetes.go    # Kubernetes utilities: get minikube IP
│       └── ngap_builders.go # NGAP message builders.
├── go.mod
├── go.sum
└── Makefile             # Dockerfile
```

## Setup Instructions

1. **Clone the Repository**:
   ```bash
   git clone https://github.com/HasukiHT/5glos.git
   cd 5glos
   ```

2. **Install Dependencies**:
   Ensure you have Go installed, then run:
   ```bash
   go mod tidy
   ```

3. **Build the Application**:
   To build the application, run:
   ```bash
   go build ./..
   ```

4. **Run the Application**:
   Start the proxy server with:
   ```bash
   ./proxy
   ```

## Usage

Once the application is running, it will listen for connections from gNB nodes and AMF instances. The proxy server will handle NGAP messages, forwarding them appropriately between the connected nodes.

## Contributing

Contributions are welcome! Please feel free to submit a pull request or open an issue for any enhancements or bug fixes.

## License

This project is licensed under the MIT License. See the LICENSE file for details.


## To-Do list

- [x] Check the gNB and UE registration of UERANSIM inside the cluster (for 1 AMF).
- [x] Check the gNB and UE registration of UERANSIM outside the cluster (for 1 AMF).
- [x] Check the gNB and UE registration of UERANSIM inside the cluster (for multiple AMFs).
- [x] Check the gNB and UE registration of UERANSIM outside the cluster (for multiple AMFs).
- [x] Proxy server: get AMF info, connect AMF, and connect gNB.
- [x] Proxy server: NGAP decoding and parsing message.
- [x] Proxy server: load balancing.
- [x] Proxy server: mapping records between AMF and gNB, UE for forwarding messages (Setup, Registration).
- [x] Proxy server: test one and multiple AMFs with mapping function.
- [x] Proxy server: Reorganize structure.
- [ ] Proxy server: traffic routing (all messages).
- [ ] Make a Dockerfile and deploy it to Kubernetes.
- [x] Test outside Kubernetes cluster.
- [ ] Set up environment and build to run inside Kuberenetes.
- [ ] Test inside the Kubernetes cluster.
- [ ] Experiment test setup.
- [ ] Documentation.
