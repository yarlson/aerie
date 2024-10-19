# 🚀 FTL

FTL (Faster Than Light) is a powerful deployment tool designed specifically for developers who want to deploy their applications to popular cloud providers like **Hetzner**, **DigitalOcean**, **Linode**, or even **Raspberry Pi** and other servers but aren't sure where to start. FTL simplifies the process of setting up servers and deploying applications, making deployment easy and reliable, even for those who aren't experts in server management or advanced deployment techniques.

## 📌 Table of Contents

- [🚀 FTL](#-ftl)
  - [📌 Table of Contents](#-table-of-contents)
  - [🔑 Key Features](#-key-features)
  - [💻 Installation](#-installation)
    - [Option 1: Install from Source](#option-1-install-from-source)
    - [Option 2: Download Binary from GitHub](#option-2-download-binary-from-github)
    - [Option 3: Install via Homebrew (macOS and Linux)](#option-3-install-via-homebrew-macos-and-linux)
  - [⚙️ Configuration](#️-configuration)
  - [🚀 Usage](#-usage)
    - [Setup](#setup)
    - [Build](#build)
    - [Deploy](#deploy)
  - [🔄 How FTL Deploys Your Application](#-how-ftl-deploys-your-application)
  - [🌟 Benefits of FTL's Deployment Process](#-benefits-of-ftls-deployment-process)
  - [🛠️ Development](#️-development)
    - [Contributing](#contributing)
    - [Code of Conduct](#code-of-conduct)
  - [📄 License](#-license)

## 🔑 Key Features

- **Simple Configuration**: Define your entire infrastructure in a single YAML file.
- **Automated Server Setup**: Quickly provision new servers with all necessary software.
- **Zero-Downtime Deployments**: Update your applications without any service interruption.
- **Docker-based**: Leverage the power and simplicity of Docker for consistent deployments.
- **Automatic HTTPS**: Built-in Nginx proxy handles SSL/TLS certificate management.
- **Build Management**: Build and push Docker images for your services.

## 💻 Installation

There are several ways to install FTL:

### Option 1: Install from Source

To install FTL from source, you need to have [Go](https://golang.org/dl/) installed on your system. Then, you can use the following command:

```bash
go install github.com/yarlson/ftl@latest
```

### Option 2: Download Binary from GitHub

You can download the pre-compiled binary for your operating system and architecture from the [GitHub Releases page](https://github.com/yarlson/ftl/releases). After downloading, follow these steps:

1. Extract the downloaded archive.
2. Make the binary executable.
3. Move it to a directory in your PATH.

For example, on Linux or macOS:

```bash
tar -xzf ftl_0.1.0_linux_amd64.tar.gz
chmod +x ftl
sudo mv ftl /usr/local/bin/
```

### Option 3: Install via Homebrew (macOS and Linux)

If you're using Homebrew, you can install FTL using the following commands:

```bash
brew tap yarlson/ftl
brew install ftl
```

## ⚙️ Configuration

Create an `ftl.yaml` file in your project root. Here's an example:

```yaml
project:
  name: my-project
  domain: my-project.example.com
  email: my-project@example.com

servers:
  - host: my-project.example.com
    port: 22
    user: my-project
    ssh_key: ~/.ssh/id_rsa

services:
  - name: my-app
    image: my-app:latest
    port: 80
    health_check:
      path: /
      interval: 10s
      timeout: 5s
      retries: 3
    routes:
      - path: /
        strip_prefix: false

storages:
  - name: my-app-storage
    image: my-app-storage:latest
    volumes:
      - my-app-storage:/var/www/html/storage

volumes:
  - name: my-app-storage
    path: /var/www/html/storage
```

This configuration defines your project, servers, services, and storage requirements.

## Usage

FTL provides three main commands: `setup`, `build`, and `deploy`.

### Setup

The `setup` command prepares your servers for deployment:

```bash
ftl setup
```

This command:

1. Creates a new server on your chosen provider (Hetzner, DigitalOcean, Linode) or prepares your custom server (e.g., Raspberry Pi).
2. Installs Docker and other necessary software.
3. Sets up firewall rules to secure your server.
4. Adds a new user and grants them the necessary permissions.
5. Configures SSH keys for secure access.

Run this command once for each new server before deploying.

### Build

The `build` command builds Docker images for your services:

```bash
ftl build
```

This command:

1. Parses your project and service definitions.
2. Constructs Docker images for each service defined in the configuration.
3. Uploads the built images to your specified Docker registry.

You can use the `--no-push` flag to build images without pushing them to the registry:

```bash
ftl build --no-push
```

### Deploy

The `deploy` command is where the magic happens. It deploys your application to all configured servers:

```bash
ftl deploy
```

This command:

1. Parses the `ftl.yaml` file to understand your infrastructure.
2. Establishes secure SSH connections to each server.
3. Creates dedicated Docker networks for your project.
4. Ensures the latest Docker images are available on the servers.
5. Performs Zero-Downtime Deployment:
   - Starts new containers with updated images.
   - Conducts health checks to verify readiness.
   - Switches traffic to the new containers once healthy.
   - Gracefully stops and removes old containers.
6. Sets up Nginx as a reverse proxy to handle SSL/TLS and route traffic.
7. Removes any unused resources to maintain server hygiene.

## 🔄 How FTL Deploys Your Application

FTL uses a sophisticated deployment process to ensure your application is always available, even during updates. Here's what happens when you run `ftl deploy`:

1. FTL reads your `ftl.yaml` file to understand your infrastructure.
2. It securely connects to each server using SSH.
3. A dedicated Docker network is created for your project, ensuring proper isolation.
4. The latest versions of your Docker images are pulled to ensure you're deploying the most recent code.
5. For each service:

   - A new container is started with the updated image and configuration.
   - Health checks are performed to ensure the new container is ready.
   - Once healthy, traffic is instantly switched to the new container.
   - The old container is gracefully stopped and removed.

   This process ensures that your application remains available throughout the update.

6. An Nginx proxy is automatically configured to route traffic to your services, handle SSL/TLS, and provide automatic HTTPS.
7. Any unused resources are cleaned up to keep your server tidy.

The entire process is automatic and requires no manual intervention. You can deploy updates as frequently as needed without worrying about downtime or complex deployment procedures.

## 🌟 Benefits of FTL's Deployment Process

- **No Downtime**: Your application remains available during updates.
- **Automatic Rollback**: If a new version fails health checks, the old version continues to run.
- **Consistency**: Every deployment follows the same process, reducing the chance of errors.
- **Simplicity**: Complex deployment logic is handled for you, so you can focus on developing your application.
- **Scalability**: Easily deploy to multiple servers or add new services as your project grows.
- **Multi-Target Flexibility**: Choose between Hetzner, DigitalOcean, Linode, Raspberry Pi, or any other SSH-accessible server based on your needs and preferences.

## 🛠️ Development

To contribute to FTL, clone the repository and install the dependencies:

```bash
git clone https://github.com/yarlson/ftl.git
cd ftl
go mod download
```

Run the tests:

```bash
go test ./...
```

### Contributing

We welcome contributions! Whether it's reporting bugs, suggesting features, or submitting pull requests, your help is invaluable. Please ensure that your code follows the project's coding standards and that all tests pass before submitting.

## 📄 License

[MIT License](LICENSE)

---

Feel free to reach out or open an issue on our [GitHub repository](https://github.com/yarlson/ftl) if you have any questions or need assistance getting started with FTL!
