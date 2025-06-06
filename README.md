# RSSGrid

A simple, fast, and persistent personal dashboard for consuming text-based content from various web feeds.

## Features

- OIDC authentication for secure access
- Grid-based feed layout
- Mark posts as read/unread
- Feed management (add/remove feeds)
- Automatic feed updates
- Clean, minimalist UI

## Requirements

- Go 1.21 or later
- SQLite 3
- An OIDC provider (e.g., Google, Auth0, etc.)

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/boris/go-rssgrid.git
   cd go-rssgrid
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Set up environment variables:
   ```bash
   export MONOCLE_OIDC_ISSUER_URL="https://your-oidc-provider.com"
   export MONOCLE_OIDC_CLIENT_ID="your-client-id"
   export MONOCLE_OIDC_CLIENT_SECRET="your-client-secret"
   export MONOCLE_SESSION_KEY="your-secure-session-key"
   ```

4. Build and run:
   ```bash
   go build -o rssgrid cmd/rssgrid/main.go
   ./rssgrid
   ```

## Configuration

The application can be configured using environment variables:

- `MONOCLE_OIDC_ISSUER_URL`: The URL of your OIDC provider
- `MONOCLE_OIDC_CLIENT_ID`: Your OIDC client ID
- `MONOCLE_OIDC_CLIENT_SECRET`: Your OIDC client secret
- `MONOCLE_SESSION_KEY`: A secure key for session encryption

Command line flags:

- `-addr`: HTTP server address (default: ":8080")
- `-db`: Path to SQLite database file (default: "rssgrid.db")

## Usage

1. Visit `http://localhost:8080` in your browser
2. Log in using your OIDC provider
3. Add feeds through the settings page
4. View your feeds in the grid layout
5. Click on posts to mark them as read

## Development

The project structure is organized as follows:

- `cmd/rssgrid/`: Main application entry point
- `internal/`: Internal packages
  - `auth/`: OIDC authentication
  - `db/`: Database operations
  - `feed/`: Feed fetching and parsing
  - `server/`: HTTP server and handlers
  - `templates/`: HTML templates
- `migrations/`: Database migrations

## License

This project is licensed under the MIT License - see the LICENSE file for details.
