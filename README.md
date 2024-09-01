# Aerie

Aerie is a command-line tool designed to streamline the process of setting up a new server. It automates the creation of a new user, installation of essential software (including Docker and Docker Compose), and configuration of UFW (Uncomplicated Firewall) on a remote server.

## 📦 Installation

To install Aerie, ensure you have Go 1.21.12 or later installed on your system. Then, follow these steps:

1. Clone the repository:

```
git clone https://github.com/enclave-ci/aerie.git
```

2. Navigate to the project directory:

```
cd aerie
```

3. Build the project:

```
go build
```

## 🚀 Usage

To use Aerie, run the following command:

```
./aerie --host <server-ip> --user <new-username> [--ssh-key <path-to-ssh-key>] [--root-key <path-to-root-key>]
```

Required flags:

- `--host, -H`: The IP address of the target server
- `--user, -u`: The username for the new user to be created

Optional flags:

- `--ssh-key, -k`: Path to the SSH public key file for the new user
- `--root-key, -r`: Path to the root SSH private key file

Example:

```
./aerie --host 192.168.1.100 --user newadmin --ssh-key ~/.ssh/id_rsa.pub --root-key ~/.ssh/root_id_rsa
```

## ⚙️ Configuration

Aerie doesn't require any additional configuration files. All necessary parameters are passed as command-line arguments.

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
