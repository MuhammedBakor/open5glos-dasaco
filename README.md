# **5GLoS**: A Scaling and Load Balancing for Free5GC

Open5glos is a Proof-of-Concept(PoC) application that serves as a load balancer proxy server for managing connections between AMF (Access and Mobility Management Function) and gNB (Next Generation Node B) nodes in a 5G network. This project facilitates the handling of NGAP (Next Generation Application Protocol) messages, enabling communication, and coordination between different network components, while maintaing even load distribution. Moreover, it provides a optimal solution for automated scaling AMF with unique AMF Id.

## Project Structure

The project is organized into several directories, each serving a specific purpose:

- **cmd/proxy**: Contains the entry point of the application.
- **internal/config**: Manages application configuration settings.
- **internal/proxy**: Implements the core proxy server functionality, including connection management and message forwarding.
- **internal/ngap**: Handles NGAP message processing and construction.
- **internal/k8s**: Interacts with Kubernetes to manage AMF pod information.
- **pkg/types**: Defines shared types and structures used throughout the application.
- **k8s_deployment**: Kubernetes deployments of Free5gc Core network, including AMF template and automated scalable AMF.
- **deployments**: Kuberenetes deployments of open5glos inside the cluster.

## Setup Instructions

1. **Clone the Repository**:
   ```bash
   git clone <repository-url>
   cd ngap-proxy
   ```

2. **Install Dependencies**:
   Ensure you have Go installed, then run:
   ```bash
   go mod tidy
   ```

3. **Build the Application**:
   To build the application, run:
   ```bash
   go build -o ngap-proxy ./cmd/proxy
   ```

4. **Run the Application**:
   Start the proxy server with:
   ```bash
   ./ngap-proxy
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
- [ ] Proxy server: NGAP decoding and parsing message.
- [ ] Proxy server: load balancing.
- [ ] Proxy server: mapping records between AMF and gNB, UE for forwarding messages.
- [ ] Proxy server: traffic routing.
- [ ] Make a Dockerfile and deploy it to Kubernetes.
- [ ] Test outside Kubernetes cluster.
- [ ] Test inside Kubernetes cluster
- [ ] Documentation
