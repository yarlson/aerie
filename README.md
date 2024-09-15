# Aerie

Aerie is a command-line tool designed to simplify server setup and enable zero-downtime application deployment. It streamlines the process of configuring a new server and facilitates seamless deployment of your application on a remote server without requiring extensive DevOps knowledge.

## 📦 Installation

To install Aerie, ensure you have Go 1.21 or later installed on your system. Then, follow these steps:

1. Clone the repository:

```shell
git clone https://github.com/enclave-ci/aerie.git
```

2. Navigate to the project directory:

```shell
cd aerie
```

3. Build the project:

```shell
go build
```

## 🚀 Usage

### Server Setup

Before deploying your application, you need to set up your server:

```shell
./aerie setup --host <server-ip> --user <new-username> [--ssh-key <path-to-ssh-key>] [--root-key <path-to-root-key>]
```

**Required flags:**

- `--host, -H`: The IP address of the target server
- `--user, -u`: The username for the new user to be created

**Optional flags:**

- `--ssh-key, -k`: Path to the SSH public key file for the new user
- `--root-key, -r`: Path to the root SSH private key file

**Example:**

```shell
./aerie setup --host 192.168.1.100 --user newadmin --ssh-key ~/.ssh/id_rsa.pub --root-key ~/.ssh/root_id_rsa
```

### Application Deployment

After setting up your server, you can deploy your application using the `deploy` command:

```shell
./aerie deploy --host <server-ip> --user <deploy-username> [--ssh-key <path-to-ssh-key>] [--app-dir <path-to-your-app>]
```

**Required flags:**

- `--host, -H`: The IP address of the target server
- `--user, -u`: The SSH username for deployment (should be the user you created during setup)

**Optional flags:**

- `--ssh-key, -k`: Path to the SSH private key file for deployment
- `--app-dir, -d`: Path to your application's directory (default is current directory)

**Example:**

```shell
./aerie deploy --host 192.168.1.100 --user newadmin --ssh-key ~/.ssh/id_rsa --app-dir ./myapp
```

**What the `deploy` Command Does:**

1. **Builds Your Application Locally:**

   - Prepares your application for deployment using standard build processes.

2. **Connects to the Remote Server via SSH:**

   - Establishes a secure connection using the provided user credentials.

3. **Transfers Necessary Files to the Server:**

   - Uploads your application artifacts to the remote server efficiently.

4. **Deploys the Application with Zero Downtime:**

   - Utilizes a zero-downtime strategy to ensure uninterrupted service during deployment.

5. **Handles Rollbacks Automatically:**

   - If any issues are detected, Aerie rolls back to the previous stable version to maintain service availability.

### 🔑 SSH Key Handling

Aerie provides flexible options for SSH key management:

#### Root SSH Key

- If `--root-key` is provided during setup, Aerie will use this key to connect to the server as root.
- If not provided, Aerie will search for a suitable SSH key in the following order:
  1. `~/.ssh/id_rsa`
  2. `~/.ssh/id_ecdsa`
  3. `~/.ssh/id_ed25519`
- If no suitable key is found, Aerie will return an error.

#### Deployment SSH Key

- If `--ssh-key` is provided during deployment, Aerie will use this key to connect to the server.
- If not provided, Aerie will search for a suitable SSH key in the same order as the root key.
- If no suitable key is found, Aerie will return an error.

This flexible approach allows Aerie to work with your existing SSH setup while providing options for custom key paths when needed.

## ⚙️ Configuration

Aerie doesn't require any additional configuration files for basic usage. All necessary parameters are passed as command-line arguments.

For advanced configurations, you can customize your deployment settings as needed.

## 🤝 Contributing

Contributions to Aerie are welcome! Please follow these steps to contribute:

1. Fork the repository
2. Create a new branch for your feature or bug fix
3. Make your changes and commit them with a clear commit message
4. Push your changes to your fork
5. Submit a pull request to the main repository

Please ensure your code adheres to the project's coding standards and includes appropriate tests.

## 📄 License

This project is licensed under the [MIT License](LICENSE).
